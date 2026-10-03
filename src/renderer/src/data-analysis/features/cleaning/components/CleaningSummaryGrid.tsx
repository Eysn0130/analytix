import { FileText, Clock3, CompareArrows, Identity, ShieldCheck } from "../../../../design/AnalytixUiIcons";
import type { CleaningSummaryCardIcon, CleaningSummaryCardViewModel } from "../model/view-model";

interface CleaningSummaryGridProps {
  cards: CleaningSummaryCardViewModel[];
}

function CleaningSummaryIcon({ icon }: { icon: CleaningSummaryCardIcon }): JSX.Element {
  if (icon === "file") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <FileText aria-hidden="true" focusable="false" />
      </span>
    );
  }
  if (icon === "scope") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <Clock3 aria-hidden="true" focusable="false" />
      </span>
    );
  }
  if (icon === "transaction") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <CompareArrows aria-hidden="true" focusable="false" />
      </span>
    );
  }
  if (icon === "account") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <Identity aria-hidden="true" focusable="false" />
      </span>
    );
  }
  return (
    <span className="cleaning-kpi__icon" aria-hidden="true">
            <ShieldCheck aria-hidden="true" focusable="false" />
    </span>
  );
}

export function CleaningSummaryGrid({ cards }: CleaningSummaryGridProps): JSX.Element {
  return (
    <section className="cleaning-summary-grid">
      {cards.map((card) => (
        <article key={card.key} className={`cleaning-kpi ${card.className}`}>
          <div className="cleaning-kpi__head">
            <CleaningSummaryIcon icon={card.icon} />
            <span className="cleaning-kpi__label">{card.label}</span>
          </div>
          <strong className="cleaning-kpi__value">{card.value}</strong>
          <small className="cleaning-kpi__meta">{card.meta}</small>
        </article>
      ))}
    </section>
  );
}
