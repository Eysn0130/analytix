import { ReactNode, useMemo } from "react";

type ChartPanelHeaderMode = "default" | "primary" | "compact";
type ChartPanelActionMode = "full" | "menu" | "none";

interface ChartPanelProps {
  panelId: string;
  title: string;
  subtitle?: string;
  loading?: boolean;
  empty?: boolean;
  emptyText?: string;
  emptyTitle?: string;
  emptyDescription?: string;
  emptyVariant?: "selection" | "filtered";
  className?: string;
  headerControls?: ReactNode;
  headerMode?: ChartPanelHeaderMode;
  actionMode?: ChartPanelActionMode;
  children: ReactNode;
}

export function ChartPanel({
  panelId,
  title,
  subtitle,
  loading = false,
  empty = false,
  emptyText = "暂无数据",
  emptyTitle,
  emptyDescription,
  emptyVariant = "filtered",
  className = "",
  headerControls,
  headerMode = "default",
  children,
}: ChartPanelProps): JSX.Element {
  const panelClassName = useMemo(
    () =>
      [
        "chart-panel",
        empty ? "is-empty" : "",
        loading ? "is-loading" : "",
        `chart-panel--${headerMode}`,
        className,
      ]
        .filter(Boolean)
        .join(" "),
    [className, empty, headerMode, loading]
  );

  return (
    <section className={panelClassName} data-panel-id={panelId}>
      <header className="chart-panel__header">
        <div className="chart-panel__title-wrap">
          <div className="chart-panel__title">{title}</div>
          {subtitle ? <div className="chart-panel__subtitle">{subtitle}</div> : null}
        </div>
        {headerControls ? <div className="chart-panel__header-actions">{headerControls}</div> : null}
      </header>
      <div className="chart-panel__body">
        {loading ? <div className="chart-panel__state">加载中...</div> : null}
        {!loading && empty ? (
          <div className={`chart-panel__empty chart-panel__empty--${emptyVariant}`}>
            <div className="chart-panel__empty-icon" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none">
                <path d="M5 7.5h14" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
                <path d="M7.25 12h9.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
                <path d="M9.25 16.5h5.5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" />
                <rect x="3.75" y="4.75" width="16.5" height="14.5" rx="3.25" stroke="currentColor" strokeWidth="1.5" />
              </svg>
            </div>
            <div className="chart-panel__empty-title">{emptyTitle || emptyText}</div>
            {emptyDescription ? <div className="chart-panel__empty-description">{emptyDescription}</div> : null}
          </div>
        ) : null}
        {!loading && !empty ? children : null}
      </div>
    </section>
  );
}
