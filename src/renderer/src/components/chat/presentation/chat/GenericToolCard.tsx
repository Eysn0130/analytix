/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { ToolView, type ToolViewProps } from './ToolViews'
// Unknown producer contracts retain their real public output and metadata.
// Known views dispatch before this fallback; no upstream runtime is imported.
export function GenericToolCard(props: ToolViewProps) { return <ToolView {...props} family="tool" /> }
