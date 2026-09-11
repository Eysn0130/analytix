use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::json;
use std::path::Path;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_summary_cli_returns_dashboard_summary_payload() -> Result<()> {
    let db = TempDb::new("chart-summary")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--large-txn-threshold",
        "5000",
    ])?;

    assert_eq!(
        output["summary"],
        json!({
            "total_in_amount": 10000.0,
            "total_out_amount": 3000.0,
            "net_in_amount": 7000.0,
            "txn_total_count": 2,
            "active_counterparty_count": 2,
            "success_rate": 100.0,
            "latest_balance": 7000.0,
            "max_single_amount": 10000.0,
            "large_txn_threshold": 5000.0,
            "large_txn_count": 1,
            "latest_txn_time": "2026-04-01 10:00:00",
            "account_name": "张三",
            "account_key": "A-001",
            "card_no": "CARD-001",
            "balance_available": true,
            "selection_mode": "single-card"
        })
    );
    Ok(())
}

#[test]
fn query_chart_summary_cli_rejects_nonfinite_thresholds() -> Result<()> {
    let db = TempDb::new("chart-summary-nonfinite-threshold")?;
    seed_stats_query_tables(db.path())?;

    for threshold in ["NaN", "inf", "-inf"] {
        let failure = run_analysis_compute_failure(&[
            "query-chart-summary",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--selected-key",
            "CARD-001",
            "--large-txn-threshold",
            threshold,
        ])?;
        assert!(
            failure.stderr.contains("--large-txn-threshold"),
            "threshold={threshold} stderr={}",
            failure.stderr
        );
    }
    Ok(())
}

#[test]
fn query_chart_summary_cli_blocks_missing_amount_but_preserves_verified_zero() -> Result<()> {
    let missing_db = TempDb::new("chart-summary-missing-amount")?;
    seed_stats_query_tables(missing_db.path())?;
    let conn = Connection::open(missing_db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, 'CARD-MISSING' AS acct_key, 'CARD-MISSING' AS card_no,
           'A-MISSING' AS acct_no, NULL AS amount, 0 AS amount_source_present,
           0 AS amount_parse_failed, 'txn-missing-amount' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 1;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(missing_db.path())?;
    let failure = run_analysis_compute_failure(&[
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &missing_db.path().to_string_lossy(),
        "--selected-key",
        "CARD-MISSING",
    ])?;
    assert!(
        failure
            .stderr
            .contains("transaction_amount_coverage_incomplete"),
        "stderr={}",
        failure.stderr
    );

    let zero_db = TempDb::new("chart-summary-real-zero")?;
    seed_stats_query_tables(zero_db.path())?;
    let conn = Connection::open(zero_db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, 'CARD-ZERO' AS acct_key, 'CARD-ZERO' AS card_no,
           'A-ZERO' AS acct_no, 0.0 AS amount, 0.0 AS balance,
           1 AS amount_source_present, 0 AS amount_parse_failed,
           'txn-real-zero' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 1;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(zero_db.path())?;
    let output = run_analysis_compute_command(&[
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &zero_db.path().to_string_lossy(),
        "--selected-key",
        "CARD-ZERO",
    ])?;
    assert_eq!(output["summary"]["total_in_amount"], 0.0);
    assert_eq!(output["summary"]["max_single_amount"], 0.0);
    assert_eq!(output["summary"]["latest_balance"], 0.0);
    assert_eq!(output["summary"]["balance_available"], true);
    assert_eq!(output["summary"]["txn_total_count"], 1);
    Ok(())
}

#[test]
fn query_chart_summary_cli_does_not_publish_missing_balance_as_zero() -> Result<()> {
    let db = TempDb::new("chart-summary-balance-unavailable")?;
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
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-NOBAL",
    ])?;
    assert_eq!(output["summary"]["total_in_amount"], 25.0);
    assert_eq!(output["summary"]["latest_balance"], json!(null));
    assert_eq!(output["summary"]["balance_available"], false);
    Ok(())
}

#[test]
fn query_chart_summary_cli_rejects_context_rows_without_known_direction() -> Result<()> {
    let db = TempDb::new("chart-summary-context-direction")?;
    seed_stats_query_tables(db.path())?;
    insert_other_direction_row(db.path().to_string_lossy().as_ref())?;

    let failure = run_analysis_compute_failure(&[
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--large-txn-threshold",
        "15000",
    ])?;

    assert!(
        failure
            .stderr
            .contains("transaction_amount_coverage_incomplete"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_chart_summary_cli_prefers_manual_account_mapping_name() -> Result<()> {
    let db = TempDb::new("chart-summary-manual-account")?;
    seed_stats_query_tables(db.path())?;
    insert_manual_account_mapping(db.path().to_string_lossy().as_ref())?;

    let output = run_analysis_compute_command(&[
        "query-chart-summary",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    let summary = &output["summary"];
    assert_eq!(summary["account_name"], json!("人工补录张三"));
    assert_eq!(summary["account_key"], json!("A-001"));
    assert_eq!(summary["card_no"], json!("CARD-001"));
    Ok(())
}

fn insert_other_direction_row(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for extra summary row")?;
    conn.execute_batch(
        "
        INSERT INTO analysis_txn_detail_idx
        SELECT * REPLACE (
          4 AS id, NULL AS cp_key, '' AS cp_raw, 'empty' AS cp_placeholder_kind,
          NULL AS cp_name, NULL AS cp_name_pick, 0 AS cp_name_pick_cnt,
          '__cp_placeholder__::empty' AS stats_name_key, '其他' AS dc_val,
          TIMESTAMP '2026-04-01 11:00:00' AS txn_ts,
          '2026-04-01 11:00:00' AS txn_time, 20000.0 AS amount,
          9000.0 AS balance, '' AS counterparty_acct, '' AS counterparty_name,
          '其他调整' AS summary, '失败' AS is_success,
          'txn-other' AS txn_id, '调整' AS txn_type
        ) FROM analysis_txn_detail_idx WHERE id=1;
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(Path::new(path))
}

fn insert_manual_account_mapping(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for manual account mapping")?;
    conn.execute_batch(
        "
        CREATE TABLE analysis_manual_account_mapping(
          case_id TEXT,
          account_key TEXT,
          account_open_name TEXT
        );
        INSERT INTO analysis_manual_account_mapping VALUES
          ('case-stats-query-golden', 'CARD-001', '人工补录张三');
        ",
    )?;
    Ok(())
}
