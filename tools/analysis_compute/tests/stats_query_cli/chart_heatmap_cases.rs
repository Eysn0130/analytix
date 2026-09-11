use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::json;
use std::path::Path;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_heatmap_cli_returns_dashboard_heatmap_payload() -> Result<()> {
    let db = TempDb::new("chart-heatmap")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-heatmap",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let heatmap = &output["heatmap"];
    assert_eq!(heatmap["metric_mode"], json!("amount"));
    assert_eq!(heatmap["direction_mode"], json!("all"));
    assert_eq!(
        heatmap["cells"],
        json!([
            {
                "weekday": 2,
                "hour": 9,
                "in_amount": 10000.0,
                "out_amount": 0.0,
                "in_count": 1,
                "out_count": 0,
                "value": 10000.0
            },
            {
                "weekday": 2,
                "hour": 10,
                "in_amount": 0.0,
                "out_amount": 3000.0,
                "in_count": 0,
                "out_count": 1,
                "value": 3000.0
            }
        ])
    );
    assert_eq!(
        heatmap["hours"],
        json!([
            {
                "label": "09:00",
                "in_amount": 10000.0,
                "out_amount": 0.0,
                "in_count": 1,
                "out_count": 0,
                "value": 10000.0
            },
            {
                "label": "10:00",
                "in_amount": 0.0,
                "out_amount": 3000.0,
                "in_count": 0,
                "out_count": 1,
                "value": 3000.0
            }
        ])
    );
    assert_eq!(
        heatmap["weekdays"],
        json!([
            {
                "label": "周三",
                "in_amount": 10000.0,
                "out_amount": 3000.0,
                "in_count": 1,
                "out_count": 1,
                "value": 13000.0
            }
        ])
    );
    Ok(())
}

#[test]
fn query_chart_heatmap_cli_counts_distinct_counterparties_per_projection_bucket() -> Result<()> {
    let db = TempDb::new("chart-heatmap-counterparty")?;
    seed_stats_query_tables(db.path())?;
    insert_duplicate_counterparty_different_hour(db.path().to_string_lossy().as_ref())?;

    let output = run_analysis_compute_command(&[
        "query-chart-heatmap",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "counterparty",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let weekday = &output["heatmap"]["weekdays"][0];
    assert_eq!(weekday["label"], json!("周三"));
    assert_eq!(weekday["in_count"], json!(2));
    assert_eq!(weekday["out_count"], json!(1));
    assert_eq!(weekday["value"], json!(2.0));
    Ok(())
}

fn insert_duplicate_counterparty_different_hour(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for extra heatmap row")?;
    conn.execute_batch(
        "
        INSERT INTO analysis_txn_detail_idx
        SELECT * REPLACE (
          4 AS id, TIMESTAMP '2026-04-01 11:00:00' AS txn_ts,
          '2026-04-01 11:00:00' AS txn_time, 2000.0 AS amount,
          9000.0 AS balance, '二次入账' AS summary, 'txn-rust-in-2' AS txn_id
        ) FROM analysis_txn_detail_idx WHERE id=1;
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(Path::new(path))
}
