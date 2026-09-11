mod filters;
mod request;
mod sql_builder;

use anyhow::{Context, Result};
use serde_json::Value;

use super::amount_coverage::require_complete_aggregate_amount_coverage;
use super::args::QueryFlowFocusRowsArgs;
use super::session::VerifiedStatsQuerySession;
use super::values::{
    query_flow_focus_row_values, query_flow_focus_rows_typed as query_typed_values, FlowFocusRow,
};
use filters::build_flow_focus_where_sql;
use request::FlowFocusRowsRequest;
use sql_builder::build_flow_focus_sql;

pub(crate) fn query_flow_focus_rows(args: &QueryFlowFocusRowsArgs) -> Result<Vec<Value>> {
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let sql = flow_focus_query_sql(args, &mut session)?;
    let rows = query_flow_focus_row_values(session.conn(), &sql)?;
    session.commit()?;
    Ok(rows)
}

pub(in crate::stats_query_store) fn query_flow_focus_rows_typed_with_session(
    args: &QueryFlowFocusRowsArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
) -> Result<Vec<FlowFocusRow>> {
    let sql = flow_focus_query_sql(args, session)?;
    query_typed_values(session.conn(), &sql)
}

fn flow_focus_query_sql(
    args: &QueryFlowFocusRowsArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
) -> Result<String> {
    let request = FlowFocusRowsRequest::from_args(args)?;
    if request.should_return_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    require_complete_aggregate_amount_coverage(session.conn())?;

    let where_sql = build_flow_focus_where_sql(&request);
    let sql = build_flow_focus_sql(&where_sql, request.include_missing_counterparty);
    session.require_nonempty_query("flow_focus_rows", &sql)?;
    Ok(sql)
}
