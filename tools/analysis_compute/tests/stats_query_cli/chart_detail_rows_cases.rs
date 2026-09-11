use anyhow::Result;
use duckdb::Connection;
use serde_json::{json, Value};
use std::fs;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{
    insert_replacement_card_duplicate, refresh_stats_query_materialization_meta,
    seed_stats_query_tables, CASE_ID,
};
use crate::temp_db::TempDb;

#[test]
fn query_chart_detail_rows_cli_filters_sorts_and_pages_detail_rows() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "1",
        "--limit",
        "1",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01","label":"2026-04-01","payload":{"granularity":"day","bucket":"2026-04-01"}}]"#,
    ])?;

    assert_eq!(output["total"], json!(2));
    assert_eq!(output["rows"][0]["txn_id"], json!("txn-rust-in"));
    assert_eq!(output["rows"][0]["amount"], json!(10000.0));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_preserves_zero_but_blocks_missing_amount_coverage() -> Result<()> {
    let db = TempDb::new("chart-detail-numeric-null-order")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
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

    for sort_col in ["amount", "balance"] {
        let output = run_analysis_compute_command(&[
            "query-chart-detail-rows",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--selected-key",
            "CARD-001",
            "--direction-mode",
            "all",
            "--success-filter",
            "all",
            "--cash-filter",
            "all",
            "--sort-col",
            sort_col,
            "--sort-dir",
            "asc",
            "--page",
            "1",
            "--limit",
            "10",
        ])?;
        assert_eq!(output["rows"][0]["txn_id"], "txn-real-zero");
        assert_eq!(output["rows"][0][sort_col], 0.0);
    }

    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_txn_detail_idx
         SELECT * REPLACE (
           4 AS id, NULL AS amount, 0 AS amount_source_present,
           0 AS amount_parse_failed, NULL AS balance, 'txn-missing-numeric' AS txn_id
         )
         FROM analysis_txn_detail_idx WHERE id = 2;",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;
    let failure = run_analysis_compute_failure(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "asc",
        "--page",
        "1",
        "--limit",
        "10",
    ])?;
    assert!(failure.stdout.trim().is_empty());
    assert!(failure
        .stderr
        .contains("transaction_amount_coverage_incomplete"));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_dedupes_replacement_card_rows() -> Result<()> {
    let db = TempDb::new("chart-detail-replacement-card-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_replacement_card_duplicate(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selected-key",
        "CARD-003",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "1",
        "--limit",
        "50",
    ])?;

    assert_eq!(output["total"], json!(2));
    assert_eq!(output["rows"].as_array().map(Vec::len), Some(2));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_writes_result_payload_to_output_json() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-output-json")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("chart-detail-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let direct = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "1",
        "--limit",
        "1",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01","label":"2026-04-01","payload":{"granularity":"day","bucket":"2026-04-01"}}]"#,
    ])?;
    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "1",
        "--limit",
        "1",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01","label":"2026-04-01","payload":{"granularity":"day","bucket":"2026-04-01"}}]"#,
        "--output-json",
        &output_path_text,
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["total"], 2);
    assert_eq!(output["row_count"], 1);
    assert_eq!(
        output["output_json"].as_str(),
        Some(output_path_text.as_str())
    );
    assert!(output.get("rows").is_none());

    let payload: Value = serde_json::from_str(&fs::read_to_string(&output_path)?)?;
    let _ = fs::remove_file(&output_path);
    assert_eq!(payload["rows"], direct["rows"]);
    assert_eq!(payload["total"], direct["total"]);
    assert_eq!(payload["rows"].as_array().map(Vec::len), Some(1));
    assert_eq!(payload["rows"][0]["txn_id"], json!("txn-rust-in"));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_applies_manual_account_identity() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-manual-identity")?;
    seed_stats_query_tables(db.path())?;
    {
        let conn = Connection::open(db.path())?;
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
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--page",
        "1",
        "--limit",
        "50",
    ])?;

    assert_eq!(output["total"], json!(2));
    assert_eq!(
        output["rows"][0]["account_open_name"],
        json!("人工补录张三")
    );
    assert_eq!(output["rows"][0]["opener_id_no"], json!("ID-MANUAL"));
    assert_eq!(
        output["rows"][1]["account_open_name"],
        json!("人工补录张三")
    );
    assert_eq!(output["rows"][1]["opener_id_no"], json!("ID-MANUAL"));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_rejects_empty_offset_page() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-empty-offset")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "3",
        "--limit",
        "1",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01","label":"2026-04-01","payload":{"granularity":"day","bucket":"2026-04-01"}}]"#,
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_does_not_write_empty_offset_page() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-output-empty-offset")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("chart-detail-empty-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let failure = run_analysis_compute_failure(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "amount",
        "--sort-dir",
        "desc",
        "--page",
        "3",
        "--limit",
        "1",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01","label":"2026-04-01","payload":{"granularity":"day","bucket":"2026-04-01"}}]"#,
        "--output-json",
        &output_path_text,
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    assert!(!output_path.exists());
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_applies_summary_keyword_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-summary-keyword")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--page",
        "1",
        "--limit",
        "50",
        "--chart-filters-json",
        r#"[{"source_panel_id":"anomaly","dimension":"summary_keyword","value":"工资入账","label":"工资入账","payload":{}}]"#,
    ])?;

    assert_eq!(output["total"], json!(1));
    assert_eq!(output["rows"][0]["txn_id"], json!("txn-rust-in"));
    assert_eq!(output["rows"][0]["summary"], json!("工资入账"));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_applies_remark_keyword_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-remark-keyword")?;
    seed_stats_query_tables(db.path())?;
    {
        let conn = Connection::open(db.path())?;
        conn.execute_batch(
            "
            UPDATE analysis_txn_detail_idx
               SET remark = 'ATM取现过路费'
             WHERE txn_id = 'txn-rust-out';
            INSERT INTO analysis_txn_keyword_idx VALUES
              (2, 'txn-rust-out', 'remark', 'ATM', 0),
              (2, 'txn-rust-out', 'remark', '取现', 1),
              (2, 'txn-rust-out', 'remark', '过路费', 2);
            ",
        )?;
    }
    refresh_stats_query_materialization_meta(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--page",
        "1",
        "--limit",
        "50",
        "--chart-filters-json",
        r#"[{"source_panel_id":"anomaly","dimension":"remark_keyword","value":"过路费","label":"过路费","payload":{}}]"#,
    ])?;

    assert_eq!(output["total"], json!(1));
    assert_eq!(output["rows"][0]["txn_id"], json!("txn-rust-out"));
    assert_eq!(output["rows"][0]["remark"], json!("ATM取现过路费"));
    Ok(())
}

#[test]
fn query_chart_detail_rows_cli_applies_heatmap_and_direction_filters() -> Result<()> {
    let db = TempDb::new("chart-detail-rows-heatmap")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-detail-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--date-start",
        "",
        "--date-end",
        "",
        "--direction-mode",
        "in",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--sort-col",
        "txn_time",
        "--sort-dir",
        "asc",
        "--page",
        "1",
        "--limit",
        "50",
        "--chart-filters-json",
        r#"[{"source_panel_id":"heatmap","dimension":"heatmap_cell","value":"2-9","label":"周三 09:00","payload":{"weekday":2,"hour":9}}]"#,
    ])?;

    assert_eq!(output["total"], json!(1));
    assert_eq!(output["rows"][0]["txn_id"], json!("txn-rust-in"));
    assert_eq!(output["rows"][0]["dc_flag"], json!("进"));
    Ok(())
}
