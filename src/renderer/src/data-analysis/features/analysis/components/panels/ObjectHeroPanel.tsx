import { Calendar } from "../../../../../design/AnalytixUiIcons";
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
            <Calendar aria-hidden="true" />
          </span>
          <span className="chart-object-hero__date-text">{dateRangeLabel}</span>
        </div>
      </div>
    </section>
  );
}
