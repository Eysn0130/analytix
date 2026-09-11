use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::json;
use std::path::Path;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_tree_cli_returns_by_name_tree_from_account_dim() -> Result<()> {
    let db = TempDb::new("stats-tree-by-name")?;
    seed_account_dim(db.path().to_string_lossy().as_ref())?;

    let output = run_analysis_compute_command(&[
        "query-stats-tree",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--tab",
        "byName",
    ])?;

    let groups = output["groups"]
        .as_array()
        .context("groups should be array")?;
    assert_eq!(groups[0]["title"], json!("张三"));
    assert_eq!(groups[0]["meta"], json!("ID-A"));
    assert_eq!(groups[0]["items"][0]["sub"], json!("贵州银行 · 活期结算"));
    assert_eq!(groups[0]["items"][1]["sub"], json!("工商银行 · II类"));
    let unnamed_group = groups
        .iter()
        .find(|group| group["title"] == json!("CARD-003"))
        .context("CARD-003 unnamed group should exist")?;
    assert_eq!(unnamed_group["meta"], json!("未登记户名"));
    assert_eq!(unnamed_group["items"][0]["sub"], json!(""));
    let unknown_bank_group = groups
        .iter()
        .find(|group| group["title"] == json!("6214600780000579708"))
        .context("account without source-backed bank should remain present")?;
    assert_eq!(unknown_bank_group["items"][0]["sub"], json!(""));
    Ok(())
}

#[test]
fn query_stats_tree_cli_applies_manual_mapping_without_account_number_bank_inference() -> Result<()>
{
    let db = TempDb::new("stats-tree-manual")?;
    seed_account_dim(db.path().to_string_lossy().as_ref())?;
    insert_manual_mapping(db.path().to_string_lossy().as_ref())?;

    let by_name = run_analysis_compute_command(&[
        "query-stats-tree",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--tab",
        "byName",
    ])?;
    let groups = by_name["groups"]
        .as_array()
        .context("groups should be array")?;
    let manual_group = groups
        .iter()
        .find(|group| group["title"] == json!("人工户名"))
        .context("manual group should exist")?;
    assert_eq!(manual_group["meta"], json!("ID-M"));
    assert_eq!(manual_group["items"][0]["id"], json!("CARD-003"));
    assert_eq!(manual_group["items"][0]["sub"], json!("建设银行"));

    let by_card = run_analysis_compute_command(&[
        "query-stats-tree",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--tab",
        "byCard",
    ])?;
    let bank_titles = by_card["groups"]
        .as_array()
        .context("byCard groups should be array")?
        .iter()
        .map(|group| group["title"].as_str().unwrap_or_default().to_string())
        .collect::<Vec<_>>();
    assert!(bank_titles.contains(&"贵州银行".to_string()));
    assert!(bank_titles.contains(&"建设银行".to_string()));
    let unknown_bank_group = by_card["groups"]
        .as_array()
        .context("byCard groups should be array")?
        .iter()
        .find(|group| group["title"] == json!("未知开户行"))
        .context("missing bank evidence should stay unknown")?;
    assert!(unknown_bank_group["items"]
        .as_array()
        .context("unknown-bank items should be array")?
        .iter()
        .any(|item| item["id"] == json!("6214600780000579708")));
    Ok(())
}

fn seed_account_dim(path: &str) -> Result<()> {
    seed_stats_query_tables(Path::new(path))?;
    let conn = Connection::open(path).context("open DuckDB for tree account dim")?;
    conn.execute_batch(
        "
        DELETE FROM analysis_account_dim;
        INSERT INTO analysis_account_dim VALUES
          ('CARD-001', 'CARD-001', 'CARD-001', '张三', 'ID-A', '贵州银行股份有限公司', '贵阳分行', '个人人民币活期普通结算账户'),
          ('CARD-002', 'CARD-002', 'CARD-002', '张三', 'ID-A', '中国工商银行', '贵阳分行', 'II类账户'),
          ('CARD-003', 'CARD-003', 'CARD-003', '', '', '未录入归属信息', '', 'ABC123'),
          ('6214600780000579708', '6214600780000579708', '6214600780000579708', '', '', '', '', '');
        INSERT INTO analysis_txn_detail_idx
        SELECT * REPLACE (
          4 AS id, 'CARD-003' AS acct_key, 'CARD-003' AS card_no,
          'CARD-003' AS acct_no, '' AS account_open_name, '' AS opener_id_no,
          'txn-tree-card-003' AS txn_id
        ) FROM analysis_txn_detail_idx WHERE id=1;
        INSERT INTO analysis_txn_detail_idx
        SELECT * REPLACE (
          5 AS id, '6214600780000579708' AS acct_key,
          '6214600780000579708' AS card_no, '6214600780000579708' AS acct_no,
          '' AS account_open_name, '' AS opener_id_no,
          'txn-tree-account-unknown-bank' AS txn_id
        ) FROM analysis_txn_detail_idx WHERE id=1;
        ",
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(Path::new(path))
}

fn insert_manual_mapping(path: &str) -> Result<()> {
    let conn = Connection::open(path).context("open DuckDB for tree manual mapping")?;
    conn.execute_batch(
        "
        CREATE TABLE analysis_manual_account_mapping(
          case_id TEXT,
          account_key TEXT,
          account_open_name TEXT,
          opener_id_no TEXT,
          bank_name TEXT
        );
        INSERT INTO analysis_manual_account_mapping VALUES
          ('case-stats-query-golden', 'CARD-003', '人工户名', 'ID-M', '中国建设银行股份有限公司');
        ",
    )?;
    Ok(())
}
