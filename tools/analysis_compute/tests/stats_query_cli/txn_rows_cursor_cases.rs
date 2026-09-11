use anyhow::Result;
use serde_json::{json, Value};
use std::fs;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_txn_rows_cli_returns_cursor_page() -> Result<()> {
    let db = TempDb::new("stats-txn-rows")?;
    seed_stats_query_tables(db.path())?;

    let first_page = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--limit",
        "1",
    ])?;

    assert_eq!(first_page["ok"], true);
    assert_eq!(first_page["case_id"], CASE_ID);
    assert_eq!(first_page["done"], false);
    assert_eq!(first_page["nextCursor"], json!({"id": 1, "abs": 10000.0}));
    assert_eq!(
        first_page["rows"],
        json!([
            {
                "id": 1,
                "card_no": "CARD-001",
                "acct_no": "A-001",
                "account_open_name": "张三",
                "opener_id_no": "ID-A",
                "txn_time": "2026-04-01 09:00:00",
                "amount": 10000.0,
                "balance": 10000.0,
                "dc_flag": "进",
                "counterparty_acct": "CP-001",
                "cash_flag": "否",
                "counterparty_name": "对手甲",
                "counterparty_id_no": "",
                "counterparty_bank": "测试银行",
                "summary": "工资入账",
                "currency": "CNY",
                "branch_name": "一号支行",
                "branch_code": "BR-001",
                "location": "贵阳",
                "is_success": "成功",
                "voucher_no": "",
                "terminal_no": "TERM-01",
                "ip_addr": "",
                "mac_addr": "",
                "counterparty_balance": "",
                "txn_id": "txn-rust-in",
                "log_id": "",
                "voucher_type": "",
                "voucher_id": "",
                "teller_no": "T-01",
                "merchant_name": "商户甲",
                "merchant_no": "M-001",
                "remark": "",
                "txn_type": "转账",
                "query_feedback_reason": "",
            }
        ])
    );

    let cursor = first_page["nextCursor"].to_string();
    let second_page = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--limit",
        "1",
        "--cursor-json",
        &cursor,
    ])?;

    assert_eq!(second_page["done"], false);
    assert_eq!(second_page["nextCursor"], json!({"id": 2, "abs": 3000.0}));
    assert_eq!(second_page["rows"][0]["txn_id"], "txn-rust-out");
    assert_eq!(second_page["rows"][0]["amount"], 3000.0);
    assert_eq!(second_page["rows"][0]["dc_flag"], "出");
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_preserves_zero_but_blocks_missing_amount_coverage() -> Result<()> {
    let db = TempDb::new("stats-txn-amount-null-cursor")?;
    seed_stats_query_tables(db.path())?;
    let conn = duckdb::Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           5 AS id, 0.0 AS amount, 1 AS amount_source_present,
           0 AS amount_parse_failed, 0.0 AS balance, 'txn-real-zero' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 2;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let first = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
    ])?;
    assert_eq!(first["rows"][0]["txn_id"], "txn-real-zero");
    assert_eq!(first["rows"][0]["amount"], 0.0);
    assert_eq!(first["nextCursor"], json!({"id": 5, "abs": 0.0}));

    let conn = duckdb::Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, NULL AS amount, 0 AS amount_source_present,
           0 AS amount_parse_failed, NULL AS balance, 'txn-missing-amount' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 2;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;
    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
    ])?;
    assert!(failure.stdout.trim().is_empty());
    assert!(failure
        .stderr
        .contains("transaction_amount_coverage_incomplete"));
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_pages_by_txn_time_cursor() -> Result<()> {
    let db = TempDb::new("stats-txn-time-cursor")?;
    seed_stats_query_tables(db.path())?;

    let first_page = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
    ])?;

    assert_eq!(first_page["done"], false);
    assert_eq!(
        first_page["nextCursor"],
        json!({"id": 1, "ts": "2026-04-01 09:00:00", "tsNull": 0})
    );
    assert_eq!(first_page["rows"][0]["txn_id"], "txn-rust-in");

    let first_cursor = first_page["nextCursor"].to_string();
    let second_page = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
        "--cursor-json",
        &first_cursor,
    ])?;

    assert_eq!(second_page["done"], false);
    assert_eq!(
        second_page["nextCursor"],
        json!({"id": 2, "ts": "2026-04-01 10:00:00", "tsNull": 0})
    );
    assert_eq!(second_page["rows"][0]["txn_id"], "txn-rust-out");

    let second_cursor = second_page["nextCursor"].to_string();
    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
        "--cursor-json",
        &second_cursor,
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_keeps_null_txn_time_last() -> Result<()> {
    let db = TempDb::new("stats-txn-null-time-order")?;
    seed_stats_query_tables(db.path())?;
    {
        let conn = duckdb::Connection::open(db.path())?;
        conn.execute_batch(
            "
            INSERT INTO analysis_txn_detail_idx
            SELECT * REPLACE (
              4 AS id, 'CP-003' AS cp_key, 'CP-003' AS cp_raw,
              '对手丙' AS cp_name, '对手丙' AS cp_name_pick,
              '对手丙' AS stats_name_key, NULL AS txn_ts, '' AS txn_time,
              100.0 AS amount, 7100.0 AS balance, 'CP-003' AS counterparty_acct,
              '对手丙' AS counterparty_name, '空时间入账' AS summary,
              'txn-null-time' AS txn_id
            ) FROM analysis_txn_detail_idx WHERE id=1;
            ",
        )?;
    }
    refresh_stats_query_materialization_meta(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "0",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["rows"].as_array().map(|rows| rows
            .iter()
            .map(|row| row["txn_id"].as_str().unwrap_or_default())
            .collect::<Vec<_>>()),
        Some(vec!["txn-rust-in", "txn-rust-out", "txn-null-time"])
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_omits_cursor_fields_when_limit_is_zero() -> Result<()> {
    let db = TempDb::new("stats-txn-limit-zero")?;
    seed_stats_query_tables(db.path())?;

    let direct = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "0",
    ])?;

    assert_eq!(direct["ok"], true);
    assert_eq!(direct["case_id"], CASE_ID);
    assert!(direct.get("done").is_none());
    assert!(direct.get("nextCursor").is_none());
    assert_eq!(direct["rows"].as_array().map(Vec::len), Some(2));
    assert_eq!(direct["rows"][0]["txn_id"], "txn-rust-in");
    assert_eq!(direct["rows"][1]["txn_id"], "txn-rust-out");

    let output_path = db.path().with_extension("txn-limit-zero-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();
    let output_ref = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "0",
        "--output-json",
        &output_path_text,
    ])?;

    assert_eq!(output_ref["ok"], true);
    assert_eq!(output_ref["case_id"], CASE_ID);
    assert_eq!(output_ref["row_count"], 2);
    assert_eq!(output_ref["done"], json!(null));
    assert_eq!(
        output_ref["output_json"].as_str(),
        Some(output_path_text.as_str())
    );
    assert!(output_ref.get("rows").is_none());

    let payload: Value = serde_json::from_str(&fs::read_to_string(&output_path)?)?;
    let _ = fs::remove_file(&output_path);
    assert_eq!(payload["rows"], direct["rows"]);
    assert!(payload.get("done").is_none());
    assert!(payload.get("next_cursor").is_none());
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_rejects_invalid_cursor_json_shape() -> Result<()> {
    let db = TempDb::new("stats-txn-invalid-cursor")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--limit",
        "1",
        "--cursor-json",
        "{\"abs\":10000.0}",
    ])?;

    assert!(failure.stdout.trim().is_empty());
    assert!(
        failure.stderr.contains("--cursor-json requires numeric id"),
        "unexpected stderr: {}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_rejects_amount_cursor_without_abs() -> Result<()> {
    let db = TempDb::new("stats-txn-invalid-amount-cursor")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--limit",
        "1",
        "--cursor-json",
        "{\"id\":1}",
    ])?;

    assert!(failure.stdout.trim().is_empty());
    assert!(
        failure
            .stderr
            .contains("--cursor-json requires numeric abs for amount sorting"),
        "unexpected stderr: {}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_rejects_nonfinite_amount_cursors() -> Result<()> {
    let db = TempDb::new("stats-txn-nonfinite-cursor")?;
    seed_stats_query_tables(db.path())?;

    for abs in ["NaN", "inf", "-inf"] {
        let cursor = format!(r#"{{"id":1,"abs":"{abs}"}}"#);
        let failure = run_analysis_compute_failure(&[
            "query-stats-txn-rows",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--key-type",
            "account",
            "--key-value",
            "CARD-001",
            "--sort-col",
            "amount",
            "--sort-dir",
            "asc",
            "--limit",
            "1",
            "--cursor-json",
            &cursor,
        ])?;
        assert!(
            failure
                .stderr
                .contains("--cursor-json requires numeric abs for amount sorting"),
            "abs={abs} stderr={}",
            failure.stderr
        );
    }
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_rejects_negative_absolute_cursor() -> Result<()> {
    let db = TempDb::new("stats-txn-negative-abs-cursor")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "amount",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
        "--cursor-json",
        r#"{"id":1,"abs":-1}"#,
    ])?;
    assert!(
        failure
            .stderr
            .contains("--cursor-json abs must be non-negative"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_rejects_txn_time_cursor_without_ts_or_ts_null() -> Result<()> {
    let db = TempDb::new("stats-txn-invalid-time-cursor")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--key-type",
        "account",
        "--key-value",
        "CARD-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "1",
        "--cursor-json",
        "{\"id\":1}",
    ])?;

    assert!(failure.stdout.trim().is_empty());
    assert!(
        failure
            .stderr
            .contains("--cursor-json requires ts or tsNull=true for txn_time sorting"),
        "unexpected stderr: {}",
        failure.stderr
    );
    Ok(())
}
