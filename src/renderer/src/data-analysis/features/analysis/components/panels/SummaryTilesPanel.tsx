import { ArrowUp, BarChart3, Users, ReceiptText, ArrowDown } from "../../../../../design/AnalytixUiIcons";
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
      <ArrowUp aria-hidden="true" />
    );
  }

  if (kind === "net") {
    return (
      <BarChart3 aria-hidden="true" />
    );
  }

  if (kind === "counterparty") {
    return (
      <Users aria-hidden="true" />
    );
  }

  if (kind === "txn") {
    return (
      <ReceiptText aria-hidden="true" />
    );
  }

  return (
    <ArrowDown aria-hidden="true" />
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
