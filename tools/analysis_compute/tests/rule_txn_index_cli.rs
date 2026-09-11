use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde_json::Value;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, Output};
use std::time::{SystemTime, UNIX_EPOCH};

struct TempDb {
    path: PathBuf,
}

impl TempDb {
    fn new(label: &str) -> Result<Self> {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .context("system clock before UNIX_EPOCH")?
            .as_nanos();
        Ok(Self {
            path: std::env::temp_dir().join(format!(
                "analytix-{label}-{}-{unique}.duckdb",
                std::process::id()
            )),
        })
    }

    fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for TempDb {
    fn drop(&mut self) {
        let _ = fs::remove_file(&self.path);
        let mut wal_path = self.path.as_os_str().to_os_string();
        wal_path.push(".wal");
        let _ = fs::remove_file(PathBuf::from(wal_path));
    }
}

#[test]
fn rule_txn_index_v2_is_case_content_and_missing_value_bound() -> Result<()> {
    let db = TempDb::new("rule-txn-index-v2")?;
    let conn = Connection::open(db.path()).context("open rule transaction fixture")?;
    conn.execute_batch(
        "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
         INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7); \
         CREATE TABLE fc_transaction_norm( \
           id BIGINT, case_id TEXT, acct_no TEXT, txn_id TEXT, txn_time TEXT, \
           amount TEXT, balance TEXT, clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
         ); \
         INSERT INTO fc_transaction_norm VALUES \
           (1, 'case-a', 'acct-a', 'txn-a', '2026-04-11 13:00:00', NULL, NULL, 0, 0, 0), \
           (1, 'case-b', 'acct-b', 'txn-b', '2026-04-11 13:00:00', '0', '0', 0, 0, 0);",
    )?;
    drop(conn);

    let first = run_success(db.path(), "case-a", true)?;
    assert_eq!(first["agg_name"], "rule_txn_index:v2");
    assert_eq!(first["agg_version"], 2);
    assert_eq!(first["rebuilt"], true);
    let conn = Connection::open(db.path())?;
    let values = conn.query_row(
        "SELECT case_id, amount_val, balance_val FROM analysis_rule_txn_idx",
        [],
        |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, Option<f64>>(1)?,
                row.get::<_, Option<f64>>(2)?,
            ))
        },
    )?;
    assert_eq!(values, ("case-a".to_string(), None, None));
    let first_signature = source_signature(&conn)?;
    let first_result_signature = result_signature(&conn)?;
    conn.execute(
        "UPDATE analysis_rule_txn_idx SET account_name='tampered' WHERE case_id='case-a'",
        [],
    )?;
    drop(conn);

    let repaired = run_success(db.path(), "case-a", false)?;
    assert_eq!(repaired["rebuilt"], true);
    let conn = Connection::open(db.path())?;
    assert_eq!(
        conn.query_row(
            "SELECT account_name FROM analysis_rule_txn_idx WHERE case_id='case-a'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        "acct-a"
    );
    assert_eq!(result_signature(&conn)?, first_result_signature);
    conn.execute(
        "UPDATE fc_transaction_norm SET amount='12.34', balance='0' WHERE case_id='case-a'",
        [],
    )?;
    drop(conn);

    let changed = run_success(db.path(), "case-a", false)?;
    assert_eq!(changed["rebuilt"], true);
    let conn = Connection::open(db.path())?;
    assert_ne!(source_signature(&conn)?, first_signature);
    let changed_values = conn.query_row(
        "SELECT amount_val, balance_val FROM analysis_rule_txn_idx",
        [],
        |row| Ok((row.get::<_, f64>(0)?, row.get::<_, f64>(1)?)),
    )?;
    assert_eq!(changed_values, (12.34, 0.0));
    drop(conn);

    let other_case = run_success(db.path(), "case-b", false)?;
    assert_eq!(other_case["rebuilt"], true);
    let conn = Connection::open(db.path())?;
    let indexed_case = conn.query_row("SELECT case_id FROM analysis_rule_txn_idx", [], |row| {
        row.get::<_, String>(0)
    })?;
    assert_eq!(indexed_case, "case-b");
    Ok(())
}

#[test]
fn rule_txn_index_rejects_unknown_cleaning_state_before_cache_acceptance() -> Result<()> {
    let db = TempDb::new("rule-txn-index-clean-state")?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
         INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1); \
         CREATE TABLE fc_transaction_norm( \
           id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
           clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
         ); \
         INSERT INTO fc_transaction_norm VALUES (1, 'case-a', 'acct-a', '1', 0, NULL, 0);",
    )?;
    drop(conn);
    let output = run_command(db.path(), "case-a", false)?;
    assert!(!output.status.success());
    assert!(output.stdout.is_empty());
    Ok(())
}

#[test]
fn rule_txn_index_rejects_nonempty_source_without_account_lineage() -> Result<()> {
    let db = TempDb::new("rule-txn-index-account-lineage")?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
         INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1); \
         CREATE TABLE fc_transaction_norm( \
           id BIGINT, case_id TEXT, amount TEXT, \
           clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
         ); \
         INSERT INTO fc_transaction_norm VALUES (1, 'case-a', '1', 0, 0, 0);",
    )?;
    drop(conn);
    let output = run_command(db.path(), "case-a", false)?;
    assert!(!output.status.success());
    assert!(output.stdout.is_empty());
    Ok(())
}

#[test]
fn empty_rule_txn_index_rebuilds_when_zero_valued_metadata_is_null() -> Result<()> {
    for column in ["source_row_count", "source_max_id"] {
        let db = TempDb::new(&format!("rule-txn-index-null-{column}"))?;
        let conn = Connection::open(db.path())?;
        conn.execute_batch(
            "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
             INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id TEXT, acct_no TEXT, amount TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             );",
        )?;
        drop(conn);

        assert_eq!(run_success(db.path(), "case-a", false)?["rebuilt"], true);
        let conn = Connection::open(db.path())?;
        conn.execute_batch(&format!(
            "UPDATE analysis_materialization_meta SET {column}=NULL \
               WHERE agg_name='rule_txn_index:v2'"
        ))?;
        drop(conn);

        let repaired = run_success(db.path(), "case-a", false)?;
        assert_eq!(
            repaired["rebuilt"], true,
            "NULL {column} must invalidate the cache"
        );
        let conn = Connection::open(db.path())?;
        let repaired_value = conn.query_row(
            &format!(
                "SELECT {column} FROM analysis_materialization_meta \
                  WHERE agg_name='rule_txn_index:v2'"
            ),
            [],
            |row| row.get::<_, i64>(0),
        )?;
        assert_eq!(repaired_value, 0);
    }
    Ok(())
}

fn source_signature(conn: &Connection) -> Result<String> {
    Ok(conn.query_row(
        "SELECT source_signature FROM analysis_materialization_meta WHERE agg_name='rule_txn_index:v2'",
        [],
        |row| row.get::<_, String>(0),
    )?)
}

fn result_signature(conn: &Connection) -> Result<String> {
    Ok(conn.query_row(
        "SELECT result_signature FROM analysis_materialization_meta WHERE agg_name='rule_txn_index:v2'",
        [],
        |row| row.get::<_, String>(0),
    )?)
}

fn run_success(path: &Path, case_id: &str, force: bool) -> Result<Value> {
    let output = run_command(path, case_id, force)?;
    if !output.status.success() {
        bail!(
            "materialize-rule-txn-index failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    serde_json::from_slice(&output.stdout).context("parse rule transaction output")
}

fn run_command(path: &Path, case_id: &str, force: bool) -> Result<Output> {
    let mut command = Command::new(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    command
        .arg("materialize-rule-txn-index")
        .arg("--case-id")
        .arg(case_id)
        .arg("--db-path")
        .arg(path);
    if force {
        command.arg("--force").arg("true");
    }
    command.output().context("run materialize-rule-txn-index")
}
