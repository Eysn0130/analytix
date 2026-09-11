use anyhow::Result;
use serde_json::Value;
use std::fs;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_rows_cli_writes_result_payload_to_output_json() -> Result<()> {
    let db = TempDb::new("stats-rows-output-json")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("rows-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let direct = run_analysis_compute_command(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
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
    ])?;
    let output = run_analysis_compute_command(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
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
        "--output-json",
        &output_path_text,
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["group_key"], "cp_key");
    assert_eq!(output["total"], 2);
    assert_eq!(output["row_count"], 2);
    assert_eq!(
        output["output_json"].as_str(),
        Some(output_path_text.as_str())
    );
    assert!(output.get("rows").is_none());

    let payload: Value = serde_json::from_str(&fs::read_to_string(&output_path)?)?;
    let _ = fs::remove_file(&output_path);
    assert_eq!(payload["rows"], direct["rows"]);
    assert_eq!(payload["status"], "");
    assert_eq!(payload["total"], direct["total"]);
    assert_eq!(payload["row_summary"], direct["row_summary"]);
    assert_eq!(payload["rows"].as_array().map(Vec::len), Some(2));
    assert_eq!(payload["rows"][0]["counterparty_account"], "CP-002");
    assert_eq!(payload["rows"][1]["counterparty_account"], "CP-001");
    Ok(())
}

#[test]
fn query_stats_txn_rows_cli_writes_result_payload_to_output_json() -> Result<()> {
    let db = TempDb::new("stats-txn-output-json")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("txn-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let direct = run_analysis_compute_command(&[
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
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
    ])?;
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
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--limit",
        "10",
        "--output-json",
        &output_path_text,
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["row_count"], 1);
    assert_eq!(output["done"], true);
    assert_eq!(
        output["output_json"].as_str(),
        Some(output_path_text.as_str())
    );
    assert!(output.get("rows").is_none());

    let payload: Value = serde_json::from_str(&fs::read_to_string(&output_path)?)?;
    let _ = fs::remove_file(&output_path);
    assert_eq!(payload["rows"], direct["rows"]);
    assert_eq!(payload["done"], direct["done"]);
    assert_eq!(payload["next_cursor"], direct["nextCursor"]);
    assert_eq!(payload["rows"].as_array().map(Vec::len), Some(1));
    assert_eq!(payload["rows"][0]["txn_id"], "txn-rust-out");
    assert_eq!(payload["rows"][0]["counterparty_acct"], "CP-002");
    Ok(())
}
