/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useLayoutEffect, useState } from 'react'

/** Disclosure belongs to this view generation, never to runtime authority. */
export function useDisclosure(version: string, defaultOpen = false) {
  const [override, setOverride] = useState<{ version: string; open: boolean } | null>(null)
  // Forget the old override, including when the same version string later
  // recurs after an outer fold or identity transition.
  useLayoutEffect(() => { setOverride(null) }, [version])
  const open = override?.version === version ? override.open : defaultOpen
  const toggle = useCallback(() => setOverride({ version, open: !open }), [version, open])
  const setOpen = useCallback((value: boolean) => setOverride({ version, open: value }), [version])
  return { open, toggle, setOpen, manuallyOpened: override?.version === version && override.open }
}
