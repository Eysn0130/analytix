use anyhow::Result;
use duckdb::Connection;
use serde_json::json;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{
    insert_replacement_card_duplicate, refresh_stats_query_materialization_meta,
    seed_stats_query_tables, CASE_ID,
};
use crate::temp_db::TempDb;

#[test]
fn query_stats_rows_cli_returns_sorted_aggregate_rows() -> Result<()> {
    let db = TempDb::new("stats-rows")?;
    seed_stats_query_tables(db.path())?;

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
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "group_key": "cp_key",
            "total": 2,
            "row_summary": {"total_amount": 13000.0, "total_count": 2},
            "rows": [
                {
                    "id": "cp_key:CP-002",
                    "counterparty_account": "CP-002",
                    "counterparty_name": "对手乙",
                    "relation": "",
                    "location": "贵阳",
                    "bank": "测试银行",
                    "doc": "未调单",
                    "total_amount": 3000.0,
                    "total_count": 1,
                    "net_in": -3000.0,
                    "net_out": 3000.0,
                    "in_amount": 0.0,
                    "in_count": 0,
                    "out_amount": 3000.0,
                    "out_count": 1,
                    "first_time": "2026-04-01 10:00:00",
                    "last_time": "2026-04-01 10:00:00",
                    "keyType": "account",
                    "keyValue": "CP-002",
                    "keyLabel": "CP-002",
                    "drillKeyType": "account",
                    "drillKeyValue": "CP-002",
                    "placeholderKind": "",
                },
                {
                    "id": "cp_key:CP-001",
                    "counterparty_account": "CP-001",
                    "counterparty_name": "对手甲",
                    "relation": "",
                    "location": "贵阳",
                    "bank": "测试银行",
                    "doc": "未调单",
                    "total_amount": 10000.0,
                    "total_count": 1,
                    "net_in": 10000.0,
                    "net_out": -10000.0,
                    "in_amount": 10000.0,
                    "in_count": 1,
                    "out_amount": 0.0,
                    "out_count": 0,
                    "first_time": "2026-04-01 09:00:00",
                    "last_time": "2026-04-01 09:00:00",
                    "keyType": "account",
                    "keyValue": "CP-001",
                    "keyLabel": "CP-001",
                    "drillKeyType": "account",
                    "drillKeyValue": "CP-001",
                    "placeholderKind": "",
                }
            ],
        })
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_rejects_incomplete_amount_coverage_instead_of_zero_fill() -> Result<()> {
    let db = TempDb::new("stats-rows-incomplete-amount")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, NULL AS amount, 0 AS amount_source_present,
           0 AS amount_parse_failed, 'txn-missing-amount' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 1;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inAccount",
        "--selected-key",
        "CARD-001",
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
fn query_stats_rows_cli_dedupes_replacement_card_rows_with_different_account_metadata() -> Result<()>
{
    let db = TempDb::new("stats-rows-replacement-card-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_replacement_card_duplicate(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inName",
        "--selected-key",
        "CARD-001",
        "--selected-key",
        "CARD-003",
        "--row-sort-col",
        "total_amount",
        "--row-sort-dir",
        "desc",
        "--row-limit",
        "10",
    ])?;

    assert_eq!(output["row_summary"]["total_amount"], json!(13000.0));
    assert_eq!(output["row_summary"]["total_count"], json!(2));
    assert_eq!(output["rows"][0]["counterparty_name"], json!("对手甲"));
    assert_eq!(output["rows"][0]["in_amount"], json!(10000.0));
    assert_eq!(output["rows"][0]["in_count"], json!(1));
    Ok(())
}

#[test]
fn query_stats_rows_cli_returns_compact_array_rows() -> Result<()> {
    let db = TempDb::new("stats-rows-compact")?;
    seed_stats_query_tables(db.path())?;

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
        "--row-format",
        "array",
        "--field",
        "id",
        "--field",
        "keyValue",
        "--field",
        "total_amount",
        "--field",
        "total_count",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "group_key": "cp_key",
            "total": 2,
            "row_summary": {"total_amount": 13000.0, "total_count": 2},
            "row_fields": ["id", "keyValue", "total_amount", "total_count"],
            "rows": [
                ["cp_key:CP-002", "CP-002", 3000.0, 1],
                ["cp_key:CP-001", "CP-001", 10000.0, 1],
            ],
        })
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_groups_by_counterparty_name() -> Result<()> {
    let db = TempDb::new("stats-rows-name")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inName",
        "--selected-key",
        "CARD-001",
        "--row-sort-col",
        "total_amount",
        "--row-sort-dir",
        "asc",
        "--row-limit",
        "10",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "group_key": "cp_name",
            "total": 2,
            "row_summary": {"total_amount": 13000.0, "total_count": 2},
            "rows": [
                {
                    "id": "cp_name:对手乙",
                    "counterparty_account": "CP-002",
                    "counterparty_name": "对手乙",
                    "relation": "",
                    "location": "贵阳",
                    "bank": "测试银行",
                    "doc": "未调单",
                    "total_amount": 3000.0,
                    "total_count": 1,
                    "net_in": -3000.0,
                    "net_out": 3000.0,
                    "in_amount": 0.0,
                    "in_count": 0,
                    "out_amount": 3000.0,
                    "out_count": 1,
                    "first_time": "2026-04-01 10:00:00",
                    "last_time": "2026-04-01 10:00:00",
                    "keyType": "name",
                    "keyValue": "对手乙",
                    "keyLabel": "对手乙",
                    "drillKeyType": "name",
                    "drillKeyValue": "对手乙",
                    "placeholderKind": "",
                },
                {
                    "id": "cp_name:对手甲",
                    "counterparty_account": "CP-001",
                    "counterparty_name": "对手甲",
                    "relation": "",
                    "location": "贵阳",
                    "bank": "测试银行",
                    "doc": "未调单",
                    "total_amount": 10000.0,
                    "total_count": 1,
                    "net_in": 10000.0,
                    "net_out": -10000.0,
                    "in_amount": 10000.0,
                    "in_count": 1,
                    "out_amount": 0.0,
                    "out_count": 0,
                    "first_time": "2026-04-01 09:00:00",
                    "last_time": "2026-04-01 09:00:00",
                    "keyType": "name",
                    "keyValue": "对手甲",
                    "keyLabel": "对手甲",
                    "drillKeyType": "name",
                    "drillKeyValue": "对手甲",
                    "placeholderKind": "",
                },
            ],
        })
    );
    Ok(())
}
