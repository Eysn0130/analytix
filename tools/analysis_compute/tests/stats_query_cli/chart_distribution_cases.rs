use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::json;
use std::path::Path;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_distribution_cli_returns_dashboard_distribution_payload() -> Result<()> {
    let db = TempDb::new("chart-distribution")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-distribution",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let distribution = &output["distribution"];
    assert_eq!(distribution["selection_mode"], json!("single-card"));
    assert_eq!(distribution["default_view"], json!("bank"));
    assert_eq!(
        distribution["quality"],
        json!({
            "bank_non_empty_rate": 100.0,
            "location_non_empty_rate": 100.0,
            "branch_non_empty_rate": 100.0
        })
    );
    assert_eq!(
        distribution["views"]["bank"],
        json!([
            {
                "label": "测试银行",
                "in_amount": 10000.0,
                "out_amount": 3000.0,
                "total_amount": 13000.0,
                "net_amount": 7000.0,
                "in_count": 1,
                "out_count": 1,
                "total_count": 2,
                "value": 13000.0,
                "count": 2
            }
        ])
    );
    assert_eq!(distribution["views"]["location"][0]["label"], json!("贵阳"));
    assert_eq!(
        distribution["views"]["branch"][0]["label"],
        json!("一号支行")
    );
    Ok(())
}

#[test]
fn query_chart_distribution_cli_counts_distinct_counterparties_per_view_bucket() -> Result<()> {
    let db = TempDb::new("chart-distribution-counterparty")?;
    seed_stats_query_tables(db.path())?;
    insert_duplicate_counterparty_same_bank(db.path().to_string_lossy().as_ref())?;

    let output = run_analysis_compute_command(&[
        "query-chart-distribution",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--metric-mode",
        "counterparty",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let bank_item = &output["distribution"]["views"]["bank"][0];
    assert_eq!(bank_item["label"], json!("测试银行"));
    assert_eq!(bank_item["in_count"], json!(2));
    assert_eq!(bank_item["out_count"], json!(1));
    assert_eq!(bank_item["value"], json!(2.0));
    Ok(())
}

fn insert_duplicate_counterparty_same_bank(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for extra distribution row")?;
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
