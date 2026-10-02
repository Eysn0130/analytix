//! Bounded R-H1 controls. Each case opens a new verified read-only session and
//! executes exactly one of the production aggregate/page/count-page queries.
use super::*;
use sha2::{Digest, Sha256};

#[allow(dead_code)]
#[path = "../../../tests/stats_query_cli/fixtures.rs"]
mod fixtures;
#[path = "../../../tests/stats_query_cli/temp_db.rs"]
mod temp_db;

fn probe(stage: &str) -> Result<()> {
    let db = temp_db::TempDb::new(stage)?;
    fixtures::seed_stats_query_tables(db.path())?;
    let args = super::super::args::parse_query_stats_rows_args(
        [
            "--case-id",
            fixtures::CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--mode",
            "inAccount",
            "--selected-key",
            "CARD-001",
            "--row-sort-col",
            "total_amount",
            "--row-sort-dir",
            "asc",
            "--row-limit",
            "10",
        ]
        .into_iter()
        .map(str::to_owned),
    )?;
    let conn = crate::open_readonly_connection(db.path())?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, &args)?;
    let metadata = StatsRowsQueryMetadata::load_dimensions(session.conn())?;
    let request = QueryStatsRowsRequest::from_args(&args);
    let agg = build_stats_rows_agg_sql(
        &request,
        &build_source_where_sql(&request),
        metadata.has_account_dim,
        metadata.has_doc_status,
    );
    let search = build_search_where_sql(&request.search_text);
    let page = build_paginated_data_sql(
        &agg,
        &search,
        stats_rows_sort_expr(&request.row_sort_col).expect("known sort"),
        request.sort_order_sql(),
        request.limit_sql(),
        request.row_offset,
        2,
    );
    let sql = match stage {
        "aggregate_count" => build_paginated_count_sql(&agg, &search),
        "page" => page,
        "count_page" => format!("SELECT COUNT(1) FROM ({page}) verified_stats_query_range"),
        _ => unreachable!(),
    };
    let settings: Vec<(String, String)> = conn.prepare(
        "SELECT name,value FROM duckdb_settings() WHERE name IN ('threads','access_mode','preserve_insertion_order','enable_external_access','disabled_optimizers','memory_limit') ORDER BY name"
    )?.query_map([], |row| Ok((row.get(0)?, row.get(1)?)))?.collect::<duckdb::Result<_>>()?;
    let version: String = conn.query_row("SELECT version()", [], |row| row.get(0))?;
    eprintln!(
        "R_H1_INPUT {}",
        serde_json::json!({
            "stage": stage, "version": version, "settings": settings, "sql": sql,
            "fixture_sha256": format!("{:x}", Sha256::digest(include_bytes!("../../../tests/stats_query_cli/fixtures.rs"))),
            "parameters": [], "connection": "fresh verified read-only transaction"
        })
    );
    let observed_count = if stage == "page" {
        let rows: Vec<(f64, i64)> = conn
            .prepare(&sql)?
            .query_map([], |row| {
                Ok((row.get::<_, f64>(7)? + row.get::<_, f64>(8)?, row.get(14)?))
            })?
            .collect::<duckdb::Result<_>>()?;
        assert_eq!(rows, vec![(3000.0, 2), (10000.0, 2)]);
        rows.len() as i64
    } else {
        let count = crate::scalar_i64(&conn, &sql)?;
        assert_eq!(count, 2);
        count
    };
    session.record_nonempty_range(stage, &sql, observed_count)?;
    session.commit()?;
    eprintln!("R_H1_RESULT {stage} PASS");
    Ok(())
}

#[test]
fn aggregate_count_fresh_session() -> Result<()> {
    probe("aggregate_count")
}
#[test]
fn page_fresh_session() -> Result<()> {
    probe("page")
}
#[test]
fn count_page_fresh_session() -> Result<()> {
    probe("count_page")
}

// Exact CI search/tie/page shape, isolated from CLI startup. Controls change
// only the test connection's thread count or the redundant window-total column.
fn probe_search_page(control: &str, offset: i64, iterations: usize) -> Result<()> {
    assert!((1..=256).contains(&iterations));
    let db = temp_db::TempDb::new("search-page-control")?;
    fixtures::seed_stats_query_tables(db.path())?;
    let args = super::super::args::parse_query_stats_rows_args(
        [
            "--case-id",
            fixtures::CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--mode",
            "inAccount",
            "--selected-key",
            "CARD-001",
            "--date-start",
            "2026-04-01",
            "--date-end",
            "2026-04-01",
            "--search-text",
            "对手",
            "--row-sort-col",
            "total_count",
            "--row-sort-dir",
            "desc",
            "--row-limit",
            "1",
            "--row-offset",
            &offset.to_string(),
        ]
        .into_iter()
        .map(str::to_owned),
    )?;
    let conn = crate::open_readonly_connection(db.path())?;
    crate::configure_connection(&conn)?;
    if matches!(control, "single_thread" | "legacy_single_thread") {
        conn.execute_batch("SET threads=1")?;
    }
    let mut session = VerifiedStatsQuerySession::begin(&conn, &args)?;
    let metadata = StatsRowsQueryMetadata::load_dimensions(session.conn())?;
    let request = QueryStatsRowsRequest::from_args(&args);
    let agg = build_stats_rows_agg_sql(
        &request,
        &build_source_where_sql(&request),
        metadata.has_account_dim,
        metadata.has_doc_status,
    );
    let search = build_search_where_sql(&request.search_text);
    let total = crate::scalar_i64(session.conn(), &build_paginated_count_sql(&agg, &search))?;
    assert_eq!(total, 2);
    let mut page = build_paginated_data_sql(
        &agg,
        &search,
        stats_rows_sort_expr(&request.row_sort_col).expect("known sort"),
        request.sort_order_sql(),
        request.limit_sql(),
        request.row_offset,
        total,
    );
    match control {
        "window" | "legacy_single_thread" => {
            // Historical diagnostic control only; normal page probes use the
            // production snapshot total and never restore this engine path.
            let window = "CAST(COUNT(1) OVER() AS BIGINT) AS total_count";
            let literal = format!("CAST({total} AS BIGINT) AS total_count");
            assert_eq!(page.matches(&literal).count(), 1);
            page = page.replace(&literal, window);
        }
        "count_literal" | "single_thread" => {}
        _ => unreachable!(),
    }
    let threads: String =
        conn.query_row("SELECT current_setting('threads')::VARCHAR", [], |row| {
            row.get(0)
        })?;
    let version: String = conn.query_row("SELECT version()", [], |row| row.get(0))?;
    let plan: Vec<String> = conn
        .prepare(&format!("EXPLAIN {page}"))?
        .query_map([], |row| row.get(1))?
        .collect::<duckdb::Result<_>>()?;
    let has_window = plan.iter().any(|part| part.contains("WINDOW"));
    assert_eq!(
        has_window,
        matches!(control, "window" | "legacy_single_thread"),
        "single-account page plan must only use a window in the explicit historical control"
    );
    eprintln!(
        "R_H1_SEARCH_INPUT {}",
        serde_json::json!({"control":control,"offset":offset,"iterations":iterations,"has_window":has_window,"threads":threads,"version":version,"fixture_sha256":format!("{:x}",Sha256::digest(include_bytes!("../../../tests/stats_query_cli/fixtures.rs")))})
    );
    // Exercise the actual failed consumer, including every display column and
    // JSON row summary, rather than only reading a subset of SQL columns.
    let spec = stats_rows_json_output_spec(&request.row_format, &request.fields);
    let (key, amount) = match offset {
        0 => ("CP-001", 10000.0),
        1 => ("CP-002", 3000.0),
        _ => unreachable!(),
    };
    for iteration in 0..iterations {
        let result = query_stats_row_values(session.conn(), &page, request.group_key, true, &spec)
            .with_context(|| {
                format!("search control={control} offset={offset} iteration={iteration}")
            })?;
        assert_eq!(result.total, 2);
        assert_eq!(result.rows.len(), 1);
        assert_eq!(
            result.rows[0]["id"],
            serde_json::json!(format!("cp_key:{key}"))
        );
        assert_eq!(result.rows[0]["keyValue"], serde_json::json!(key));
        assert_eq!(result.rows[0]["total_amount"], serde_json::json!(amount));
        assert_eq!(result.rows[0]["total_count"], serde_json::json!(1));
        assert_eq!(
            result.row_summary,
            serde_json::json!({"total_amount":amount,"total_count":1})
        );
    }
    session.record_nonempty_range("search_page_control", &page, 1)?;
    session.commit()?;
    eprintln!("R_H1_SEARCH_RESULT control={control} offset={offset} completed={iterations} PASS");
    Ok(())
}

#[test]
fn search_page_snapshot_total_exact_ci_shape() -> Result<()> {
    for offset in 0..2 {
        probe_search_page("count_literal", offset, 1)
            .with_context(|| format!("snapshot-total control offset={offset}"))?;
    }
    Ok(())
}
#[test]
fn search_page_snapshot_total_single_thread_control() -> Result<()> {
    for offset in 0..2 {
        probe_search_page("single_thread", offset, 1)
            .with_context(|| format!("single-thread control offset={offset}"))?;
    }
    Ok(())
}
#[test]
fn search_page_count_literal_control() -> Result<()> {
    for offset in 0..2 {
        probe_search_page("count_literal", offset, 1)
            .with_context(|| format!("count-literal control offset={offset}"))?;
    }
    Ok(())
}

#[test]
#[ignore = "bounded scheduling diagnostic; never an acceptance substitute"]
fn search_page_bounded_window_scheduling_diagnostic() -> Result<()> {
    // Controls use fresh verified sessions before the suspected path, so an
    // engine-invalidating failure cannot contaminate their results. No retries.
    for control in ["count_literal", "legacy_single_thread", "window"] {
        let iterations = if control == "window" { 256 } else { 32 };
        for offset in 0..2 {
            probe_search_page(control, offset, iterations)?;
        }
    }
    Ok(())
}
