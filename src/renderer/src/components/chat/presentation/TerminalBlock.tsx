/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useMemo, useState } from 'react'
import clsx from 'clsx'
import { parseAnsiLines, type AnsiLine } from './ansi'
import { headTailCap } from './head-tail-cap'
import css from './TerminalBlock.module.css'

/** Output lines shown before the height cap collapses the middle. */
export const DEFAULT_TERMINAL_MAX_LINES = 16

/**
 * Display copy for the terminal surface; the owner passes localized labels
 * (this package is cordis-free, so copy arrives via props).
 */
export interface TerminalBlockLabels {
  /** Status pill text for a signal-terminated command. */
  signal: (signal: string) => string
  /** Status pill text for a non-zero exit code. */
  exitCode: (exitCode: number) => string
  /** Status pill text for a command that ended without an exit code: killed by a signal the view does not know, or never started. */
  noExitCode: string
  /** Run-state text while the command is still running. */
  running: string
  /** Run-state text for a signal or non-zero-exit settle. */
  failed: string
  /** Run-state text for a clean settle. */
  done: string
  /** Copy-button idle label. */
  copy: string
  /** Copy-button label during the post-copy confirmation window. */
  copied: string
  /** Placeholder when a settled command produced no visible output. */
  status: (status: string) => string
  save: string
  noOutput: string
  /** Collapse-toggle aria label while expanded. */
  collapseAria: string
  /** Collapse-toggle text while expanded. */
  collapse: string
  /** Expand-toggle aria label while capped, given the hidden line count. */
  expandAria: (hidden: number) => string
  /** Expand-toggle text while capped, given the hidden line count. */
  expand: (hidden: number) => string
}

export interface TerminalBlockProps {
  command: string
  output?: string
  exitCode?: number
  status: 'completed' | 'failed' | 'timeout' | 'canceled' | 'unknown'
  maxLines?: number
  className?: string
  labels: TerminalBlockLabels
  onCopy: () => void
  onSave: () => void
  copied: boolean
}

/**
 * Render one parsed output line. Runs without SGR state render as bare text,
 * so uncolored output carries no span wrappers.
 * @param line - the line's styled runs.
 * @returns the line's children.
 */
function renderLine(line: AnsiLine) {
  return line.map((span, index) => span.style === undefined
    ? span.text
    : <span key={index} style={span.style}>{span.text}</span>)
}

/**
 * Render a shell command as a terminal surface.
 * @param props - see {@link TerminalBlockProps}.
 * @returns the terminal block element.
 */
export function TerminalBlock({
  command,
  output,
  exitCode,
  status: hostStatus,
  onCopy,
  onSave,
  copied,
  maxLines = DEFAULT_TERMINAL_MAX_LINES,
  className,
  labels,
}: TerminalBlockProps) {
  const copy = labels
  const text = output ?? ''
  // A command's output ends with a newline; that terminator is not an extra
  // blank line to draw or to count against the height cap. The check runs on the
  // PARSED lines rather than on the raw text, because a reset after the final
  // newline (`line\n\x1b[0m`) leaves the string not ending in one while still
  // producing a last line with nothing visible in it. A genuinely blank final
  // line — the double newline — survives, since it has a real empty line before
  // the terminator. The copy control's payload is unaffected.
  const lines = useMemo(() => {
    const parsed = parseAnsiLines(text)
    const last = parsed[parsed.length - 1]
    const terminated = parsed.length > 1 && last !== undefined
      && last.every(span => span.text === '')
    return terminated ? parsed.slice(0, -1) : parsed
  }, [text])
  const [expanded, setExpanded] = useState(false)
  // Host owns effects; no renderer text is a clipboard or save authority.

  const onToggle = useCallback(() => { setExpanded(value => !value) }, [])

  const running = false // This surface only receives sealed foreground results.
  const status = exitCode !== undefined && exitCode !== null && exitCode !== 0 ? copy.exitCode(exitCode) : undefined
  const state = { label: copy.status(hostStatus) }
  const statusColor = {
    completed: 'var(--ds-success)', failed: 'var(--ds-danger)', timeout: 'var(--ds-danger)',
    canceled: 'var(--ds-text-muted)', unknown: 'var(--ds-text-faint)'
  }[hostStatus]
  // A multi-line command gets one prompt row per line, so a two-command shell
  // snippet reads as the two commands it is instead of collapsing into one
  // ellipsized row. A trailing newline is a terminator, not an empty command.
  const commandLines = useMemo(() => {
    const body = command.endsWith('\n') ? command.slice(0, -1) : command
    return body.split('\n')
  }, [command])
  // Read from the parsed lines the card actually renders, not from the raw text:
  // output that is only escapes or control bytes (a lone reset, an OSC title, an
  // erase) survives `text.trim()` yet parses to nothing visible. Judging it on
  // the raw text would draw an output box of blank rows plus a copy control
  // for invisible bytes, and hide the placeholder that belongs there.
  const empty = lines.every(line => line.every(span => span.text.trim() === ''))
  const { hidden, capped, headLines, tailLines } = headTailCap(lines.length, maxLines, expanded)
  // A running command with nothing printed yet is banner-only; once it has
  // printed, the output streams in under the banner as it would in a terminal.
  const body = !running || !empty

  return (
    <div
      className={clsx(css.block, className)}
      data-terminal=""
      data-running={running ? '' : undefined}
      data-body={body ? '' : undefined}
    >
      <div className={css.header}>
        <div className={css.prompt}>
          <span className={css.runStateLabel}>{state.label}</span>
          {commandLines.map((line, index) => (
            <div key={index} className={css.promptLine}>
              {/* One dot for the card, on the first row: the exit status the
                  view carries is the whole call's, and bash reports no
                  per-command status, so a dot per row would assert a
                  per-line outcome nothing here knows. */}
              {index === 0 && <span className={css.runState} aria-hidden style={{ width: 6, height: 6, borderRadius: '50%', background: statusColor }} />}
              {/* The cwd labels the CALL, so only its first row carries it. The
                  view knows one working directory — where the call started —
                  and a later line may well run somewhere else (a `cd` in the
                  command is enough), so repeating the label down the rows would
                  assert a directory per line that nothing here knows. Later
                  rows keep a bare `$` to stay aligned as prompts. */}
              <span className={css.cwd}>
                {'$'}
              </span>
              <span className={css.command}>{line}</span>
            </div>
          ))}
        </div>
        <span className={css.status} style={{ color: statusColor }} data-tool-status={hostStatus}>{state.label}</span>
        {status !== undefined && <span className={css.status} style={{ color: statusColor }}>{status}</span>}
        <button type="button" className={css.copyButton} onClick={onSave}>{copy.save}</button>
        {true && (
          <button type="button" className={css.copyButton} onClick={onCopy}>
            {copied ? copy.copied : copy.copy}
          </button>
        )}
      </div>
      {body && (empty
        ? <div className={css.empty}>{copy.noOutput}</div>
        : (
          <div className={css.output}>
            {(capped ? lines.slice(0, headLines) : lines).map((line, index) => (
              <div key={index} className={css.line}>{renderLine(line)}</div>
            ))}
            {hidden > 0 && (
              <button
                type="button"
                className={css.expand}
                aria-expanded={expanded}
                aria-label={expanded ? copy.collapseAria : copy.expandAria(hidden)}
                onClick={onToggle}
              >
                {expanded ? copy.collapse : copy.expand(hidden)}
              </button>
            )}
            {capped && lines.slice(lines.length - tailLines).map((line, index) => (
              <div key={index} className={css.line}>{renderLine(line)}</div>
            ))}
          </div>
        ))}
    </div>
  )
}
