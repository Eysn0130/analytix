use anyhow::Result;
use serde_json::json;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_chart_flow_cli_returns_dashboard_flow_payload() -> Result<()> {
    let db = TempDb::new("chart-flow")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-flow",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--direction",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "flow": {
                "center_label": "张三",
                "nodes": [
                    {"id": "selected-center", "label": "张三", "side": "center", "value": 0, "amount": 0, "count": 0},
                    {"id": "in-0", "label": "对手甲", "side": "in", "value": 10000.0, "amount": 10000.0, "count": 1, "first_time": "2026-04-01 09:00:00", "last_time": "2026-04-01 09:00:00"},
                    {"id": "out-0", "label": "对手乙", "side": "out", "value": 3000.0, "amount": 3000.0, "count": 1, "first_time": "2026-04-01 10:00:00", "last_time": "2026-04-01 10:00:00"}
                ],
                "links": [
                    {"source": "in-0", "target": "selected-center", "label": "对手甲", "direction": "in", "amount": 10000.0, "count": 1, "value": 10000.0, "first_time": "2026-04-01 09:00:00", "last_time": "2026-04-01 09:00:00"},
                    {"source": "selected-center", "target": "out-0", "label": "对手乙", "direction": "out", "amount": 3000.0, "count": 1, "value": 3000.0, "first_time": "2026-04-01 10:00:00", "last_time": "2026-04-01 10:00:00"}
                ],
                "totals": {"in_amount": 10000.0, "out_amount": 3000.0, "in_count": 1, "out_count": 1},
                "inbound_items": [
                    {"label": "对手甲", "amount": 10000.0, "count": 1, "first_time": "2026-04-01 09:00:00", "last_time": "2026-04-01 09:00:00"}
                ],
                "outbound_items": [
                    {"label": "对手乙", "amount": 3000.0, "count": 1, "first_time": "2026-04-01 10:00:00", "last_time": "2026-04-01 10:00:00"}
                ]
            }
        })
    );
    Ok(())
}

#[test]
fn query_chart_flow_cli_applies_direction_and_cash_filters() -> Result<()> {
    let db = TempDb::new("chart-flow-filters")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-flow",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--direction",
        "out",
        "--success-filter",
        "success",
        "--cash-filter",
        "non-cash",
    ])?;

    let flow = &output["flow"];
    assert_eq!(
        flow["totals"],
        json!({"in_amount": 0.0, "out_amount": 3000.0, "in_count": 0, "out_count": 1})
    );
    assert_eq!(flow["inbound_items"], json!([]));
    assert_eq!(flow["outbound_items"][0]["label"], json!("对手乙"));
    Ok(())
}

#[test]
fn query_chart_flow_cli_groups_counterparties_by_normalized_name() -> Result<()> {
    let db = TempDb::new("chart-flow-normalized-counterparty-name")?;
    seed_stats_query_tables(db.path())?;
    let conn = duckdb::Connection::open(db.path())?;
    conn.execute_batch(
        "UPDATE analysis_txn_detail_idx
            SET counterparty_name = '原始户名',
                stats_name_key = '规范户名',
                cp_name_pick = '规范户名',
                cp_name = '规范户名'
          WHERE id = 2;
         UPDATE analysis_txn_daily_agg
            SET stats_name_key = '规范户名',
                cp_name_pick = '规范户名',
                cp_name = '规范户名'
          WHERE cp_key = 'CP-002';",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-flow",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selection-mode",
        "single-card",
        "--direction",
        "all",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
    ])?;

    assert_eq!(
        output["flow"]["outbound_items"][0]["label"],
        json!("规范户名")
    );
    assert_eq!(output["flow"]["outbound_items"][0]["amount"], json!(3000.0));
    assert_eq!(output["flow"]["outbound_items"][0]["count"], json!(1));
    Ok(())
}
