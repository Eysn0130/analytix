use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{SystemTime, UNIX_EPOCH};

const CASE_ID: &str = "case-rule-pattern-golden";
const PARAM_SIGNATURE: &str = "sig-rule-pattern-golden";

struct TempDb {
    path: PathBuf,
}

impl TempDb {
    fn new(label: &str) -> Result<Self> {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .context("system clock before UNIX_EPOCH")?
            .as_nanos();
        let path = std::env::temp_dir().join(format!(
            "analytix-{label}-{}-{unique}.duckdb",
            std::process::id()
        ));
        Ok(Self { path })
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

#[derive(Debug)]
struct FeatureRow {
    case_id: String,
    param_signature: String,
    feature_code: String,
    account_key: String,
    direction: String,
    txn_count: i64,
    total_amount: f64,
    txn_ids_json: String,
    amounts_json: String,
    detail_json: String,
    first_time: String,
    last_time: String,
    min_amount: f64,
    min_count: i64,
    min_total_amount: f64,
}

#[test]
fn materialize_rule_pattern_index_cli_writes_small_fast_feature_and_current_meta() -> Result<()> {
    let db = TempDb::new("rule-pattern-index")?;
    let source_result_signature = seed_rule_txn_index(db.path())?;

    let first_output = run_materialize_rule_pattern_index(db.path(), true)?;
    assert_eq!(first_output["ok"], true);
    assert_eq!(first_output["case_id"], CASE_ID);
    assert_eq!(first_output["row_count"], 1);
    assert_eq!(first_output["rebuilt"], true);
    assert_eq!(first_output["agg_name"], "rule_pattern_index:v9");
    assert_eq!(first_output["agg_version"], 9);
    assert_eq!(first_output["input_coverage"]["total_rows"], 3);
    assert_eq!(first_output["input_coverage"]["accepted_rows"], 3);
    assert_eq!(first_output["input_coverage"]["rejected_rows"], 0);
    assert_eq!(first_output["input_coverage"]["amount_covered_rows"], 3);
    assert_eq!(first_output["input_coverage"]["cash_covered_rows"], 3);
    assert_eq!(first_output["input_coverage"]["cash_unknown_rows"], 0);
    assert_eq!(first_output["input_coverage"]["cash_conflict_rows"], 0);
    assert_eq!(
        first_output["feature_readiness"],
        json!({
            "cash_dependent_rules": {
                "status": "complete",
                "requested_rows": 3,
                "eligible_rows": 3,
                "unknown_cash_rows": 0,
                "conflict_cash_rows": 0,
                "blocker": null,
            },
            "cash_independent_rules": {
                "status": "complete",
                "requested_rows": 3,
                "eligible_rows": 3,
                "blocker": null,
            },
        })
    );

    let conn = Connection::open(db.path()).context("open golden DuckDB after materialize")?;
    let feature = read_single_feature(&conn)?;
    assert_eq!(feature.case_id, CASE_ID);
    assert_eq!(feature.param_signature, PARAM_SIGNATURE);
    assert_eq!(feature.feature_code, "SMALL_FAST_EVENT");
    assert_eq!(feature.account_key, "A-004");
    assert_eq!(feature.direction, "");
    assert_eq!(feature.txn_count, 3);
    assert_eq!(feature.total_amount, 4000.0);
    assert_eq!(
        feature.txn_ids_json,
        "[\"txn-small-fast-in-1\",\"txn-small-fast-out-1\",\"txn-small-fast-out-2\"]"
    );
    assert_eq!(feature.amounts_json, "[]");
    assert_eq!(feature.first_time, "2026-04-11 13:00:00");
    assert_eq!(feature.last_time, "2026-04-11 13:10:00");
    assert_eq!(feature.min_amount, 1000.0);
    assert_eq!(feature.min_count, 1);
    assert_eq!(feature.min_total_amount, 0.0);
    let detail = serde_json::from_str::<Value>(&feature.detail_json)?;
    assert_eq!(
        detail,
        json!({
            "in_txn_id": "txn-small-fast-in-1",
            "out_txn_ids": ["txn-small-fast-out-1", "txn-small-fast-out-2"],
            "in_amount": 4000.0,
            "out_amount": 3200.0,
            "out_ratio": 0.8,
            "duration_minutes": 10.0,
            "start_time": "2026-04-11 13:00:00",
            "end_time": "2026-04-11 13:10:00",
        })
    );
    let meta = conn.query_row(
        "
        SELECT agg_version, source_row_count, source_max_id,
               COALESCE(source_signature, ''), COALESCE(source_parameter_signature, ''),
               COALESCE(result_signature, ''), row_count
          FROM analysis_materialization_meta
         WHERE agg_name='rule_pattern_index:v9'
        ",
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, String>(4)?,
                row.get::<_, String>(5)?,
                row.get::<_, i64>(6)?,
            ))
        },
    )?;
    assert_eq!(
        meta,
        (
            9,
            3,
            3,
            source_result_signature,
            PARAM_SIGNATURE.to_string(),
            meta.5.clone(),
            1
        )
    );
    assert_eq!(meta.5.len(), 64);
    drop(conn);

    let second_output = run_materialize_rule_pattern_index(db.path(), false)?;
    assert_eq!(second_output["row_count"], 1);
    assert_eq!(second_output["rebuilt"], false);

    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_rule_pattern_idx SET total_amount=total_amount+1 WHERE case_id=?",
        [CASE_ID],
    )?;
    drop(conn);
    let repaired_output = run_materialize_rule_pattern_index(db.path(), false)?;
    assert_eq!(repaired_output["rebuilt"], true);
    let conn = Connection::open(db.path())?;
    assert_eq!(read_single_feature(&conn)?.total_amount, 4000.0);

    Ok(())
}

#[test]
fn materialize_rule_pattern_index_rejects_incomplete_required_field_coverage_without_publish(
) -> Result<()> {
    for (label, mutation, missing_field) in [
        (
            "missing-account",
            "UPDATE analysis_rule_txn_idx SET account_key=NULL WHERE row_id='2'",
            "account_key=2",
        ),
        (
            "invalid-account",
            "UPDATE analysis_rule_txn_idx SET account_key='unknown' WHERE row_id='2'",
            "account_key=2",
        ),
        (
            "missing-transaction-id",
            "UPDATE analysis_rule_txn_idx SET txn_id=NULL, row_id=NULL WHERE txn_id='txn-small-fast-out-1'",
            "transaction_id=2",
        ),
        (
            "invalid-transaction-identifiers",
            "UPDATE analysis_rule_txn_idx SET txn_id='unknown', row_id='--' WHERE row_id='2'",
            "transaction_id=2",
        ),
        (
            "missing-time",
            "UPDATE analysis_rule_txn_idx SET txn_time=NULL, txn_ts_val=NULL WHERE row_id='2'",
            "transaction_time=2",
        ),
        (
            "missing-amount",
            "UPDATE analysis_rule_txn_idx SET amount_val=NULL WHERE row_id='2'",
            "amount=2",
        ),
        (
            "nonfinite-amount",
            "UPDATE analysis_rule_txn_idx SET amount_val=CAST('NaN' AS DOUBLE) WHERE row_id='2'",
            "amount=2",
        ),
        (
            "unroundable-amount",
            "UPDATE analysis_rule_txn_idx SET amount_val=1.7976931348623157E308 WHERE row_id='2'",
            "amount=2",
        ),
        (
            "invalid-direction",
            "UPDATE analysis_rule_txn_idx SET direction_raw='sideways' WHERE row_id='2'",
            "direction=2",
        ),
    ] {
        let db = TempDb::new(label)?;
        seed_rule_txn_index(db.path())?;
        run_materialize_rule_pattern_index(db.path(), true)?;
        let conn = Connection::open(db.path())?;
        let published_signature = canonical_table_signature(
            &conn,
            "analysis_rule_pattern_idx",
            CASE_ID,
            "analytix.rule-pattern-index-result/v9",
            "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
        )?;
        let published_meta = rule_pattern_meta_json(&conn)?;
        conn.execute_batch(mutation)?;
        refresh_rule_txn_result_signature(&conn)?;
        drop(conn);

        let failure = run_materialize_rule_pattern_index_failure(db.path(), true, &[])?;
        assert!(
            failure.contains("rule_pattern_required_field_coverage_incomplete"),
            "{label}: {failure}"
        );
        assert!(failure.contains("total=3:accepted=2:rejected=1"), "{label}: {failure}");
        assert!(failure.contains(missing_field), "{label}: {failure}");

        let conn = Connection::open(db.path())?;
        assert!(!table_exists(&conn, "analysis_rule_pattern_idx__staging")?);
        assert_eq!(rule_pattern_meta_json(&conn)?, published_meta);
        assert_eq!(
            canonical_table_signature(
                &conn,
                "analysis_rule_pattern_idx",
                CASE_ID,
                "analytix.rule-pattern-index-result/v9",
                "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
            )?,
            published_signature
        );
    }
    Ok(())
}

#[test]
fn incomplete_source_never_creates_pattern_index_staging_or_meta() -> Result<()> {
    let db = TempDb::new("rule-pattern-no-first-publish")?;
    seed_rule_txn_index(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_rule_txn_idx SET amount_val=NULL WHERE row_id='1'",
        [],
    )?;
    refresh_rule_txn_result_signature(&conn)?;
    drop(conn);

    let failure = run_materialize_rule_pattern_index_failure(db.path(), true, &[])?;
    assert!(failure.contains("rule_pattern_required_field_coverage_incomplete"));

    let conn = Connection::open(db.path())?;
    assert!(!table_exists(&conn, "analysis_rule_pattern_idx")?);
    assert!(!table_exists(&conn, "analysis_rule_pattern_idx__staging")?);
    let meta_count = conn.query_row(
        "SELECT COUNT(1) FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v9'",
        [],
        |row| row.get::<_, i64>(0),
    )?;
    assert_eq!(meta_count, 0);
    Ok(())
}

#[test]
fn optional_description_fields_do_not_reduce_coverage_and_real_zero_remains_present() -> Result<()>
{
    let db = TempDb::new("rule-pattern-optional-descriptions")?;
    seed_rule_txn_index(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "UPDATE analysis_rule_txn_idx
            SET amount_val=0, cash_raw=NULL, summary=NULL, txn_type=NULL, remark=NULL, voucher_type=NULL
          WHERE row_id='2'",
    )?;
    refresh_rule_txn_result_signature(&conn)?;
    drop(conn);

    let output = run_materialize_rule_pattern_index(db.path(), true)?;
    assert_eq!(output["input_coverage"]["total_rows"], 3);
    assert_eq!(output["input_coverage"]["accepted_rows"], 3);
    assert_eq!(output["input_coverage"]["amount_covered_rows"], 3);
    assert_eq!(output["input_coverage"]["rejected_rows"], 0);
    assert_eq!(output["input_coverage"]["cash_covered_rows"], 2);
    assert_eq!(output["input_coverage"]["cash_unknown_rows"], 1);
    assert_eq!(output["input_coverage"]["cash_conflict_rows"], 0);
    assert_eq!(
        output["feature_readiness"]["cash_dependent_rules"],
        json!({
            "status": "partial",
            "requested_rows": 3,
            "eligible_rows": 2,
            "unknown_cash_rows": 1,
            "conflict_cash_rows": 0,
            "blocker": "cash_classification_coverage_incomplete",
        })
    );
    assert_eq!(
        output["feature_readiness"]["cash_independent_rules"]["status"],
        "complete"
    );
    assert_eq!(output["row_count"], 0);

    let conn = Connection::open(db.path())?;
    assert_eq!(
        conn.query_row(
            "SELECT amount_val FROM analysis_rule_txn_idx WHERE row_id='2'",
            [],
            |row| row.get::<_, f64>(0),
        )?,
        0.0
    );
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE feature_code IN ('SMALL_FAST_EVENT', 'CASH_QUICK_EVENT', 'CASH_QUICK_CANDIDATE_EVENT')",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    Ok(())
}

#[test]
fn conflicting_cash_cue_is_partial_and_never_materializes_cash_dependent_features() -> Result<()> {
    let db = TempDb::new("rule-pattern-cash-conflict")?;
    seed_rule_txn_index(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_rule_txn_idx SET cash_raw='0', summary='现金支取' WHERE row_id='2'",
        [],
    )?;
    refresh_rule_txn_result_signature(&conn)?;
    drop(conn);

    let output = run_materialize_rule_pattern_index(db.path(), true)?;
    assert_eq!(output["input_coverage"]["cash_covered_rows"], 2);
    assert_eq!(output["input_coverage"]["cash_unknown_rows"], 0);
    assert_eq!(output["input_coverage"]["cash_conflict_rows"], 1);
    assert_eq!(
        output["feature_readiness"]["cash_dependent_rules"]["status"],
        "partial"
    );
    assert_eq!(
        output["feature_readiness"]["cash_dependent_rules"]["blocker"],
        "cash_classification_coverage_incomplete"
    );

    let conn = Connection::open(db.path())?;
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE feature_code IN ('SMALL_FAST_EVENT', 'CASH_QUICK_EVENT', 'CASH_QUICK_CANDIDATE_EVENT')",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    Ok(())
}

#[test]
fn incomplete_cash_coverage_does_not_suppress_cash_independent_features() -> Result<()> {
    let db = TempDb::new("rule-pattern-independent-with-cash-partial")?;
    seed_rule_txn_index(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "UPDATE analysis_rule_txn_idx
            SET direction_raw='CREDIT', amount_val=10000 * CAST(row_id AS BIGINT);
         UPDATE analysis_rule_txn_idx
            SET cash_raw=NULL, summary=NULL, txn_type=NULL, remark=NULL, voucher_type=NULL
          WHERE row_id='2';",
    )?;
    refresh_rule_txn_result_signature(&conn)?;
    drop(conn);

    let output = run_materialize_rule_pattern_index(db.path(), true)?;
    assert_eq!(
        output["feature_readiness"]["cash_dependent_rules"]["status"],
        "partial"
    );
    assert_eq!(
        output["feature_readiness"]["cash_independent_rules"]["status"],
        "complete"
    );

    let conn = Connection::open(db.path())?;
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE feature_code='ROUND_AMOUNT_PATTERN'",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        1
    );
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE feature_code IN ('SMALL_FAST_EVENT', 'CASH_QUICK_EVENT', 'CASH_QUICK_CANDIDATE_EVENT')",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    Ok(())
}

#[test]
fn legacy_v8_cash_classifier_cache_is_never_reused() -> Result<()> {
    let db = TempDb::new("rule-pattern-v8-cache-retirement")?;
    seed_rule_txn_index(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute("UPDATE analysis_rule_txn_idx SET cash_raw='noncash'", [])?;
    refresh_rule_txn_result_signature(&conn)?;
    drop(conn);

    run_materialize_rule_pattern_index(db.path(), true)?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_rule_pattern_idx SET feature_code='CASH_QUICK_EVENT'",
        [],
    )?;
    let legacy_result_signature = canonical_table_signature(
        &conn,
        "analysis_rule_pattern_idx",
        CASE_ID,
        "analytix.rule-pattern-index-result/v8",
        "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
    )?;
    conn.execute(
        "UPDATE analysis_materialization_meta
            SET agg_name='rule_pattern_index:v8', agg_version=8, result_signature=?
          WHERE agg_name='rule_pattern_index:v9'",
        [legacy_result_signature],
    )?;
    drop(conn);

    let output = run_materialize_rule_pattern_index(db.path(), false)?;
    assert_eq!(output["rebuilt"], true);
    assert_eq!(output["agg_name"], "rule_pattern_index:v9");
    assert_eq!(output["agg_version"], 9);

    let conn = Connection::open(db.path())?;
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE feature_code='CASH_QUICK_EVENT'",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v8'",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        0
    );
    assert_eq!(
        conn.query_row(
            "SELECT COUNT(1) FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v9' AND agg_version=9",
            [],
            |row| row.get::<_, i64>(0),
        )?,
        1
    );
    Ok(())
}

#[test]
fn metadata_failure_rolls_back_the_pattern_index_swap() -> Result<()> {
    let db = TempDb::new("rule-pattern-meta-rollback")?;
    seed_rule_txn_index(db.path())?;
    run_materialize_rule_pattern_index(db.path(), true)?;
    let conn = Connection::open(db.path())?;
    let published_signature = canonical_table_signature(
        &conn,
        "analysis_rule_pattern_idx",
        CASE_ID,
        "analytix.rule-pattern-index-result/v9",
        "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
    )?;
    let published_meta_signature = conn.query_row(
        "SELECT result_signature FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v9'",
        [],
        |row| row.get::<_, String>(0),
    )?;
    conn.execute_batch(
        "ALTER TABLE analysis_materialization_meta DROP COLUMN source_parameter_signature;
         ALTER TABLE analysis_materialization_meta ADD COLUMN source_parameter_signature BIGINT",
    )?;
    drop(conn);

    let failure = run_materialize_rule_pattern_index_failure(db.path(), true, &[])?;
    assert!(
        failure.contains("source_parameter_signature") || failure.contains("Conversion Error"),
        "stderr={failure}"
    );

    let conn = Connection::open(db.path())?;
    assert_eq!(
        canonical_table_signature(
            &conn,
            "analysis_rule_pattern_idx",
            CASE_ID,
            "analytix.rule-pattern-index-result/v9",
            "r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
        )?,
        published_signature
    );
    assert_eq!(
        conn.query_row(
            "SELECT result_signature FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v9'",
            [],
            |row| row.get::<_, String>(0),
        )?,
        published_meta_signature
    );
    Ok(())
}

#[test]
fn invalid_and_duplicate_cli_parameters_fail_before_database_mutation() -> Result<()> {
    for (label, extra_args, expected) in [
        (
            "nan",
            vec!["--round-unit", "NaN"],
            "--round-unit must be finite",
        ),
        (
            "out-of-range",
            vec!["--night-start-hour", "24"],
            "--night-start-hour must be between 0 and 23",
        ),
        (
            "duplicate",
            vec!["--round-unit", "100", "--round-unit", "200"],
            "duplicate argument: --round-unit",
        ),
    ] {
        let db = TempDb::new(label)?;
        seed_rule_txn_index(db.path())?;
        let failure = run_materialize_rule_pattern_index_failure(db.path(), true, &extra_args)?;
        assert!(failure.contains(expected), "{label}: {failure}");
        let conn = Connection::open(db.path())?;
        assert!(!table_exists(&conn, "analysis_rule_pattern_idx")?);
        assert!(!table_exists(&conn, "analysis_rule_pattern_idx__staging")?);
        let meta_count = conn.query_row(
            "SELECT COUNT(1) FROM analysis_materialization_meta WHERE agg_name='rule_pattern_index:v9'",
            [],
            |row| row.get::<_, i64>(0),
        )?;
        assert_eq!(meta_count, 0);
    }
    Ok(())
}

fn seed_rule_txn_index(path: &Path) -> Result<String> {
    let conn = Connection::open(path).context("open golden DuckDB for seed")?;
    conn.execute_batch(
        "
        CREATE TABLE analysis_rule_txn_idx(
          case_id TEXT,
          row_id TEXT,
          account_key TEXT,
          txn_id TEXT,
          txn_time TEXT,
          txn_ts_val TIMESTAMP,
          amount_val DOUBLE,
          direction_raw TEXT,
          cash_raw TEXT,
          summary TEXT,
          txn_type TEXT,
          remark TEXT,
          voucher_type TEXT
        );
        INSERT INTO analysis_rule_txn_idx VALUES
          ('case-rule-pattern-golden', '1', 'A-004', 'txn-small-fast-in-1',
           '2026-04-11 13:00:00', TIMESTAMP '2026-04-11 13:00:00',
           4000.0, 'CREDIT', '0', 'small incoming', 'transfer', '', ''),
          ('case-rule-pattern-golden', '2', 'A-004', 'txn-small-fast-out-1',
           '2026-04-11 13:05:00', TIMESTAMP '2026-04-11 13:05:00',
           1500.0, 'DEBIT', '0', 'small outgoing', 'transfer', '', ''),
          ('case-rule-pattern-golden', '3', 'A-004', 'txn-small-fast-out-2',
           '2026-04-11 13:10:00', TIMESTAMP '2026-04-11 13:10:00',
           1700.0, 'DEBIT', '0', 'small outgoing', 'transfer', '', '');
        CREATE TABLE analysis_materialization_meta(
          agg_name TEXT PRIMARY KEY,
          agg_version INTEGER NOT NULL,
          case_id TEXT,
          source_revision BIGINT NOT NULL,
          source_row_count BIGINT,
          source_max_txn_ts TIMESTAMP,
          source_max_id BIGINT,
          source_signature TEXT,
          source_parameter_signature TEXT,
          result_signature TEXT,
          built_at TIMESTAMP,
          row_count BIGINT DEFAULT 0
        );
        ",
    )?;
    let result_signature = canonical_table_signature(
        &conn,
        "analysis_rule_txn_idx",
        CASE_ID,
        "analytix.rule-transaction-index-result/v2",
        "TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
    )?;
    conn.execute(
        "INSERT INTO analysis_materialization_meta( \
           agg_name, agg_version, case_id, source_revision, source_row_count, \
           source_max_txn_ts, source_max_id, source_signature, result_signature, built_at, row_count \
         ) VALUES ('rule_txn_index:v2', 2, ?, 1, 3, TIMESTAMP '2026-04-11 13:10:00', 3, ?, ?, NOW(), 3)",
        duckdb::params![CASE_ID, "a".repeat(64), result_signature],
    )?;
    Ok(result_signature)
}

fn canonical_table_signature(
    conn: &Connection,
    table: &str,
    case_id: &str,
    domain: &str,
    order_by: &str,
) -> Result<String> {
    let mut hasher = Sha256::new();
    hash_framed(&mut hasher, domain.as_bytes());
    hash_framed(&mut hasher, case_id.as_bytes());
    let mut columns = conn.prepare(&format!(
        "SELECT column_name FROM information_schema.columns \
         WHERE table_schema='main' AND table_name='{table}' ORDER BY ordinal_position"
    ))?;
    let column_rows = columns.query_map([], |row| row.get::<_, String>(0))?;
    for column in column_rows {
        hash_framed(&mut hasher, column?.as_bytes());
    }
    let mut rows = conn.prepare(&format!(
        "SELECT to_json(r) FROM {table} r WHERE r.case_id=? ORDER BY {order_by}, to_json(r)"
    ))?;
    let result_rows = rows.query_map([case_id], |row| row.get::<_, String>(0))?;
    for row in result_rows {
        hash_framed(&mut hasher, row?.as_bytes());
    }
    Ok(format!("{:x}", hasher.finalize()))
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

fn refresh_rule_txn_result_signature(conn: &Connection) -> Result<String> {
    let signature = canonical_table_signature(
        conn,
        "analysis_rule_txn_idx",
        CASE_ID,
        "analytix.rule-transaction-index-result/v2",
        "TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
    )?;
    conn.execute(
        "UPDATE analysis_materialization_meta
            SET result_signature=?, row_count=(SELECT COUNT(1) FROM analysis_rule_txn_idx WHERE case_id=?)
          WHERE agg_name='rule_txn_index:v2'",
        duckdb::params![signature, CASE_ID],
    )?;
    Ok(signature)
}

fn rule_pattern_meta_json(conn: &Connection) -> Result<String> {
    conn.query_row(
        "SELECT to_json(m) FROM analysis_materialization_meta m WHERE agg_name='rule_pattern_index:v9'",
        [],
        |row| row.get::<_, String>(0),
    )
    .context("read rule pattern metadata")
}

fn table_exists(conn: &Connection, table_name: &str) -> Result<bool> {
    let count = conn.query_row(
        "SELECT COUNT(1) FROM information_schema.tables WHERE table_schema='main' AND table_name=?",
        [table_name],
        |row| row.get::<_, i64>(0),
    )?;
    Ok(count == 1)
}

fn materialize_rule_pattern_command(path: &Path, force: bool) -> Command {
    let binary = PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let mut command = Command::new(binary);
    command
        .arg("materialize-rule-pattern-index")
        .arg("--case-id")
        .arg(CASE_ID)
        .arg("--db-path")
        .arg(path)
        .arg("--param-signature")
        .arg(PARAM_SIGNATURE);
    if force {
        command.arg("--force").arg("true");
    }
    command
}

fn run_materialize_rule_pattern_index_failure(
    path: &Path,
    force: bool,
    extra_args: &[&str],
) -> Result<String> {
    let mut command = materialize_rule_pattern_command(path, force);
    command.args(extra_args);
    let output = command
        .output()
        .context("run failing rule pattern materialization")?;
    if output.status.success() {
        bail!(
            "materialize-rule-pattern-index unexpectedly succeeded: {}",
            String::from_utf8_lossy(&output.stdout)
        );
    }
    Ok(format!(
        "{}{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    ))
}

fn run_materialize_rule_pattern_index(path: &Path, force: bool) -> Result<Value> {
    let mut command = materialize_rule_pattern_command(path, force);
    command
        .arg("--round-unit")
        .arg("10000")
        .arg("--round-min-amount")
        .arg("10000")
        .arg("--round-min-count")
        .arg("3")
        .arg("--small-fast-window-minutes")
        .arg("60")
        .arg("--small-fast-ratio")
        .arg("0.75")
        .arg("--small-fast-min-amount")
        .arg("1000")
        .arg("--small-fast-max-amount")
        .arg("50000");
    let output = command
        .output()
        .context("run materialize-rule-pattern-index")?;
    if !output.status.success() {
        bail!(
            "materialize-rule-pattern-index failed: stdout={} stderr={}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }
    let stdout = String::from_utf8(output.stdout).context("decode command stdout")?;
    serde_json::from_str(stdout.trim()).context("parse command stdout JSON")
}

fn read_single_feature(conn: &Connection) -> Result<FeatureRow> {
    conn.query_row(
        "
        SELECT
          case_id, param_signature, feature_code, account_key, direction,
          txn_count, total_amount, txn_ids_json, amounts_json, detail_json,
          first_time, last_time, min_amount, min_count, min_total_amount
        FROM analysis_rule_pattern_idx
        ORDER BY feature_code
        ",
        [],
        |row| {
            Ok(FeatureRow {
                case_id: row.get(0)?,
                param_signature: row.get(1)?,
                feature_code: row.get(2)?,
                account_key: row.get(3)?,
                direction: row.get(4)?,
                txn_count: row.get(5)?,
                total_amount: row.get(6)?,
                txn_ids_json: row.get(7)?,
                amounts_json: row.get(8)?,
                detail_json: row.get(9)?,
                first_time: row.get(10)?,
                last_time: row.get(11)?,
                min_amount: row.get(12)?,
                min_count: row.get(13)?,
                min_total_amount: row.get(14)?,
            })
        },
    )
    .context("read materialized rule pattern feature")
}
