/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
/** Consumer-owned navigation for Markdown links. */
import { createContext, useContext, useMemo } from 'react'
import type { ReactNode } from 'react'

/**
 * Handle one sanitized absolute HTTP(S) URL selected from Markdown.
 * @param href - destination URL.
 */
export type MarkdownExternalLinkHandler = (href: string) => void

/** Ordinary external-link activation supplied by the nearest Markdown owner. */
export interface MarkdownDelegate {
  /** Ordinary HTTP(S) activation; absent handlers retain native anchor behavior. */
  readonly openExternalLink?: MarkdownExternalLinkHandler | undefined
}

const MarkdownDelegateContext = createContext<MarkdownDelegate>({})

/** Props for one Markdown navigation scope. */
export interface MarkdownDelegateProviderProps extends MarkdownDelegate {
  readonly children: ReactNode
}

/**
 * Scope Markdown navigation without threading callbacks through renderers.
 * Nested providers replace the enclosing capabilities. Handler changes reach cached links.
 * @param props - Child tree and its ordinary HTTP(S) link handler.
 * @returns the scoped child tree.
 */
export function MarkdownDelegateProvider({
  children,
  openExternalLink,
}: MarkdownDelegateProviderProps): ReactNode {
  const delegate = useMemo(() => ({ openExternalLink }), [openExternalLink])
  return (
    <MarkdownDelegateContext.Provider value={delegate}>
      {children}
    </MarkdownDelegateContext.Provider>
  )
}

/**
 * Read the nearest Markdown navigation capabilities.
 * @returns Owner callbacks, or an empty delegate outside a provider.
 */
export function useMarkdownDelegate(): MarkdownDelegate {
  return useContext(MarkdownDelegateContext)
}
