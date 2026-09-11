use anyhow::Result;
use serde_json::json;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{insert_replacement_card_duplicate, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_txn_rows_cli_returns_placeholder_key_rows() -> Result<()> {
    let db = TempDb::new("stats-txn-placeholder")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-002",
        "--key-type",
        "account",
        "--key-value",
        "__cp_placeholder__::empty",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["done"], true);
    assert_eq!(
        output["nextCursor"],
        json!({"id": 3, "ts": "2026-04-02 11:00:00", "tsNull": 0})
    );
    assert_eq!(
        output["rows"],
        json!([
            {
                "id": 3,
                "card_no": "CARD-002",
                "acct_no": "A-002",
                "account_open_name": "李四",
                "opener_id_no": "ID-B",
                "txn_time": "2026-04-02 11:00:00",
                "amount": 500.0,
                "balance": 500.0,
                "dc_flag": "进",
                "counterparty_acct": "",
                "cash_flag": "否",
                "counterparty_name": "",
                "counterparty_id_no": "",
                "counterparty_bank": "",
                "summary": "未知对手入账",
                "currency": "CNY",
                "branch_name": "",
                "branch_code": "",
                "location": "",
                "is_success": "成功",
                "voucher_no": "",
                "terminal_no": "",
                "ip_addr": "",
                "mac_addr": "",
                "counterparty_balance": "",
                "txn_id": "txn-placeholder",
                "log_id": "",
                "voucher_type": "",
                "voucher_id": "",
                "teller_no": "",
                "merchant_name": "",
                "merchant_no": "",
                "remark": "",
                "txn_type": "转账",
                "query_feedback_reason": "",
            }
        ])
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_dedupes_replacement_card_detail_rows() -> Result<()> {
    let db = TempDb::new("stats-txn-replacement-card-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_replacement_card_duplicate(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selected-key",
        "CARD-003",
        "--key-type",
        "account",
        "--key-value",
        "CP-001",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["rows"].as_array().map(Vec::len), Some(1));
    assert_eq!(output["rows"][0]["amount"], json!(10000.0));
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_filters_source_date_and_direction() -> Result<()> {
    let db = TempDb::new("stats-txn-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--key-type",
        "account",
        "--key-value",
        "CP-002",
        "--date-start",
        "2026-04-01",
        "--date-end",
        "2026-04-01",
        "--direction",
        "out",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["done"], true);
    assert_eq!(
        output["nextCursor"],
        json!({"id": 2, "ts": "2026-04-01 10:00:00", "tsNull": 0})
    );
    assert_eq!(output["rows"][0]["txn_id"], "txn-rust-out");
    assert_eq!(output["rows"][0]["dc_flag"], "出");
    assert_eq!(output["rows"][0]["counterparty_acct"], "CP-002");
    assert_eq!(output["rows"][0]["amount"], 3000.0);
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_filters_repeated_counterparty_keys() -> Result<()> {
    let db = TempDb::new("stats-txn-batched-counterparty")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--key-type",
        "account",
        "--key-value",
        "CP-001",
        "--key-value",
        "CP-002",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(
        output["rows"].as_array().map(|rows| rows
            .iter()
            .map(|row| row["txn_id"].as_str().unwrap_or(""))
            .collect::<Vec<_>>()),
        Some(vec!["txn-rust-in", "txn-rust-out"])
    );
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_filters_account_rows_by_time_window() -> Result<()> {
    let db = TempDb::new("stats-txn-time-window")?;
    seed_stats_query_tables(db.path())?;

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
        "--start-time",
        "2026-04-01 09:30:00",
        "--end-time",
        "2026-04-01 10:30:00",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["done"], true);
    assert_eq!(
        output["nextCursor"],
        json!({"id": 2, "ts": "2026-04-01 10:00:00", "tsNull": 0})
    );
    assert_eq!(output["rows"].as_array().map(Vec::len), Some(1));
    assert_eq!(output["rows"][0]["txn_id"], "txn-rust-out");
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_applies_manual_account_identity() -> Result<()> {
    let db = TempDb::new("stats-txn-manual-identity")?;
    seed_stats_query_tables(db.path())?;
    {
        let conn = duckdb::Connection::open(db.path())?;
        conn.execute_batch(&format!(
            "
            CREATE TABLE analysis_manual_account_mapping(
              case_id TEXT,
              account_key TEXT,
              account_open_name TEXT,
              opener_id_no TEXT,
              bank_name TEXT,
              updated_at TIMESTAMP
            );
            INSERT INTO analysis_manual_account_mapping VALUES
              ('{case_id}', 'CARD-001', '人工补录张三', 'ID-MANUAL', '', TIMESTAMP '2026-04-03 00:00:00');
            ",
            case_id = CASE_ID
        ))?;
    }

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
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["rows"].as_array().map(Vec::len), Some(2));
    assert_eq!(output["rows"][0]["account_open_name"], "人工补录张三");
    assert_eq!(output["rows"][0]["opener_id_no"], "ID-MANUAL");
    assert_eq!(output["rows"][1]["account_open_name"], "人工补录张三");
    assert_eq!(output["rows"][1]["opener_id_no"], "ID-MANUAL");
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_filters_by_counterparty_name_key() -> Result<()> {
    let db = TempDb::new("stats-txn-name-key")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-txn-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--key-type",
        "name",
        "--key-value",
        "对手乙",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["done"], true);
    assert_eq!(
        output["nextCursor"],
        json!({"id": 2, "ts": "2026-04-01 10:00:00", "tsNull": 0})
    );
    assert_eq!(output["rows"][0]["txn_id"], "txn-rust-out");
    assert_eq!(output["rows"][0]["counterparty_name"], "对手乙");
    assert_eq!(output["rows"][0]["counterparty_acct"], "CP-002");
    assert_eq!(output["rows"][0]["amount"], 3000.0);
    Ok(())
}
