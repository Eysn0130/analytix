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
fn query_flow_focus_rows_cli_returns_focus_edges() -> Result<()> {
    let db = TempDb::new("flow-focus-rows")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CP-002",
        "--include-missing-counterparty",
        "false",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "rows": [
                [
                    "CARD-001", "CP-001", "CP-001", "进", 1, 10000.0,
                    "2026-04-01 09:00:00", "2026-04-01 09:00:00", "张三", "对手甲"
                ],
                [
                    "CARD-001", "CP-002", "CP-002", "出", 1, 3000.0,
                    "2026-04-01 10:00:00", "2026-04-01 10:00:00", "张三", "对手乙"
                ],
            ],
        })
    );
    Ok(())
}

#[test]
fn query_flow_focus_rows_cli_returns_placeholder_edge_with_missing_counterparty() -> Result<()> {
    let db = TempDb::new("flow-focus-placeholder")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-002",
        "--selected-placeholder-kind",
        "empty",
        "--include-missing-counterparty",
        "true",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "rows": [
                [
                    "CARD-002", "__cp_placeholder__::empty", "", "进", 1, 500.0,
                    "2026-04-02 11:00:00", "2026-04-02 11:00:00", "李四", null
                ],
            ],
        })
    );
    Ok(())
}

#[test]
fn query_flow_focus_rows_cli_rejects_empty_selected_row_scope_for_seed_counterparty() -> Result<()>
{
    let db = TempDb::new("flow-focus-seed-row-scope")?;
    seed_stats_query_tables(db.path())?;
    insert_seed_internal_mirror_row(&db)?;

    let failure = run_analysis_compute_failure(&[
        "query-flow-focus-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--query-seed-id",
        "CARD-002",
        "--selected-focus-id",
        "CARD-002",
        "--include-missing-counterparty",
        "false",
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_flow_focus_graph_cli_builds_runtime_graph_from_materialized_rows() -> Result<()> {
    let db = TempDb::new("flow-focus-graph")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CP-002",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-001",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-graph",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
        "--expected-total-amount",
        "13000.0",
        "--expected-row-count",
        "2",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "graph": {
                "nodes": [
                    {
                        "id": "CARD-001",
                        "title": "张三",
                        "name": "张三",
                        "display_id": "CARD-001",
                        "display_ids": null,
                        "ntype": "seed",
                        "total_amount": 13000.0,
                        "total_count": 2,
                    },
                    {
                        "id": "CP-001",
                        "title": "对手甲",
                        "name": "对手甲",
                        "display_id": "CP-001",
                        "display_ids": null,
                        "ntype": "node",
                        "total_amount": 10000.0,
                        "total_count": 1,
                    },
                    {
                        "id": "CP-002",
                        "title": "对手乙",
                        "name": "对手乙",
                        "display_id": "CP-002",
                        "display_ids": null,
                        "ntype": "node",
                        "total_amount": 3000.0,
                        "total_count": 1,
                    },
                ],
                "edges": [
                    {
                        "id": "CARD-001==CP-001",
                        "source": "CP-001",
                        "target": "CARD-001",
                        "amount": 10000.0,
                        "count": 1,
                        "out_amount": 0.0,
                        "in_amount": 10000.0,
                        "forward_amount": 10000.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "first_time": "2026-04-01 09:00:00",
                        "last_time": "2026-04-01 09:00:00",
                    },
                    {
                        "id": "CARD-001==CP-002",
                        "source": "CARD-001",
                        "target": "CP-002",
                        "amount": 3000.0,
                        "count": 1,
                        "out_amount": 3000.0,
                        "in_amount": 0.0,
                        "forward_amount": 3000.0,
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "first_time": "2026-04-01 10:00:00",
                        "last_time": "2026-04-01 10:00:00",
                    },
                ],
                "stats": {
                    "requested_depth": 1,
                    "direction": "all",
                    "seed_count": 1,
                    "node_count": 3,
                    "edge_count": 2,
                    "self_loop_edge_count": 0,
                    "total_amount": 13000.0,
                    "view_mode": "relation",
                    "context_applied": {
                        "source": "stats",
                        "request_id": "rust-flow-focus-graph",
                        "date_start": "",
                        "date_end": "",
                        "focus_only": true,
                        "focus_counterparty_strict": true,
                        "focus_id_count": 2,
                        "focus_name_count": 0,
                        "focus_unknown_name": false,
                        "include_missing_counterparty": false,
                        "focus_key_type": "account",
                        "build_mode": "stats_focus_account_fast",
                    },
                    "expected_total_amount": 13000.0,
                    "expected_total_amount_delta": 0.0,
                    "expected_row_count": 2,
                    "expected_row_count_delta": 0,
                },
            },
        })
    );
    Ok(())
}

#[test]
fn query_flow_focus_graph_cli_dedupes_replacement_card_edge_amount_and_count() -> Result<()> {
    let db = TempDb::new("flow-focus-graph-replacement-card-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_replacement_card_duplicate(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--query-seed-id",
        "CARD-003",
        "--seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-003",
        "--selected-focus-id",
        "CP-001",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-003",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-replacement-card-dedupe",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
    ])?;

    let edges = output["graph"]["edges"].as_array().expect("graph edges");
    assert_eq!(edges.len(), 1);
    assert_eq!(edges[0]["target"], json!("CARD-001"));
    assert_eq!(edges[0]["amount"], json!(10000.0));
    assert_eq!(edges[0]["count"], json!(1));
    assert_eq!(output["graph"]["stats"]["total_amount"], json!(10000.0));
    Ok(())
}

#[test]
fn query_flow_focus_graph_cli_dedupes_seed_internal_mirror_transactions() -> Result<()> {
    let db = TempDb::new("flow-focus-graph-mirror-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_detail_internal_mirror_pair(&db)?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--query-seed-id",
        "CARD-002",
        "--seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-002",
        "--selected-focus-id",
        "CARD-002",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-001",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-mirror-dedupe",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
        "--expected-total-amount",
        "1400.0",
        "--expected-row-count",
        "2",
    ])?;

    let edge = output["graph"]["edges"][0].clone();
    assert_eq!(edge["id"], json!("CARD-001==CARD-002"));
    assert_eq!(edge["amount"], json!(700.0));
    assert_eq!(edge["count"], json!(1));
    assert_eq!(output["graph"]["stats"]["total_amount"], json!(700.0));
    assert_eq!(
        output["graph"]["stats"]["expected_total_amount_delta"],
        json!(-700.0)
    );
    Ok(())
}

#[test]
fn query_flow_focus_graph_cli_writes_graph_payload_to_output_json() -> Result<()> {
    let db = TempDb::new("flow-focus-graph-output-json")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("flow-focus-graph-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let direct = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CP-002",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-001",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-graph-output",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
        "--expected-total-amount",
        "13000.0",
        "--expected-row-count",
        "2",
    ])?;
    let output = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CP-002",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-001",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-graph-output",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
        "--expected-total-amount",
        "13000.0",
        "--expected-row-count",
        "2",
        "--output-json",
        &output_path_text,
    ])?;

    assert_eq!(output["ok"], true);
    assert_eq!(output["case_id"], CASE_ID);
    assert_eq!(output["has_graph"], true);
    assert_eq!(output["node_count"], 3);
    assert_eq!(output["edge_count"], 2);
    assert_eq!(
        output["output_json"].as_str(),
        Some(output_path_text.as_str())
    );
    assert!(output.get("graph").is_none());

    let payload: Value = serde_json::from_str(&fs::read_to_string(&output_path)?)?;
    let _ = fs::remove_file(&output_path);
    assert_eq!(payload["graph"], direct["graph"]);
    Ok(())
}

fn insert_detail_internal_mirror_pair(db: &TempDb) -> Result<()> {
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "
        INSERT INTO analysis_txn_detail_idx(
          id, txn_day, acct_key, cp_key, cp_raw, cp_placeholder_kind, cp_name, dc_val,
          txn_ts, cp_name_pick, cp_name_pick_cnt, stats_name_key, txn_time, amount,
          amount_source_present, amount_parse_failed, balance, card_no, acct_no,
          account_open_name, opener_id_no, counterparty_acct,
          cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary,
          currency, branch_name, location, is_success, voucher_no, ip_addr, mac_addr,
          counterparty_balance, txn_id, log_id, voucher_type, voucher_id, teller_no,
          remark, txn_type, query_feedback_reason
        ) VALUES
        (
          6, DATE '2026-04-03', 'CARD-001', 'CARD-002', 'CARD-002', NULL, '李四',
          '出', TIMESTAMP '2026-04-03 12:00:00', '李四', 1, '李四',
          '2026-04-03 12:00:00', 700.0, 1, 0, 700.0, 'CARD-001', 'A-001',
          '张三', 'ID-A', 'CARD-002', '否', '李四', '', '测试银行',
          '内部转账', 'CNY', '一号支行', '贵阳', '成功', '', '', '',
          NULL, 'txn-internal-out', '', '', '', '', '', '', ''
        ),
        (
          7, DATE '2026-04-03', 'CARD-002', 'CARD-001', 'CARD-001', NULL, '张三',
          '进', TIMESTAMP '2026-04-03 12:00:20', '张三', 1, '张三',
          '2026-04-03 12:00:20', 700.0, 1, 0, 700.0, 'CARD-002', 'A-002',
          '李四', 'ID-B', 'CARD-001', '否', '张三', '', '测试银行',
          '内部转入', 'CNY', '一号支行', '贵阳', '成功', '', '', '',
          NULL, 'txn-internal-in', '', '', '', '', '', '', ''
        );
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())
}

#[test]
fn query_flow_focus_graph_cli_prefers_account_open_name_over_counterparty_name() -> Result<()> {
    let db = TempDb::new("flow-focus-open-name-priority")?;
    seed_stats_query_tables(db.path())?;
    insert_high_amount_seed_internal_mirror_row(&db)?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-graph",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--query-seed-id",
        "CARD-002",
        "--seed-id",
        "CARD-001",
        "--seed-id",
        "CARD-002",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CARD-001",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "all",
        "--depth",
        "1",
        "--view-mode",
        "relation",
        "--focus-id-raw",
        "CARD-001",
        "--focus-label",
        "张三",
        "--request-id",
        "rust-flow-focus-open-name",
        "--source",
        "stats",
        "--focus-key-type",
        "account",
        "--expected-total-amount",
        "30000.0",
        "--expected-row-count",
        "2",
    ])?;

    let nodes = output["graph"]["nodes"]
        .as_array()
        .expect("graph nodes must be an array");
    let card_node = nodes
        .iter()
        .find(|node| node.get("id") == Some(&json!("CARD-001")))
        .expect("CARD-001 node must be present");
    assert_eq!(card_node.get("name"), Some(&json!("张三")));
    assert_eq!(output["graph"]["stats"]["total_amount"], json!(30000.0));
    Ok(())
}

fn insert_seed_internal_mirror_row(db: &TempDb) -> Result<()> {
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "
        INSERT INTO analysis_txn_daily_agg VALUES
          (DATE '2026-04-03', 'CARD-002', 'CARD-001', 'CARD-001', 'CARD-001', NULL,
           '张三', '张三', 1, '张三', '进', 1, 1, 1, 0, 0, 700.0,
           TIMESTAMP '2026-04-03 12:00:00', TIMESTAMP '2026-04-03 12:00:00',
           '李四', '测试银行', '贵阳');
        INSERT INTO analysis_txn_detail_idx(
          id, txn_day, acct_key, cp_key, cp_raw, cp_placeholder_kind, cp_name, dc_val,
          txn_ts, cp_name_pick, cp_name_pick_cnt, stats_name_key, txn_time, amount,
          amount_source_present, amount_parse_failed, balance, card_no, acct_no,
          account_open_name, opener_id_no, counterparty_acct,
          cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary,
          currency, branch_name, location, is_success, voucher_no, ip_addr, mac_addr,
          counterparty_balance, txn_id, log_id, voucher_type, voucher_id, teller_no,
          remark, txn_type, query_feedback_reason
        ) VALUES (
          4, DATE '2026-04-03', 'CARD-002', 'CARD-001', 'CARD-001', NULL, '张三',
          '进', TIMESTAMP '2026-04-03 12:00:00', '张三', 1, '张三',
          '2026-04-03 12:00:00', 700.0, 1, 0, 700.0, 'CARD-002', 'A-002',
          '李四', 'ID-B', 'CARD-001', '否', '张三', '', '测试银行',
          '内部转入', 'CNY', '一号支行', '贵阳', '成功', '', '', '',
          NULL, 'txn-internal-in', '', '', '', '', '', '', ''
        );
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())
}

fn insert_high_amount_seed_internal_mirror_row(db: &TempDb) -> Result<()> {
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "
        INSERT INTO analysis_txn_daily_agg VALUES
          (DATE '2026-04-03', 'CARD-002', 'CARD-001', 'CARD-001', 'CARD-001', NULL,
           '旧张三', '旧张三', 1, '旧张三', '进', 1, 1, 1, 0, 0, 20000.0,
           TIMESTAMP '2026-04-03 12:00:00', TIMESTAMP '2026-04-03 12:00:00',
           '李四', '测试银行', '贵阳');
        INSERT INTO analysis_txn_detail_idx(
          id, txn_day, acct_key, cp_key, cp_raw, cp_placeholder_kind, cp_name, dc_val,
          txn_ts, cp_name_pick, cp_name_pick_cnt, stats_name_key, txn_time, amount,
          amount_source_present, amount_parse_failed, balance, card_no, acct_no,
          account_open_name, opener_id_no, counterparty_acct,
          cash_flag, counterparty_name, counterparty_id_no, counterparty_bank, summary,
          currency, branch_name, location, is_success, voucher_no, ip_addr, mac_addr,
          counterparty_balance, txn_id, log_id, voucher_type, voucher_id, teller_no,
          remark, txn_type, query_feedback_reason
        ) VALUES (
          5, DATE '2026-04-03', 'CARD-002', 'CARD-001', 'CARD-001', NULL, '旧张三',
          '进', TIMESTAMP '2026-04-03 12:00:00', '旧张三', 1, '旧张三',
          '2026-04-03 12:00:00', 20000.0, 1, 0, 20000.0, 'CARD-002', 'A-002',
          '李四', 'ID-B', 'CARD-001', '否', '旧张三', '', '测试银行',
          '内部转入', 'CNY', '一号支行', '贵阳', '成功', '', '', '',
          NULL, 'txn-high-internal-in', '', '', '', '', '', '', ''
        );
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())
}
