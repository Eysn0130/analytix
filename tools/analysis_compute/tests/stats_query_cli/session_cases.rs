use anyhow::{Context, Result};
use duckdb::Connection;
use serde_json::{json, Value};
use std::io::{BufRead, BufReader, Write};
use std::process::{Command, Stdio};

use crate::command::run_analysis_compute_failure;
use crate::fixtures::{refresh_stats_query_materialization_meta, seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

fn date_range_args<'a>(db_path: &'a str, case_id: &'a str) -> [&'a str; 5] {
    [
        "query-stats-date-range",
        "--case-id",
        case_id,
        "--db-path",
        db_path,
    ]
}

#[test]
fn verified_stats_query_session_rejects_wrong_case_binding() -> Result<()> {
    let db = TempDb::new("stats-session-wrong-case")?;
    seed_stats_query_tables(db.path())?;
    let db_path = db.path().to_string_lossy();

    let failure = run_analysis_compute_failure(&date_range_args(&db_path, "case-other"))?;
    assert!(
        failure.stderr.contains("stats_query_case_binding_mismatch"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_stale_meta() -> Result<()> {
    let db = TempDb::new("stats-session-stale-meta")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_materialization_meta SET source_revision=source_revision+1",
        [],
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_source_identity_invalid")
            || failure
                .stderr
                .contains("materialization source revision changed"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_same_count_raw_source_drift() -> Result<()> {
    let db = TempDb::new("stats-session-same-count-drift")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE fc_transaction_norm SET txn_ts=TIMESTAMP '2026-04-01 09:00:01' WHERE id=1",
        [],
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_source_identity_invalid")
            || failure
                .stderr
                .contains("materialization source content changed")
            || failure
                .stderr
                .contains("stats_query_source_identity_mismatch"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_legacy_v9_v10_v11_meta() -> Result<()> {
    let db = TempDb::new("stats-session-legacy-meta")?;
    seed_stats_query_tables(db.path())?;
    let db_path = db.path().to_string_lossy().into_owned();

    for version in [9_i64, 10, 11] {
        let conn = Connection::open(db.path())?;
        conn.execute(
            "UPDATE analysis_materialization_meta SET agg_version=?",
            [version],
        )?;
        drop(conn);
        let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
        assert!(
            failure
                .stderr
                .contains("stats_query_materialization_version_unsupported"),
            "version={version} stderr={}",
            failure.stderr
        );
    }
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_corrupt_result_with_same_row_count() -> Result<()> {
    let db = TempDb::new("stats-session-corrupt-result")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_txn_detail_idx SET amount=amount+1 WHERE id=1",
        [],
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_result_signature_mismatch"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_cross_case_result_lineage() -> Result<()> {
    let db = TempDb::new("stats-session-cross-case-result")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO fc_transaction_norm( \
           id, row_no, case_id, txn_ts, clean_amount, file_id, \
           clean_invalid, clean_failed, clean_reversal \
         ) VALUES ( \
           99, 99, 'case-other', TIMESTAMP '2026-04-01 09:00:00', \
           '10000.00', 'file-1', 0, 0, 0 \
         ); \
         UPDATE analysis_txn_detail_idx SET id=99 WHERE id=1;",
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_case_isolation_invalid"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_result_row_count_drift() -> Result<()> {
    let db = TempDb::new("stats-session-row-count")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_materialization_meta SET row_count=row_count+1",
        [],
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_result_row_count_mismatch"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_fixed_table_schema_drift() -> Result<()> {
    let db = TempDb::new("stats-session-schema")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch("ALTER TABLE analysis_txn_detail_idx RENAME amount TO amount_legacy")?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_result_schema_mismatch:analysis_txn_detail_idx"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_multiple_txn_daily_meta_rows() -> Result<()> {
    let db = TempDb::new("stats-session-ambiguous-meta")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch(
        "INSERT INTO analysis_materialization_meta \
         SELECT * REPLACE ('txn_daily_shadow:v12' AS agg_name) \
           FROM analysis_materialization_meta;",
    )?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_materialization_meta_ambiguous:2:2"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_empty_verified_range() -> Result<()> {
    let db = TempDb::new("stats-session-empty-range")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute(
        "UPDATE analysis_txn_daily_agg SET first_ts=NULL, last_ts=NULL",
        [],
    )?;
    drop(conn);
    refresh_stats_query_materialization_meta(db.path())?;

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn verified_stats_query_session_rejects_missing_raw_source_even_with_valid_meta() -> Result<()> {
    let db = TempDb::new("stats-session-missing-raw")?;
    seed_stats_query_tables(db.path())?;
    let conn = Connection::open(db.path())?;
    conn.execute_batch("DROP TABLE fc_transaction_norm")?;
    drop(conn);

    let db_path = db.path().to_string_lossy();
    let failure = run_analysis_compute_failure(&date_range_args(&db_path, CASE_ID))?;
    assert!(
        failure
            .stderr
            .contains("stats_query_base_table_unavailable:fc_transaction_norm"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn stats_query_worker_revalidates_case_binding_for_every_request() -> Result<()> {
    let db = TempDb::new("stats-session-worker")?;
    seed_stats_query_tables(db.path())?;
    let binary = std::path::PathBuf::from(env!("CARGO_BIN_EXE_analytix-analysis-compute"));
    let mut child = Command::new(binary)
        .arg("stats-query-worker")
        .env("ANALYTIX_STATS_WORKER_CONN_IDLE_MS", "60000")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .spawn()
        .context("spawn stats query worker")?;
    let mut stdin = child.stdin.take().context("worker stdin unavailable")?;
    let stdout = child.stdout.take().context("worker stdout unavailable")?;
    let mut stdout = BufReader::new(stdout);

    let request = |id: i64, case_id: &str| {
        json!({
            "id": id,
            "command": "query-stats-date-range",
            "args": {"case_id": case_id, "db_path": db.path()},
        })
    };
    let mut line = String::new();
    writeln!(stdin, "{}", request(1, CASE_ID))?;
    stdin.flush()?;
    stdout.read_line(&mut line)?;
    let accepted: Value = serde_json::from_str(line.trim())?;
    assert_eq!(accepted["id"], 1);
    assert_eq!(accepted["ok"], true);

    line.clear();
    writeln!(stdin, "{}", request(2, "case-other"))?;
    stdin.flush()?;
    stdout.read_line(&mut line)?;
    let rejected: Value = serde_json::from_str(line.trim())?;
    assert_eq!(rejected["id"], 2);
    assert_eq!(rejected["ok"], false);
    assert!(rejected["error"]
        .as_str()
        .is_some_and(|value| value.contains("stats_query_case_binding_mismatch")));

    line.clear();
    let empty_page = json!({
        "id": 3,
        "command": "query-stats-rows",
        "args": {
            "case_id": CASE_ID,
            "db_path": db.path(),
            "mode": "inAccount",
            "selected_keys": ["CARD-001"],
            "date_start": "",
            "date_end": "",
            "search_text": "",
            "row_sort_col": "total_amount",
            "row_sort_dir": "desc",
            "row_offset": 99,
            "row_limit": 10,
            "row_format": "object",
            "fields": [],
            "output_json": null
        }
    });
    writeln!(stdin, "{empty_page}")?;
    stdin.flush()?;
    stdout.read_line(&mut line)?;
    let rejected_empty: Value = serde_json::from_str(line.trim())?;
    assert_eq!(rejected_empty["id"], 3);
    assert_eq!(rejected_empty["ok"], false);
    assert!(rejected_empty["error"]
        .as_str()
        .is_some_and(|value| value.contains("stats_query_scope_empty")));
    assert_eq!(
        rejected_empty.as_object().map(|object| object.len()),
        Some(3),
        "worker must discard partial JSON fields on gate failure: {rejected_empty}"
    );

    drop(stdin);
    assert!(child.wait()?.success());
    Ok(())
}
