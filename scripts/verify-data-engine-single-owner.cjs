#!/usr/bin/env node
function writeSummary(summary) {
  console.log(JSON.stringify(summary, null, 2))
}

async function main() {
  const summary = {
    ok: false,
    skipped: false,
    platform: process.platform,
    executionAuthority: 'unavailable',
    pingOk: false,
    ownerGuardOk: false,
    ownerConflictObserved: false,
    warnings: [],
    failures: []
  }

  if (process.platform !== 'win32') {
    summary.ok = true
    summary.skipped = true
    summary.warnings.push('windows_host_required')
    writeSummary(summary)
    return
  }

  // A packaged data-engine may only be started by the single Go execution
  // authority. Windows remains unavailable until suspended launch, immutable
  // image admission, no-breakaway kill-on-close Job Object containment,
  // bounded protocol I/O, and exact tree termination are implemented. Do not
  // restore a Node process fallback for release verification.
  summary.failures.push('managed_process_execution_authority_unavailable')
  writeSummary(summary)
  process.exitCode = 1
}

main().catch((error) => {
  writeSummary({
    ok: false,
    platform: process.platform,
    pingOk: false,
    ownerGuardOk: false,
    ownerConflictObserved: false,
    warnings: [],
    failures: ['managed_process_verification_failed']
  })
  process.exitCode = 1
})
