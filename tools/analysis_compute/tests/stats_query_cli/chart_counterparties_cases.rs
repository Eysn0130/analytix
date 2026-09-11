use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::json;
use std::path::Path;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_counterparties_cli_returns_dashboard_counterparties_payload() -> Result<()> {
    let db = TempDb::new("chart-counterparties")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-counterparties",
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

    let counterparties = &output["counterparties"];
    assert_eq!(counterparties["metric_mode"], json!("amount"));
    assert_eq!(counterparties["direction_mode"], json!("all"));
    assert_eq!(counterparties["default_view"], json!("name"));
    assert_eq!(counterparties["top10_concentration"], json!(100.0));
    assert_eq!(
        counterparties["views"]["name"]["all_items"],
        json!([
            {
                "label": "对手甲",
                "bank": "测试银行",
                "name": "对手甲",
                "account": "CP-001",
                "in_amount": 10000.0,
                "out_amount": 0.0,
                "net_amount": 10000.0,
                "total_amount": 10000.0,
                "in_count": 1,
                "out_count": 0,
                "total_count": 1,
                "value": 10000.0
            },
            {
                "label": "对手乙",
                "bank": "测试银行",
                "name": "对手乙",
                "account": "CP-002",
                "in_amount": 0.0,
                "out_amount": 3000.0,
                "net_amount": -3000.0,
                "total_amount": 3000.0,
                "in_count": 0,
                "out_count": 1,
                "total_count": 1,
                "value": 3000.0
            }
        ])
    );
    assert_eq!(
        counterparties["views"]["account"]["top_items"][0]["label"],
        json!("CP-001")
    );
    assert_eq!(
        counterparties["views"]["bank"]["top_items"][0]["label"],
        json!("测试银行")
    );
    Ok(())
}

#[test]
fn query_chart_counterparties_cli_counts_distinct_counterparties_per_view_bucket() -> Result<()> {
    let db = TempDb::new("chart-counterparties-counterparty")?;
    seed_stats_query_tables(db.path())?;
    insert_same_bank_duplicate_counterparty(db.path().to_string_lossy().as_ref())?;

    let output = run_analysis_compute_command(&[
        "query-chart-counterparties",
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

    let bank_item = &output["counterparties"]["views"]["bank"]["all_items"][0];
    assert_eq!(bank_item["label"], json!("测试银行"));
    assert_eq!(bank_item["in_count"], json!(2));
    assert_eq!(bank_item["out_count"], json!(1));
    assert_eq!(bank_item["value"], json!(2.0));
    Ok(())
}

fn insert_same_bank_duplicate_counterparty(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for extra counterparty row")?;
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
