mod cursor_filter;
mod filters;
mod request;
mod sql_builder;

use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use std::io::Write;
use std::time::Instant;

use super::args::QueryStatsTxnRowsArgs;
use super::perf::StatsQueryDiagnostics;
use super::session::VerifiedStatsQuerySession;
use super::values::{
    query_stats_txn_row_values, txn_display_row_to_json_value, txn_rows_json_output_spec,
    write_stats_txn_row_values_json, write_txn_row_fields_json, TxnRowsJsonRowFormat,
};
use cursor_filter::push_cursor_filter;
use filters::{base_where_sql, build_base_where_parts};
use request::QueryStatsTxnRowsRequest;
use sql_builder::{
    build_manual_mapping_sql, build_txn_rows_sql, load_manual_mapping_schema,
    TxnRowsManualMappingSchema,
};

pub(crate) struct QueryStatsTxnRowsResult {
    pub(crate) rows: Vec<Value>,
    pub(crate) row_fields: Option<Value>,
    pub(crate) done: Option<bool>,
    pub(crate) next_cursor: Option<Value>,
    pub(crate) diagnostics: Value,
}

pub(crate) struct QueryStatsTxnRowsJsonWriteResult {
    pub(crate) row_count: usize,
    pub(crate) done: Option<bool>,
    pub(crate) diagnostics: Value,
}

#[derive(Clone)]
pub(super) struct StatsTxnRowsQueryMetadata {
    manual_mapping_schema: TxnRowsManualMappingSchema,
    has_account_dim: bool,
}

struct TxnRowsQueryContext<'session, 'conn> {
    session: &'session mut VerifiedStatsQuerySession<'conn>,
    request: QueryStatsTxnRowsRequest,
    sql: String,
    diagnostics: StatsQueryDiagnostics,
}

impl<'session, 'conn> TxnRowsQueryContext<'session, 'conn> {
    fn new(
        args: &QueryStatsTxnRowsArgs,
        session: &'session mut VerifiedStatsQuerySession<'conn>,
        request: QueryStatsTxnRowsRequest,
        metadata: Option<&StatsTxnRowsQueryMetadata>,
        mut diagnostics: StatsQueryDiagnostics,
    ) -> Result<Self> {
        let conn = session.conn();
        super::amount_coverage::require_complete_detail_amount_coverage(conn)?;
        let stage_started = Instant::now();
        let manual_mapping = if let Some(metadata) = metadata {
            metadata.manual_mapping_schema.build_sql(&args.case_id)
        } else {
            build_manual_mapping_sql(&conn, &args.case_id)?
        };
        let has_account_dim = if let Some(metadata) = metadata {
            metadata.has_account_dim
        } else {
            crate::table_exists(conn, crate::ACCOUNT_DIM_TABLE)?
        };
        diagnostics.record_elapsed("sql.manual_mapping_build", stage_started);

        let stage_started = Instant::now();
        let mut where_parts = build_base_where_parts(&request);
        diagnostics.record_elapsed("sql.base_filter_build", stage_started);

        let stage_started = Instant::now();
        push_cursor_filter(&mut where_parts, &request);
        diagnostics.record_elapsed("sql.cursor_filter_build", stage_started);

        let stage_started = Instant::now();
        let sql = build_txn_rows_sql(
            &base_where_sql(&where_parts),
            &request.order_sql(),
            request.limit,
            &manual_mapping,
            request.selected.len() > 1 && has_account_dim,
        );
        diagnostics.record_elapsed("sql.txn_rows_build", stage_started);

        Ok(Self {
            session,
            request,
            sql,
            diagnostics,
        })
    }

    fn query_rows(mut self, total_started: Instant) -> Result<QueryStatsTxnRowsResult> {
        self.session
            .require_nonempty_query("stats_txn_rows_page", &self.sql)?;
        let stage_started = Instant::now();
        let rows_with_cursor = query_stats_txn_row_values(self.session.conn(), &self.sql)?;
        self.diagnostics
            .record_elapsed("sql.query_txn_values", stage_started);

        let stage_started = Instant::now();
        let output_spec = txn_rows_json_output_spec(&self.request.row_format, &self.request.fields);
        let rows = rows_with_cursor
            .iter()
            .map(|item| txn_display_row_to_json_value(&item.row, &output_spec))
            .collect::<Vec<_>>();
        let row_fields = if output_spec.row_format == TxnRowsJsonRowFormat::Array {
            Some(serde_json::to_value(output_spec.fields())?)
        } else {
            None
        };
        self.diagnostics
            .record_elapsed("json.rows_to_values", stage_started);
        let Some(done) = done_for_limit(self.request.limit, rows.len()) else {
            self.diagnostics.finish_total(total_started);
            return Ok(QueryStatsTxnRowsResult {
                rows,
                row_fields,
                done: None,
                next_cursor: None,
                diagnostics: self.diagnostics.to_json(),
            });
        };

        let stage_started = Instant::now();
        let next_cursor = rows_with_cursor.last().map(|last| {
            next_cursor_value(&self.request, last.id, last.amount, last.txn_ts.as_ref())
        });
        self.diagnostics
            .record_elapsed("cursor.next_cursor_build", stage_started);
        self.diagnostics.finish_total(total_started);
        Ok(QueryStatsTxnRowsResult {
            rows,
            row_fields,
            done: Some(done),
            next_cursor,
            diagnostics: self.diagnostics.to_json(),
        })
    }

    fn write_json_payload<W: Write>(
        self,
        writer: &mut W,
        total_started: Instant,
    ) -> Result<QueryStatsTxnRowsJsonWriteResult> {
        writer.write_all(b"{")?;
        let result = self.write_json_fields(writer, total_started, b"next_cursor")?;
        writer.write_all(b"}")?;
        Ok(result)
    }

    fn write_json_fields<W: Write>(
        mut self,
        writer: &mut W,
        total_started: Instant,
        next_cursor_key: &[u8],
    ) -> Result<QueryStatsTxnRowsJsonWriteResult> {
        self.session
            .require_nonempty_query("stats_txn_rows_page", &self.sql)?;
        let stage_started = Instant::now();
        let output_spec = txn_rows_json_output_spec(&self.request.row_format, &self.request.fields);
        writer.write_all(b"\"rows\":")?;
        self.diagnostics
            .record_elapsed("json.write_payload_prefix", stage_started);
        let stage_started = Instant::now();
        let write_result = write_stats_txn_row_values_json(
            self.session.conn(),
            &self.sql,
            writer,
            &mut self.diagnostics,
            &output_spec,
        )?;
        self.diagnostics
            .record_elapsed("sql.query_txn_json_write", stage_started);
        if output_spec.row_format == TxnRowsJsonRowFormat::Array {
            let stage_started = Instant::now();
            writer.write_all(b",\"row_fields\":")?;
            write_txn_row_fields_json(writer, &output_spec)?;
            self.diagnostics
                .record_elapsed("json.write_row_fields", stage_started);
        }
        let done = done_for_limit(self.request.limit, write_result.row_count);
        if let Some(done) = done {
            let stage_started = Instant::now();
            let next_cursor = if write_result.row_count == 0 {
                Value::Null
            } else {
                next_cursor_value(
                    &self.request,
                    write_result.last_id,
                    write_result.last_amount,
                    write_result.last_txn_ts.as_ref(),
                )
            };
            self.diagnostics
                .record_elapsed("cursor.next_cursor_build", stage_started);
            let stage_started = Instant::now();
            writer.write_all(b",\"done\":")?;
            write!(writer, "{done}")?;
            writer.write_all(b",\"")?;
            writer.write_all(next_cursor_key)?;
            writer.write_all(b"\":")?;
            serde_json::to_writer(&mut *writer, &next_cursor)?;
            self.diagnostics
                .record_elapsed("json.write_cursor_fields", stage_started);
        }
        self.diagnostics.finish_total(total_started);

        Ok(QueryStatsTxnRowsJsonWriteResult {
            row_count: write_result.row_count,
            done,
            diagnostics: self.diagnostics.to_json(),
        })
    }
}

impl StatsTxnRowsQueryMetadata {
    pub(super) fn load(conn: &Connection) -> Result<Self> {
        crate::ensure_required_table(conn, crate::DETAIL_TABLE)?;
        Ok(Self {
            manual_mapping_schema: load_manual_mapping_schema(conn)?,
            has_account_dim: crate::table_exists(conn, crate::ACCOUNT_DIM_TABLE)?,
        })
    }
}

pub(crate) fn query_stats_txn_rows(
    args: &QueryStatsTxnRowsArgs,
) -> Result<QueryStatsTxnRowsResult> {
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let request = QueryStatsTxnRowsRequest::from_args(args)?;
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.should_return_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = TxnRowsQueryContext::new(args, &mut session, request, None, diagnostics)?
        .query_rows(total_started)?;
    session.commit()?;
    Ok(result)
}

pub(super) fn query_stats_txn_rows_json_fields_with_session<W, F>(
    args: &QueryStatsTxnRowsArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
    mut diagnostics: StatsQueryDiagnostics,
    total_started: Instant,
    writer: &mut W,
    load_metadata: F,
) -> Result<QueryStatsTxnRowsJsonWriteResult>
where
    W: Write,
    F: FnOnce(&Connection) -> Result<StatsTxnRowsQueryMetadata>,
{
    let stage_started = Instant::now();
    let request = QueryStatsTxnRowsRequest::from_args(args)?;
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.should_return_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let metadata = load_metadata(session.conn())?;
    diagnostics.record_elapsed("db.query_metadata", stage_started);
    TxnRowsQueryContext::new(args, session, request, Some(&metadata), diagnostics)?
        .write_json_fields(writer, total_started, b"nextCursor")
}

pub(crate) fn query_stats_txn_rows_to_json_writer<W: Write>(
    args: &QueryStatsTxnRowsArgs,
    writer: &mut W,
) -> Result<QueryStatsTxnRowsJsonWriteResult> {
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let request = QueryStatsTxnRowsRequest::from_args(args)?;
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.should_return_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = TxnRowsQueryContext::new(args, &mut session, request, None, diagnostics)?
        .write_json_payload(writer, total_started)?;
    session.commit()?;
    Ok(result)
}

fn done_for_limit(limit: i64, row_count: usize) -> Option<bool> {
    if limit > 0 {
        Some(row_count < limit as usize)
    } else {
        None
    }
}

fn next_cursor_value(
    request: &QueryStatsTxnRowsRequest,
    id: i64,
    amount: Option<f64>,
    txn_ts: Option<&String>,
) -> Value {
    if request.sort_col == "amount" {
        return match amount.filter(|value| value.is_finite()) {
            Some(amount) => json!({"id": id, "abs": amount.abs()}),
            None => json!({"id": id, "absNull": true}),
        };
    } else {
        let ts = txn_ts.cloned().unwrap_or_default();
        json!({
            "id": id,
            "ts": ts,
            "tsNull": if txn_ts.is_none() { 1 } else { 0 },
        })
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    fn amount_request() -> QueryStatsTxnRowsRequest {
        QueryStatsTxnRowsRequest::from_args(&QueryStatsTxnRowsArgs {
            case_id: "case-1".to_string(),
            db_path: PathBuf::from("case.duckdb"),
            key_type: "account".to_string(),
            key_value: "CARD-001".to_string(),
            key_values: Vec::new(),
            selected_keys: Vec::new(),
            date_start: String::new(),
            date_end: String::new(),
            start_time: String::new(),
            end_time: String::new(),
            direction: "all".to_string(),
            sort_col: "amount".to_string(),
            sort_dir: "asc".to_string(),
            limit: 1,
            cursor: None,
            row_format: "object".to_string(),
            fields: Vec::new(),
            output_json: None,
        })
        .expect("amount request")
    }

    #[test]
    fn amount_cursor_keeps_verified_zero_distinct_from_missing_state() {
        let request = amount_request();

        assert_eq!(
            next_cursor_value(&request, 7, Some(0.0), None),
            json!({"id": 7, "abs": 0.0})
        );
        assert_eq!(
            next_cursor_value(&request, 8, None, None),
            json!({"id": 8, "absNull": true})
        );
        assert_eq!(
            next_cursor_value(&request, 9, Some(f64::NAN), None),
            json!({"id": 9, "absNull": true})
        );
    }
}
