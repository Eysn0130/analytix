import type { CleaningSummaryCardIcon, CleaningSummaryCardViewModel } from "../model/view-model";

interface CleaningSummaryGridProps {
  cards: CleaningSummaryCardViewModel[];
}

function CleaningSummaryIcon({ icon }: { icon: CleaningSummaryCardIcon }): JSX.Element {
  if (icon === "file") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <svg viewBox="0 0 20 20" fill="none" focusable="false">
              <path d="M6.5 3.5H11.5L14.5 6.5V15.25C14.5 15.94 13.94 16.5 13.25 16.5H6.75C6.06 16.5 5.5 15.94 5.5 15.25V4.75C5.5 4.06 6.06 3.5 6.75 3.5H6.5Z" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
              <path d="M11.5 3.75V6.5H14.25" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
            </svg>
      </span>
    );
  }
  if (icon === "scope") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <svg viewBox="0 0 20 20" fill="none" focusable="false">
              <circle cx="10" cy="10" r="6.5" stroke="currentColor" strokeWidth="1.4" />
              <path d="M10 6.6V10.15L12.45 11.6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
      </span>
    );
  }
  if (icon === "transaction") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <svg viewBox="0 0 20 20" fill="none" focusable="false">
              <path d="M4.75 7.25H13.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
              <path d="M11.25 4.9L13.6 7.25L11.25 9.6" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
              <path d="M15.25 12.75H6.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
              <path d="M8.75 10.4L6.4 12.75L8.75 15.1" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
      </span>
    );
  }
  if (icon === "account") {
    return (
      <span className="cleaning-kpi__icon" aria-hidden="true">
            <svg viewBox="0 0 20 20" fill="none" focusable="false">
              <rect x="4.5" y="4.25" width="11" height="11.5" rx="2.2" stroke="currentColor" strokeWidth="1.4" />
              <path d="M7.2 8.15C7.2 7.15 8 6.35 9 6.35H11C12 6.35 12.8 7.15 12.8 8.15V8.6C12.8 9.6 12 10.4 11 10.4H9C8 10.4 7.2 9.6 7.2 8.6V8.15Z" stroke="currentColor" strokeWidth="1.2" />
              <path d="M7.2 13.65C7.54 12.46 8.52 11.85 9.85 11.85H10.15C11.48 11.85 12.46 12.46 12.8 13.65" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
            </svg>
      </span>
    );
  }
  return (
    <span className="cleaning-kpi__icon" aria-hidden="true">
            <svg viewBox="0 0 20 20" fill="none" focusable="false">
              <path d="M10 3.75L14.4 5.55V9.15C14.4 12.05 12.58 14.74 10 15.9C7.42 14.74 5.6 12.05 5.6 9.15V5.55L10 3.75Z" stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
              <path d="M8.1 9.8L9.4 11.05L11.95 8.5" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
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
