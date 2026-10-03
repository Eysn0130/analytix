import { FileText } from "../../../../../design/AnalytixUiIcons";
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
              <FileText aria-hidden="true" />
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
