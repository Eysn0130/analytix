/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
/** Local Markdown destinations accepted by the file-preview callback. */

/**
 * Decode a file destination and its optional GitHub-style line fragment.
 * Literal `?` and `#` in filenames must be percent-encoded.
 * @param value - Parsed Markdown link destination.
 * @returns A local path and optional first line, or undefined for URLs,
 * fragment-only links, queries, malformed escapes, or invalid line ranges.
 */
export function parseFileLink(value: string): { path: string; line?: number } | undefined {
  const hash = value.indexOf('#')
  const destination = hash < 0 ? value : value.slice(0, hash)
  if (destination.includes('?')) return undefined
  let path: string
  try {
    path = decodeURIComponent(destination)
  } catch (_error) {
    // Malformed percent escapes cannot identify a file unambiguously.
    return undefined
  }
  if (path.length === 0 || /[\u0000-\u001f\u007f]/.test(path)
    || /^[\\/]{2}/.test(path)
    || (/^[a-z][a-z\d+.-]*:/i.test(path) && !/^[a-z]:[\\/]/i.test(path))) return undefined
  if (hash < 0) return { path }
  const fragment = value.slice(hash + 1)
  const match = /^L([1-9]\d*)(?:-L([1-9]\d*))?$/.exec(fragment)
  if (match === null) return undefined
  const line = Number(match[1])
  const end = match[2] === undefined ? line : Number(match[2])
  if (!Number.isSafeInteger(line) || !Number.isSafeInteger(end) || end < line) return undefined
  return { path, line }
}
