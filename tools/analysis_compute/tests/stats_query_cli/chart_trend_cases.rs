use anyhow::Result;
use duckdb::Connection;
use serde_json::json;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_trend_cli_returns_dashboard_trend_payload() -> Result<()> {
    let db = TempDb::new("chart-trend")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-trend",
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
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let trend = &output["trend"];
    assert_eq!(trend["granularity"], json!("day"));
    assert_eq!(trend["metric_mode"], json!("amount"));
    assert_eq!(trend["direction_mode"], json!("all"));
    assert_eq!(
        trend["points"],
        json!([
            {
                "bucket": "2026-04-01",
                "label": "2026-04-01",
                "in_amount": 10000.0,
                "out_amount": 3000.0,
                "net_amount": 7000.0,
                "total_amount": 13000.0,
                "in_count": 1,
                "out_count": 1,
                "net_count": 0,
                "total_count": 2,
                "counterparty_count": 2,
                "latest_balance": 7000.0,
                "value": 13000.0
            }
        ])
    );
    assert_eq!(
        trend["balance_points"],
        json!([{"bucket": "2026-04-01", "label": "2026-04-01", "balance": 7000.0}])
    );
    assert_eq!(
        trend["cumulative_net_points"],
        json!([{"bucket": "2026-04-01", "label": "2026-04-01", "value": 7000.0}])
    );
    assert_eq!(trend["balance_available"], json!(true));
    assert_eq!(trend["balance_mode"], json!("balance"));
    assert_eq!(trend["balance_coverage_rate"], json!(100.0));
    assert_eq!(
        trend["balance_markers"],
        json!([
            {
                "txn_id": "txn-rust-in",
                "txn_time": "2026-04-01 09:00:00",
                "bucket": "2026-04-01",
                "label": "2026-04-01",
                "amount": 10000.0,
                "balance": 10000.0,
                "balance_delta": null,
                "direction": "in",
                "counterparty": "对手甲",
                "summary": "工资入账"
            },
            {
                "txn_id": "txn-rust-out",
                "txn_time": "2026-04-01 10:00:00",
                "bucket": "2026-04-01",
                "label": "2026-04-01",
                "amount": 3000.0,
                "balance": 7000.0,
                "balance_delta": -3000.0,
                "direction": "out",
                "counterparty": "对手乙",
                "summary": "转出"
            }
        ])
    );
    Ok(())
}

#[test]
fn query_chart_trend_cli_keeps_direction_as_metric_projection() -> Result<()> {
    let db = TempDb::new("chart-trend-direction")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-trend",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--metric-mode",
        "count",
        "--direction-mode",
        "out",
        "--granularity",
        "day",
        "--success-filter",
        "success",
        "--cash-filter",
        "non-cash",
    ])?;

    let point = &output["trend"]["points"][0];
    assert_eq!(point["in_count"], json!(1));
    assert_eq!(point["out_count"], json!(1));
    assert_eq!(point["value"], json!(1.0));
    Ok(())
}

#[test]
fn query_chart_trend_cli_marks_missing_balances_unavailable() -> Result<()> {
    let db = TempDb::new("chart-trend-balance-unavailable")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, 'CARD-NOBAL' AS acct_key, 'CARD-NOBAL' AS card_no,
           'A-NOBAL' AS acct_no, 25.0 AS amount, NULL AS balance,
           'txn-missing-balance' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 1;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-trend",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-NOBAL",
        "--selection-mode",
        "single-card",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
    ])?;
    let trend = &output["trend"];
    assert_eq!(trend["points"][0]["total_amount"], 25.0);
    assert_eq!(trend["points"][0]["latest_balance"], json!(null));
    assert_eq!(trend["balance_points"][0]["balance"], json!(null));
    assert_eq!(trend["balance_available"], false);
    assert_eq!(trend["balance_coverage_rate"], 0.0);
    assert_eq!(trend["balance_markers"], json!([]));
    Ok(())
}
