use anyhow::Result;
use serde_json::json;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_rows_cli_returns_placeholder_key_rows() -> Result<()> {
    let db = TempDb::new("stats-rows-placeholder")?;
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
        "CARD-002",
        "--row-limit",
        "10",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "group_key": "cp_key",
            "total": 1,
            "row_summary": {"total_amount": 500.0, "total_count": 1},
            "rows": [
                {
                    "id": "cp_key:__cp_placeholder__::empty",
                    "counterparty_account": "",
                    "counterparty_name": "",
                    "relation": "",
                    "location": "",
                    "bank": "",
                    "doc": "未调单",
                    "total_amount": 500.0,
                    "total_count": 1,
                    "net_in": 500.0,
                    "net_out": -500.0,
                    "in_amount": 500.0,
                    "in_count": 1,
                    "out_amount": 0.0,
                    "out_count": 0,
                    "first_time": "2026-04-02 11:00:00",
                    "last_time": "2026-04-02 11:00:00",
                    "keyType": "account",
                    "keyValue": "__cp_placeholder__::empty",
                    "keyLabel": "空串",
                    "drillKeyType": "account",
                    "drillKeyValue": "__cp_placeholder__::empty",
                    "placeholderKind": "empty",
                },
            ],
        })
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_rejects_empty_filtered_scope_instead_of_zero_summary() -> Result<()> {
    let db = TempDb::new("stats-rows-empty-filtered-scope")?;
    seed_stats_query_tables(db.path())?;

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
        "--search-text",
        "definitely-no-match",
    ])?;
    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_filters_source_date_and_search_text() -> Result<()> {
    let db = TempDb::new("stats-rows-filtered")?;
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
        "--date-start",
        "2026-04-01",
        "--date-end",
        "2026-04-01",
        "--search-text",
        "乙",
        "--row-sort-col",
        "total_amount",
        "--row-sort-dir",
        "desc",
        "--row-limit",
        "10",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "group_key": "cp_key",
            "total": 1,
            "row_summary": {"total_amount": 3000.0, "total_count": 1},
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
            ],
        })
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_returns_doc_status_in_display_rows() -> Result<()> {
    let db = TempDb::new("stats-rows-doc-status")?;
    seed_stats_query_tables(db.path())?;
    let conn = duckdb::Connection::open(db.path())?;
    conn.execute_batch(
        "
        INSERT INTO analysis_account_dim VALUES
          ('CP-001', 'CP-001', 'CP-001', '对手甲', '', '测试银行', '', '');
        CREATE TABLE analysis_doc_status(
          case_id TEXT,
          key_type TEXT,
          key_value TEXT,
          status TEXT,
          updated_at TIMESTAMP
        );
        INSERT INTO analysis_doc_status VALUES
          ('case-stats-query-golden', 'account', 'CP-002', 'pending', NOW()),
          ('case-stats-query-golden', 'name', '对手乙', 'pending', NOW());
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let account_output = run_analysis_compute_command(&[
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
    let account_docs = account_output["rows"]
        .as_array()
        .unwrap()
        .iter()
        .map(|row| {
            (
                row["keyValue"].as_str().unwrap(),
                row["doc"].as_str().unwrap(),
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        account_docs,
        vec![("CP-002", "待调单"), ("CP-001", "已调单")]
    );

    let name_output = run_analysis_compute_command(&[
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
    let name_docs = name_output["rows"]
        .as_array()
        .unwrap()
        .iter()
        .map(|row| {
            (
                row["keyValue"].as_str().unwrap(),
                row["doc"].as_str().unwrap(),
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(name_docs, vec![("对手乙", "待调单"), ("对手甲", "已调单")]);
    Ok(())
}
