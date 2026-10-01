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
