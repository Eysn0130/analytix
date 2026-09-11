export type SummaryGlyphKind = "in" | "out" | "net" | "counterparty" | "txn";

interface SummaryTile {
  label: string;
  value: string;
  tone: string;
  glyph: SummaryGlyphKind;
  metric: "money" | "count";
}

interface SummaryTilesPanelProps {
  tiles: SummaryTile[];
  hasSelection: boolean;
}

function SummaryGlyph({ kind }: { kind: SummaryGlyphKind }): JSX.Element {
  if (kind === "out") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M7 16.5V8.25M7 8.25L4.5 10.75M7 8.25l2.5 2.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M4.5 17.5h15" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
        <path d="M12.25 14.75h5.25V8.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    );
  }

  if (kind === "net") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M5 16.5V7.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
        <path d="M5 16.5h14" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
        <path d="M7.5 14.25l3.25-3.5 2.35 1.95 3.9-5.15" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
        <circle cx="10.75" cy="10.75" r="1.15" fill="currentColor" />
        <circle cx="13.1" cy="12.7" r="1.15" fill="currentColor" />
        <circle cx="16.95" cy="7.35" r="1.15" fill="currentColor" />
      </svg>
    );
  }

  if (kind === "counterparty") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="8" cy="8.25" r="2.5" stroke="currentColor" strokeWidth="1.6" />
        <circle cx="16.25" cy="9.5" r="2.2" stroke="currentColor" strokeWidth="1.6" />
        <path d="M4.75 17.5c.75-2.35 2.62-3.75 5.25-3.75 2.55 0 4.3 1.21 5 3.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
        <path d="M14.35 17.1c.45-1.55 1.63-2.45 3.3-2.45 1.02 0 1.93.32 2.6.92" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    );
  }

  if (kind === "txn") {
    return (
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="5.25" y="4.75" width="13.5" height="14.5" rx="2.4" stroke="currentColor" strokeWidth="1.6" />
        <path d="M8.5 9h7M8.5 12h7M8.5 15h4.25" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
        <circle cx="16.8" cy="16.1" r="2.2" stroke="currentColor" strokeWidth="1.5" />
        <path d="m18.35 17.65 1.55 1.55" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    );
  }

  return (
    <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M6.25 7.75h4.5a2.25 2.25 0 0 1 2.25 2.25v7.75" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M16.25 16.5 13 19.75 9.75 16.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M17.75 7.75H13.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
    </svg>
  );
}

export function SummaryTilesPanel({ tiles, hasSelection }: SummaryTilesPanelProps): JSX.Element {
  return (
    <section className="chart-summary-tiles" aria-label="顶部卡片">
      {tiles.map((tile) => (
        <div
          key={tile.label}
          className={[
            "chart-summary-tile",
            `chart-summary-tile--${tile.tone}`,
            `chart-summary-tile--${tile.metric}`,
            !hasSelection ? "is-muted" : "",
          ]
            .filter(Boolean)
            .join(" ")}
        >
          <div className="chart-summary-tile__icon" aria-hidden="true">
            <SummaryGlyph kind={tile.glyph} />
          </div>
          <div className="chart-summary-tile__content">
            <div className="chart-summary-tile__label">{tile.label}</div>
            <div className="chart-summary-tile__value" title={tile.value}>
              {tile.value}
            </div>
          </div>
        </div>
      ))}
    </section>
  );
}
