const PREFIX = 'ANALYTIX_STARTUP_OWNER_PHASE_V1 '
const MAX_PHASE_DURATION_MS = 30 * 60_000
const MAX_TRACE_STDERR_BYTES = 4096
const MAX_TRACE_LINES = 16

export const startupOwnerPhasesV1 = [
  'funds_package_inspected',
  'funds_source_inspected',
  'funds_static_admitted',
  'funds_active_resolved',
  'funds_ready_built',
  'go_lease_acquired',
  'go_semantic_prepared',
  'go_listener_bound',
  'go_activated',
  'go_host_probed',
  'go_ready_built',
  'go_prepare_failed_cancelled',
  'go_prepare_failed_deadline',
  'go_prepare_failed_persistence_integrity',
  'go_prepare_failed_startup_resource_limit',
  'go_prepare_failed_semantic_migration_blocked',
  'go_prepare_failed_startup_snapshot_drift',
  'go_prepare_failed_private_cas_integrity',
  'go_prepare_failed_private_cas_materialization_limit',
  'go_prepare_failed_private_domain_records_unavailable',
  'go_prepare_failed_child_producer_inventory_incomplete',
  'go_prepare_failed_pending_authority',
  'go_prepare_failed_continuation_authority',
  'go_prepare_failed_history_invalid',
  'go_prepare_failed_cleanup',
  'go_prepare_failed_unknown',
  'go_prepare_failed_root_authority_unknown',
  'go_prepare_failed_original_create_unknown',
  'go_prepare_failed_journal_unknown',
  'go_prepare_failed_configuration_unknown',
  'go_prepare_failed_child_inventory_unknown',
  'go_prepare_failed_persistence_recovery_unknown',
  'go_prepare_failed_persistence_authority_advance_unknown',
  'go_prepare_failed_persistence_report_preservation_unknown',
  'go_prepare_failed_persistence_final_history_bind_unknown',
  'go_prepare_failed_persistence_retained_original_unknown',
  'go_prepare_failed_persistence_journal_load_unknown',
  'go_prepare_failed_persistence_child_revalidate_unknown',
  'go_prepare_failed_persistence_authority_revalidate_unknown',
  'go_prepare_failed_persistence_signed_recovery_validation_unknown',
  'go_prepare_failed_persistence_create_residues_unknown',
  'go_prepare_failed_persistence_semantic_recovery_unknown',
  'go_prepare_failed_persistence_orphan_topology_unknown',
  'go_prepare_failed_persistence_owner_recovery_unknown',
  'go_prepare_failed_persistence_checkpoint_recovery_unknown',
  'go_prepare_failed_semantic_plan_unknown',
  'go_prepare_failed_durable_store_unknown',
  'go_prepare_failed_history_unknown',
  'go_prepare_failed_final_reconciliation_unknown',
  'go_prepare_failed_runtime_composition_unknown',
  'go_prepare_failed_provider_authority_unknown',
  'go_prepare_failed_runtime_components_unknown',
  'go_prepare_failed_final_snapshot_unknown',
  'go_prepare_failed_generation_unknown'
] as const

export type StartupOwnerPhaseV1 = Readonly<{
  stage: typeof startupOwnerPhasesV1[number]
  durationMs: number
}>

const phaseSet: ReadonlySet<string> = new Set(startupOwnerPhasesV1)

export function parseStartupOwnerPhaseLineV1(line: string): StartupOwnerPhaseV1 | null {
  if (line.length > 128 || !line.startsWith(PREFIX)) return null
  const match = line.slice(PREFIX.length).match(/^([a-z_]+) (0|[1-9][0-9]*)$/u)
  if (!match || !phaseSet.has(match[1])) return null
  const durationMs = Number(match[2])
  if (!Number.isSafeInteger(durationMs) || durationMs > MAX_PHASE_DURATION_MS) return null
  return { stage: match[1] as StartupOwnerPhaseV1['stage'], durationMs }
}

// The materialization command retains its strict one-line stdout contract.
// Only complete, bounded, fixed diagnostic lines are accepted on stderr.
export function parseBundledFundsStartupStderrV1(stderr: Buffer): readonly StartupOwnerPhaseV1[] | null {
  if (stderr.length === 0) return []
  if (stderr.length > MAX_TRACE_STDERR_BYTES) return null
  const raw = stderr.toString('utf8')
  if (!raw.endsWith('\n')) return null
  const lines = raw.slice(0, -1).split('\n')
  if (lines.length === 0 || lines.length > MAX_TRACE_LINES) return null
  const phases = lines.map(parseStartupOwnerPhaseLineV1)
  if (phases.some((phase) => !phase || !phase.stage.startsWith('funds_'))) return null
  return phases as StartupOwnerPhaseV1[]
}

export function createStartupOwnerPhaseStreamV1(
  onPhase: (phase: StartupOwnerPhaseV1) => void
): Readonly<{ accept(chunk: Buffer | string): void }> {
  let pending = ''
  return {
    accept(chunk) {
      const text = String(chunk)
      if (text.length > MAX_TRACE_STDERR_BYTES) {
        pending = ''
        return
      }
      pending += text
      for (;;) {
        const newline = pending.indexOf('\n')
        if (newline < 0) break
        const line = pending.slice(0, newline)
        pending = pending.slice(newline + 1)
        const phase = parseStartupOwnerPhaseLineV1(line)
        if (phase && phase.stage.startsWith('go_')) {
          try { onPhase(phase) } catch { /* optional diagnostics cannot gate startup */ }
        }
      }
      if (pending.length > MAX_TRACE_STDERR_BYTES) pending = ''
    }
  }
}
