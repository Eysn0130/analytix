mod filters;
mod group_sql;
mod pagination_sql;
mod request;

use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::Value;
use std::io::Write;
use std::time::Instant;

use super::args::QueryStatsRowsArgs;
use super::perf::StatsQueryDiagnostics;
use super::session::VerifiedStatsQuerySession;
use super::values::{
    query_stats_row_values, stats_rows_json_output_spec, stats_rows_sort_expr,
    write_stats_row_fields_json, write_stats_row_values_json, StatsRowsJsonRowFormat,
};
use filters::{build_search_where_sql, build_source_where_sql};
use group_sql::build_stats_rows_agg_sql;
use pagination_sql::{
    build_paginated_count_sql, build_paginated_data_sql, build_unpaginated_data_sql,
};
use request::QueryStatsRowsRequest;

fn require_nonnegative_total(total: i64) -> Result<i64> {
    if total < 0 {
        anyhow::bail!("stats_query_numeric_state_invalid");
    }
    Ok(total)
}

pub(crate) struct QueryStatsRowsResult {
    pub(crate) group_key: String,
    pub(crate) total: i64,
    pub(crate) rows: Vec<Value>,
    pub(crate) row_fields: Option<Value>,
    pub(crate) row_summary: Value,
    pub(crate) diagnostics: Value,
}

pub(crate) struct QueryStatsRowsJsonWriteResult {
    pub(crate) group_key: String,
    pub(crate) total: i64,
    pub(crate) row_count: usize,
    pub(crate) diagnostics: Value,
}

#[derive(Clone, Copy)]
pub(super) struct StatsRowsQueryMetadata {
    has_account_dim: bool,
    has_doc_status: bool,
}

struct StatsRowsQueryContext<'session, 'conn> {
    session: &'session mut VerifiedStatsQuerySession<'conn>,
    request: QueryStatsRowsRequest,
    agg_sql: String,
    search_where: String,
    sort_expr: String,
    sort_order: &'static str,
    diagnostics: StatsQueryDiagnostics,
}

impl<'session, 'conn> StatsRowsQueryContext<'session, 'conn> {
    fn new(
        session: &'session mut VerifiedStatsQuerySession<'conn>,
        request: QueryStatsRowsRequest,
        metadata: StatsRowsQueryMetadata,
        mut diagnostics: StatsQueryDiagnostics,
    ) -> Result<Self> {
        super::amount_coverage::require_complete_detail_amount_coverage(session.conn())?;
        let stage_started = Instant::now();
        let where_sql = build_source_where_sql(&request);
        diagnostics.record_elapsed("sql.source_filter_build", stage_started);

        let stage_started = Instant::now();
        let agg_sql = build_stats_rows_agg_sql(
            &request,
            &where_sql,
            metadata.has_account_dim,
            metadata.has_doc_status,
        );
        let search_where = build_search_where_sql(&request.search_text);
        let sort_expr = stats_rows_sort_expr(&request.row_sort_col)
            .unwrap_or_else(|| request.default_sort_expr())
            .to_string();
        let sort_order = request.sort_order_sql();
        let matched_group_count = crate::scalar_i64(
            session.conn(),
            &build_paginated_count_sql(&agg_sql, &search_where),
        )
        .context("stats_rows.aggregate_count")?;
        if matched_group_count <= 0 {
            anyhow::bail!("stats_query_scope_empty");
        }
        if request.uses_pagination() && request.row_offset >= matched_group_count {
            anyhow::bail!("stats_query_scope_empty");
        }
        diagnostics.record_elapsed("sql.aggregate_search_sort_build", stage_started);

        Ok(Self {
            session,
            request,
            agg_sql,
            search_where,
            sort_expr,
            sort_order,
            diagnostics,
        })
    }

    fn query_rows(mut self, total_started: Instant) -> Result<QueryStatsRowsResult> {
        let output_spec =
            stats_rows_json_output_spec(&self.request.row_format, &self.request.fields);
        let row_fields = if output_spec.row_format == StatsRowsJsonRowFormat::Array {
            Some(serde_json::to_value(&output_spec.fields)?)
        } else {
            None
        };
        if self.request.uses_pagination() {
            let stage_started = Instant::now();
            let data_sql = self.paginated_data_sql();
            self.diagnostics
                .record_elapsed("sql.paginated_data_build", stage_started);
            let stage_started = Instant::now();
            let result = query_stats_row_values(
                self.session.conn(),
                &data_sql,
                self.request.group_key,
                true,
                &output_spec,
            )
            .context("stats_rows.page_values")?;
            self.diagnostics
                .record_elapsed("sql.query_rows_values", stage_started);
            self.session
                .record_nonempty_range(
                    "stats_rows_page",
                    &data_sql,
                    i64::try_from(result.rows.len())
                        .context("stats_query_numeric_state_invalid")?,
                )
                .context("stats_rows.page_nonempty")?;
            self.diagnostics.finish_total(total_started);
            return Ok(QueryStatsRowsResult {
                group_key: self.request.group_key.to_string(),
                total: result.total,
                rows: result.rows,
                row_fields,
                row_summary: result.row_summary,
                diagnostics: self.diagnostics.to_json(),
            });
        }

        let stage_started = Instant::now();
        let data_sql = self.unpaginated_data_sql();
        self.diagnostics
            .record_elapsed("sql.unpaginated_data_build", stage_started);
        let stage_started = Instant::now();
        let result = query_stats_row_values(
            self.session.conn(),
            &data_sql,
            self.request.group_key,
            false,
            &output_spec,
        )?;
        self.diagnostics
            .record_elapsed("sql.query_rows_values", stage_started);
        let total =
            i64::try_from(result.rows.len()).context("stats_query_numeric_state_invalid")?;
        self.session
            .record_nonempty_range("stats_rows", &data_sql, total)
            .context("stats_rows.nonempty")?;
        self.diagnostics.finish_total(total_started);
        Ok(QueryStatsRowsResult {
            group_key: self.request.group_key.to_string(),
            total,
            rows: result.rows,
            row_fields,
            row_summary: result.row_summary,
            diagnostics: self.diagnostics.to_json(),
        })
    }

    fn write_json_payload<W: Write>(
        self,
        writer: &mut W,
        total_started: Instant,
    ) -> Result<QueryStatsRowsJsonWriteResult> {
        writer.write_all(b"{")?;
        let result = self.write_json_fields(writer, total_started, true)?;
        writer.write_all(b"}")?;
        Ok(result)
    }

    fn write_json_fields<W: Write>(
        mut self,
        writer: &mut W,
        total_started: Instant,
        include_status: bool,
    ) -> Result<QueryStatsRowsJsonWriteResult> {
        let output_spec =
            stats_rows_json_output_spec(&self.request.row_format, &self.request.fields);
        let data_sql = if self.request.uses_pagination() {
            self.paginated_data_sql()
        } else {
            self.unpaginated_data_sql()
        };
        let stage_started = Instant::now();
        writer.write_all(b"\"rows\":")?;
        self.diagnostics
            .record_elapsed("json.write_payload_prefix", stage_started);
        let (total, write_result) = if self.request.uses_pagination() {
            let stage_started = Instant::now();
            self.diagnostics
                .record_elapsed("sql.paginated_data_build", stage_started);
            let stage_started = Instant::now();
            let write_result = write_stats_row_values_json(
                self.session.conn(),
                &data_sql,
                self.request.group_key,
                true,
                &output_spec,
                writer,
                &mut self.diagnostics,
            )
            .context("stats_rows.page_output")?;
            self.diagnostics
                .record_elapsed("sql.query_rows_json_write", stage_started);
            (write_result.total, write_result)
        } else {
            let stage_started = Instant::now();
            self.diagnostics
                .record_elapsed("sql.unpaginated_data_build", stage_started);
            let stage_started = Instant::now();
            let write_result = write_stats_row_values_json(
                self.session.conn(),
                &data_sql,
                self.request.group_key,
                false,
                &output_spec,
                writer,
                &mut self.diagnostics,
            )?;
            self.diagnostics
                .record_elapsed("sql.query_rows_json_write", stage_started);
            (
                i64::try_from(write_result.row_count)
                    .context("stats_query_numeric_state_invalid")?,
                write_result,
            )
        };
        // Bind the receipt to the rows actually read in this verified snapshot.
        // Counting a wrapper of the same window/page SQL executes a redundant
        // plan and has failed inside DuckDB before result decoding in Linux CI.
        self.session
            .record_nonempty_range(
                if self.request.uses_pagination() {
                    "stats_rows_page"
                } else {
                    "stats_rows"
                },
                &data_sql,
                i64::try_from(write_result.row_count)
                    .context("stats_query_numeric_state_invalid")?,
            )
            .context("stats_rows.output_nonempty")?;
        let total = require_nonnegative_total(total)?;
        let stage_started = Instant::now();
        if include_status {
            writer.write_all(b",\"status\":\"\"")?;
        }
        writer.write_all(b",\"total\":")?;
        write!(writer, "{total}")?;
        writer.write_all(b",\"row_summary\":")?;
        serde_json::to_writer(&mut *writer, &write_result.row_summary)?;
        if output_spec.row_format == StatsRowsJsonRowFormat::Array {
            writer.write_all(b",\"row_fields\":")?;
            write_stats_row_fields_json(writer, &output_spec)?;
        }
        self.diagnostics
            .record_elapsed("json.write_payload_tail", stage_started);
        self.diagnostics.finish_total(total_started);

        Ok(QueryStatsRowsJsonWriteResult {
            group_key: self.request.group_key.to_string(),
            total,
            row_count: write_result.row_count,
            diagnostics: self.diagnostics.to_json(),
        })
    }

    fn paginated_data_sql(&self) -> String {
        build_paginated_data_sql(
            &self.agg_sql,
            &self.search_where,
            &self.sort_expr,
            self.sort_order,
            self.request.limit_sql(),
            self.request.row_offset,
        )
    }

    fn unpaginated_data_sql(&self) -> String {
        build_unpaginated_data_sql(
            &self.agg_sql,
            &self.search_where,
            &self.sort_expr,
            self.sort_order,
        )
    }
}

#[cfg(test)]
mod tests {
    use super::require_nonnegative_total;

    #[test]
    fn negative_total_fails_closed_instead_of_becoming_zero() {
        assert_eq!(require_nonnegative_total(0).expect("zero total"), 0);
        let error = require_nonnegative_total(-1).expect_err("negative total must fail closed");
        assert_eq!(error.to_string(), "stats_query_numeric_state_invalid");
    }
}

impl StatsRowsQueryMetadata {
    pub(super) fn load(conn: &Connection) -> Result<Self> {
        crate::ensure_required_table(conn, crate::AGG_TABLE)?;
        crate::ensure_required_table(conn, crate::DETAIL_TABLE)?;
        Self::load_dimensions(conn)
    }

    fn load_dimensions(conn: &Connection) -> Result<Self> {
        Ok(Self {
            has_account_dim: crate::table_exists(conn, crate::ACCOUNT_DIM_TABLE)?,
            has_doc_status: crate::table_exists(conn, "analysis_doc_status")?,
        })
    }
}

pub(crate) fn query_stats_rows(args: &QueryStatsRowsArgs) -> Result<QueryStatsRowsResult> {
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let request = QueryStatsRowsRequest::from_args(args);
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.selected.is_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let stage_started = Instant::now();
    let metadata = StatsRowsQueryMetadata::load_dimensions(session.conn())?;
    diagnostics.record_elapsed("db.query_metadata", stage_started);
    let result = StatsRowsQueryContext::new(&mut session, request, metadata, diagnostics)?
        .query_rows(total_started)?;
    session.commit()?;
    Ok(result)
}

pub(super) fn query_stats_rows_json_fields_with_session<W, F>(
    args: &QueryStatsRowsArgs,
    session: &mut VerifiedStatsQuerySession<'_>,
    mut diagnostics: StatsQueryDiagnostics,
    total_started: Instant,
    writer: &mut W,
    load_metadata: F,
) -> Result<QueryStatsRowsJsonWriteResult>
where
    W: Write,
    F: FnOnce(&Connection) -> Result<StatsRowsQueryMetadata>,
{
    let stage_started = Instant::now();
    let request = QueryStatsRowsRequest::from_args(args);
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.selected.is_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let metadata = load_metadata(session.conn())?;
    diagnostics.record_elapsed("db.query_metadata", stage_started);
    StatsRowsQueryContext::new(session, request, metadata, diagnostics)?.write_json_fields(
        writer,
        total_started,
        false,
    )
}

pub(crate) fn query_stats_rows_to_json_writer<W: Write>(
    args: &QueryStatsRowsArgs,
    writer: &mut W,
) -> Result<QueryStatsRowsJsonWriteResult> {
    let total_started = Instant::now();
    let mut diagnostics = StatsQueryDiagnostics::new();
    let stage_started = Instant::now();
    let request = QueryStatsRowsRequest::from_args(args);
    diagnostics.record_elapsed("request.normalize", stage_started);
    if request.selected.is_empty() {
        anyhow::bail!("stats_query_scope_required");
    }

    let stage_started = Instant::now();
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    diagnostics.record_elapsed("db.open_config", stage_started);
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let stage_started = Instant::now();
    let metadata = StatsRowsQueryMetadata::load_dimensions(session.conn())?;
    diagnostics.record_elapsed("db.query_metadata", stage_started);
    let result = StatsRowsQueryContext::new(&mut session, request, metadata, diagnostics)?
        .write_json_payload(writer, total_started)?;
    session.commit()?;
    Ok(result)
}

#[cfg(test)]
mod plan_probe_tests;
