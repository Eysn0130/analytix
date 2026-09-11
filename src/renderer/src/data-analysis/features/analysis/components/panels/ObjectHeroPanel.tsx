interface ObjectHeroStat {
  tone: "in" | "out" | "net";
  label: string;
  value: string;
}

interface ObjectHeroCopy {
  muted: boolean;
  title: string;
  subtitle: string;
  stats: ObjectHeroStat[];
}

interface ObjectHeroPanelProps {
  heroCopy: ObjectHeroCopy;
  dateRangeLabel: string;
}

export function ObjectHeroPanel({ heroCopy, dateRangeLabel }: ObjectHeroPanelProps): JSX.Element {
  return (
    <section className={`chart-object-hero ${heroCopy.muted ? "is-muted" : ""}`}>
      <div className="chart-object-hero__main">
        <div className="chart-object-hero__title">{heroCopy.title}</div>
        {heroCopy.stats.length ? (
          <div className="chart-object-hero__stats" aria-label="分析摘要">
            {heroCopy.stats.map((stat) => (
              <div key={stat.label} className={`chart-object-hero__stat chart-object-hero__stat--${stat.tone}`}>
                <span className="chart-object-hero__stat-label">{stat.label}</span>
                <span className="chart-object-hero__stat-value" title={stat.value}>
                  {stat.value}
                </span>
              </div>
            ))}
          </div>
        ) : null}
        {heroCopy.subtitle ? <div className="chart-object-hero__subtitle">{heroCopy.subtitle}</div> : null}
      </div>
      <div className="chart-object-hero__aside">
        <div className="chart-object-hero__date-pill" aria-label={`分析时间范围：${dateRangeLabel}`}>
          <span className="chart-object-hero__date-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none">
              <rect x="4.75" y="5.75" width="14.5" height="13.5" rx="2.2" stroke="currentColor" strokeWidth="1.6" />
              <path d="M8 3.75v4M16 3.75v4M4.75 9.25h14.5" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </span>
          <span className="chart-object-hero__date-text">{dateRangeLabel}</span>
        </div>
      </div>
    </section>
  );
}
