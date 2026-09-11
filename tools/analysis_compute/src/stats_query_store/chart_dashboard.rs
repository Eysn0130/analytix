use anyhow::{bail, Context, Result};
use serde_json::{json, Value};

use super::args::{
    QueryChartCounterpartiesArgs, QueryChartDashboardArgs, QueryChartDistributionArgs,
    QueryChartFlowArgs, QueryChartHeatmapArgs, QueryChartSummaryArgs, QueryChartTrendArgs,
};
use super::chart_anomaly::query_chart_anomaly_with_session;
use super::chart_common::chart_filters_supported_by_native_dashboard;
use super::chart_counterparties::query_chart_counterparties_with_session;
use super::chart_distribution::query_chart_distribution_with_session;
use super::chart_flow::query_chart_flow_with_session;
use super::chart_heatmap::query_chart_heatmap_with_session;
use super::chart_structure::query_chart_structure_with_session;
use super::chart_summary::query_chart_summary_with_session;
use super::chart_trend::query_chart_trend_with_session;
use super::session::VerifiedStatsQuerySession;

pub(crate) fn query_chart_dashboard(args: &QueryChartDashboardArgs) -> Result<Value> {
    if !chart_filters_supported_by_native_dashboard(&args.chart_filters) {
        bail!("unsupported chart filter for native dashboard");
    }
    let chart_args = DashboardChartArgs::from_dashboard_args(args);
    if !args.large_txn_threshold.is_finite() || args.large_txn_threshold < 0.0 {
        bail!("--large-txn-threshold must be finite and non-negative");
    }
    if chart_args.is_empty_selection() {
        bail!("stats_query_scope_required");
    }

    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = chart_args.query_with_session(&mut session)?;
    session.commit()?;
    Ok(result)
}

struct DashboardChartArgs {
    summary: QueryChartSummaryArgs,
    trend: QueryChartTrendArgs,
    counterparties: QueryChartCounterpartiesArgs,
    heatmap: QueryChartHeatmapArgs,
    distribution: QueryChartDistributionArgs,
    flow: QueryChartFlowArgs,
}

impl DashboardChartArgs {
    fn from_dashboard_args(args: &QueryChartDashboardArgs) -> Self {
        let selected_keys = args.selected_keys.clone();
        let common_case_id = args.case_id.clone();
        let common_db_path = args.db_path.clone();
        let date_start = args.date_start.clone();
        let date_end = args.date_end.clone();
        let metric_mode = args.metric_mode.clone();
        let direction_mode = args.direction_mode.clone();
        let success_filter = args.success_filter.clone();
        let cash_filter = args.cash_filter.clone();
        let selection_mode = args.selection_mode.clone();
        let chart_filters = args.chart_filters.clone();

        Self {
            summary: QueryChartSummaryArgs {
                case_id: common_case_id.clone(),
                db_path: common_db_path.clone(),
                selected_keys: selected_keys.clone(),
                date_start: date_start.clone(),
                date_end: date_end.clone(),
                success_filter: success_filter.clone(),
                cash_filter: cash_filter.clone(),
                selection_mode: selection_mode.clone(),
                large_txn_threshold: args.large_txn_threshold,
                chart_filters: chart_filters.clone(),
            },
            trend: QueryChartTrendArgs {
                case_id: common_case_id.clone(),
                db_path: common_db_path.clone(),
                selected_keys: selected_keys.clone(),
                date_start: date_start.clone(),
                date_end: date_end.clone(),
                metric_mode: metric_mode.clone(),
                direction_mode: direction_mode.clone(),
                granularity: args.granularity.clone(),
                success_filter: success_filter.clone(),
                cash_filter: cash_filter.clone(),
                selection_mode: selection_mode.clone(),
                chart_filters: chart_filters.clone(),
            },
            counterparties: QueryChartCounterpartiesArgs {
                case_id: common_case_id.clone(),
                db_path: common_db_path.clone(),
                selected_keys: selected_keys.clone(),
                date_start: date_start.clone(),
                date_end: date_end.clone(),
                metric_mode: metric_mode.clone(),
                direction_mode: direction_mode.clone(),
                success_filter: success_filter.clone(),
                cash_filter: cash_filter.clone(),
                chart_filters: chart_filters.clone(),
            },
            heatmap: QueryChartHeatmapArgs {
                case_id: common_case_id.clone(),
                db_path: common_db_path.clone(),
                selected_keys: selected_keys.clone(),
                date_start: date_start.clone(),
                date_end: date_end.clone(),
                metric_mode: metric_mode.clone(),
                direction_mode: direction_mode.clone(),
                success_filter: success_filter.clone(),
                cash_filter: cash_filter.clone(),
                chart_filters: chart_filters.clone(),
            },
            distribution: QueryChartDistributionArgs {
                case_id: common_case_id.clone(),
                db_path: common_db_path.clone(),
                selected_keys: selected_keys.clone(),
                date_start: date_start.clone(),
                date_end: date_end.clone(),
                metric_mode,
                direction_mode: direction_mode.clone(),
                success_filter: success_filter.clone(),
                cash_filter: cash_filter.clone(),
                selection_mode: selection_mode.clone(),
                chart_filters: chart_filters.clone(),
            },
            flow: QueryChartFlowArgs {
                case_id: common_case_id,
                db_path: common_db_path,
                selected_keys,
                date_start,
                date_end,
                direction: direction_mode,
                success_filter,
                cash_filter,
                selection_mode,
                chart_filters,
            },
        }
    }

    fn is_empty_selection(&self) -> bool {
        !self
            .summary
            .selected_keys
            .iter()
            .any(|item| !item.trim().is_empty())
    }

    fn query_with_session(&self, session: &mut VerifiedStatsQuerySession<'_>) -> Result<Value> {
        Ok(json!({
            "summary": query_chart_summary_with_session(session, &self.summary)?,
            "trend": query_chart_trend_with_session(session, &self.trend)?,
            "counterparties": query_chart_counterparties_with_session(session, &self.counterparties)?,
            "structure": query_chart_structure_with_session(session, &self.as_dashboard_args())?,
            "heatmap": query_chart_heatmap_with_session(session, &self.heatmap)?,
            "distribution": query_chart_distribution_with_session(session, &self.distribution)?,
            "flow": query_chart_flow_with_session(session, &self.flow)?,
            "anomaly": query_chart_anomaly_with_session(session, &self.as_dashboard_args())?,
        }))
    }

    fn as_dashboard_args(&self) -> QueryChartDashboardArgs {
        QueryChartDashboardArgs {
            case_id: self.summary.case_id.clone(),
            db_path: self.summary.db_path.clone(),
            selected_keys: self.summary.selected_keys.clone(),
            date_start: self.summary.date_start.clone(),
            date_end: self.summary.date_end.clone(),
            metric_mode: self.trend.metric_mode.clone(),
            direction_mode: self.trend.direction_mode.clone(),
            granularity: self.trend.granularity.clone(),
            success_filter: self.summary.success_filter.clone(),
            cash_filter: self.summary.cash_filter.clone(),
            selection_mode: self.summary.selection_mode.clone(),
            large_txn_threshold: self.summary.large_txn_threshold,
            chart_filters: self.summary.chart_filters.clone(),
        }
    }
}
