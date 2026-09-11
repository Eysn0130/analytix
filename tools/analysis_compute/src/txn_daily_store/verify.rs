use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde::Serialize;
use std::path::PathBuf;

use super::args::MaterializeArgs;
use super::dataset_snapshot_manifest_v2::verify_dataset_snapshot_manifest_v2;
use super::funds_producer_content_manifest_v1::{
    funds_analytical_schema_digest_v1, require_base_table,
    verify_funds_producer_content_manifest_v1,
};
use super::identity::build_materialization_identity;
use super::meta_swap::materialization_result_signature;
use crate::{
    AGG_TABLE, AGG_VERSION, MATERIALIZATION_IDENTITY_PREFIX,
    MATERIALIZATION_IDENTITY_SCHEMA_VERSION, META_TABLE,
};

#[derive(Debug)]
pub(crate) struct VerifyArgs {
    case_id: String,
    db_path: PathBuf,
}

#[derive(Debug, Serialize)]
pub(crate) struct VerifyTxnDailyResult {
    pub(crate) case_id: String,
    pub(crate) source_revision: i64,
    pub(crate) source_row_count: i64,
    pub(crate) accepted_row_count: i64,
    pub(crate) rejected_row_count: i64,
    pub(crate) duplicate_row_count: i64,
    pub(crate) aggregate_row_count: i64,
    pub(crate) materialization_identity: String,
    pub(crate) result_signature: String,
    pub(crate) producer_content_contract: String,
    pub(crate) producer_content_id: String,
    pub(crate) producer_manifest_sha256: String,
    pub(crate) raw_artifact_manifest_sha256: String,
    pub(crate) duckdb_version: String,
    pub(crate) duckdb_content_snapshot_digest: String,
    pub(crate) duckdb_snapshot_manifest_sha256: String,
    pub(crate) schema_digest: String,
}

// VerifiedTxnDailySnapshotBinding is content verification output only. The
// DuckDB digest proves the exact admitted materialization bytes; it is not a
// Go DatasetSnapshotAuthorityV2 identity or currentness assertion.
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct VerifiedTxnDailySnapshotBinding {
    pub(crate) case_id: String,
    pub(crate) source_revision: i64,
    pub(crate) source_row_count: i64,
    pub(crate) accepted_row_count: i64,
    pub(crate) rejected_row_count: i64,
    pub(crate) duplicate_row_count: i64,
    pub(crate) aggregate_row_count: i64,
    pub(crate) materialization_identity: String,
    pub(crate) source_signature: String,
    pub(crate) result_signature: String,
    pub(crate) producer_content_contract: String,
    pub(crate) producer_content_id: String,
    pub(crate) producer_manifest_sha256: String,
    pub(crate) raw_artifact_manifest_sha256: String,
    pub(crate) duckdb_version: String,
    pub(crate) duckdb_content_snapshot_digest: String,
    pub(crate) duckdb_snapshot_manifest_sha256: String,
    pub(crate) schema_digest: String,
}

#[derive(Debug)]
struct PersistedMaterializationMeta {
    agg_name: String,
    agg_version: i64,
    case_id: String,
    identity_schema_version: i64,
    source_revision: i64,
    source_row_count: i64,
    source_max_txn_ts: String,
    source_max_id: i64,
    source_signature: String,
    result_signature: String,
    row_count: i64,
    built_at: String,
}

pub(crate) fn parse_verify_args(iter: impl Iterator<Item = String>) -> Result<VerifyArgs> {
    let mut case_id = None;
    let mut db_path = None;
    let mut args = iter.peekable();
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--case-id" => {
                if case_id.is_some() {
                    bail!("duplicate --case-id");
                }
                case_id = Some(crate::required_value(&mut args, "--case-id")?);
            }
            "--db-path" => {
                if db_path.is_some() {
                    bail!("duplicate --db-path");
                }
                db_path = Some(PathBuf::from(crate::required_value(
                    &mut args,
                    "--db-path",
                )?));
            }
            other => bail!("unknown argument: {other}"),
        }
    }
    let case_id = case_id
        .filter(|value| !value.is_empty())
        .context("missing --case-id")?;
    validate_case_id(&case_id)?;
    let db_path = db_path
        .filter(|value| !value.as_os_str().is_empty())
        .context("missing --db-path")?;
    Ok(VerifyArgs { case_id, db_path })
}

pub(crate) fn verify_txn_daily(args: &VerifyArgs) -> Result<VerifyTxnDailyResult> {
    validate_case_id(&args.case_id)?;
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {} read-only", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    conn.execute_batch("BEGIN TRANSACTION")?;
    let result = verify_txn_daily_snapshot(&conn, args);
    match result {
        Ok(result) => {
            conn.execute_batch("COMMIT")?;
            Ok(result)
        }
        Err(error) => {
            let _ = conn.execute_batch("ROLLBACK");
            Err(error)
        }
    }
}

fn verify_txn_daily_snapshot(conn: &Connection, args: &VerifyArgs) -> Result<VerifyTxnDailyResult> {
    let binding = verify_txn_daily_snapshot_binding(conn, &args.case_id)?;
    Ok(VerifyTxnDailyResult {
        case_id: binding.case_id,
        source_revision: binding.source_revision,
        source_row_count: binding.source_row_count,
        accepted_row_count: binding.accepted_row_count,
        rejected_row_count: binding.rejected_row_count,
        duplicate_row_count: binding.duplicate_row_count,
        aggregate_row_count: binding.aggregate_row_count,
        materialization_identity: binding.materialization_identity,
        result_signature: binding.result_signature,
        producer_content_contract: binding.producer_content_contract,
        producer_content_id: binding.producer_content_id,
        producer_manifest_sha256: binding.producer_manifest_sha256,
        raw_artifact_manifest_sha256: binding.raw_artifact_manifest_sha256,
        duckdb_version: binding.duckdb_version,
        duckdb_content_snapshot_digest: binding.duckdb_content_snapshot_digest,
        duckdb_snapshot_manifest_sha256: binding.duckdb_snapshot_manifest_sha256,
        schema_digest: binding.schema_digest,
    })
}

pub(crate) fn verify_txn_daily_snapshot_binding(
    conn: &Connection,
    case_id: &str,
) -> Result<VerifiedTxnDailySnapshotBinding> {
    validate_case_id(case_id)?;
    for table in [META_TABLE, "analysis_revision_state"] {
        require_base_table(conn, table)?;
    }
    let meta = load_materialization_meta(conn)?;
    validate_materialization_meta(&meta, case_id)?;
    let materialize_args = MaterializeArgs {
        case_id: meta.case_id.clone(),
        db_path: PathBuf::new(),
        source_revision: meta.source_revision,
        source_row_count: meta.source_row_count,
        source_max_txn_ts: meta.source_max_txn_ts.clone(),
        source_max_id: meta.source_max_id,
    };

    let identity = build_materialization_identity(conn, &materialize_args)?;
    if identity.value != meta.agg_name || identity.source_signature != meta.source_signature {
        bail!("transaction materialization source identity is stale");
    }

    let aggregate_row_count =
        crate::scalar_i64(conn, &format!("SELECT COUNT(*) FROM {AGG_TABLE}"))?;
    if aggregate_row_count != meta.row_count {
        bail!("transaction materialization metadata row count is stale");
    }
    let result_signature = materialization_result_signature(conn, case_id)?;
    if result_signature != meta.result_signature {
        bail!("transaction materialization result signature is stale");
    }

    let manifest = verify_funds_producer_content_manifest_v1(conn, &materialize_args)?;
    if manifest.normalized_row_count() != meta.source_row_count
        || manifest.aggregate_row_count() != meta.row_count
    {
        bail!("funds producer content manifest v1 metadata coverage is stale");
    }
    let snapshot =
        verify_dataset_snapshot_manifest_v2(conn, &materialize_args, &identity, &manifest)?;
    Ok(VerifiedTxnDailySnapshotBinding {
        case_id: case_id.to_string(),
        source_revision: meta.source_revision,
        source_row_count: meta.source_row_count,
        accepted_row_count: manifest.accepted_row_count(),
        rejected_row_count: manifest.rejected_row_count(),
        duplicate_row_count: manifest.duplicate_row_count(),
        aggregate_row_count: manifest.aggregate_row_count(),
        materialization_identity: identity.value,
        source_signature: identity.source_signature,
        result_signature,
        producer_content_contract: manifest.contract().to_string(),
        producer_content_id: manifest.producer_content_id.clone(),
        producer_manifest_sha256: manifest.manifest_sha256().to_string(),
        raw_artifact_manifest_sha256: manifest.raw_manifest_sha256().to_string(),
        duckdb_version: manifest.duckdb_version().to_string(),
        duckdb_content_snapshot_digest: snapshot.content_snapshot_digest().to_string(),
        duckdb_snapshot_manifest_sha256: snapshot.manifest_sha256().to_string(),
        schema_digest: funds_analytical_schema_digest_v1(),
    })
}

fn validate_case_id(case_id: &str) -> Result<()> {
    if case_id.is_empty()
        || case_id != case_id.trim()
        || case_id.len() > 512
        || case_id.chars().any(char::is_control)
    {
        bail!("transaction materialization verification case identity is invalid");
    }
    Ok(())
}

fn load_materialization_meta(conn: &Connection) -> Result<PersistedMaterializationMeta> {
    let mut statement = conn.prepare(&format!(
        "SELECT agg_name, agg_version, case_id, identity_schema_version, source_revision, \
                source_row_count, CAST(source_max_txn_ts AS VARCHAR), source_max_id, \
                source_signature, result_signature, row_count, CAST(built_at AS VARCHAR) \
           FROM {META_TABLE} WHERE starts_with(agg_name, 'txn_daily_') ORDER BY agg_name"
    ))?;
    let mut rows = statement.query([])?;
    let Some(row) = rows.next()? else {
        bail!("transaction materialization metadata is unavailable");
    };
    let meta = PersistedMaterializationMeta {
        agg_name: row.get(0)?,
        agg_version: row.get(1)?,
        case_id: row
            .get::<_, Option<String>>(2)?
            .context("transaction materialization case metadata is missing")?,
        identity_schema_version: row
            .get::<_, Option<i64>>(3)?
            .context("transaction materialization identity schema is missing")?,
        source_revision: row.get(4)?,
        source_row_count: row
            .get::<_, Option<i64>>(5)?
            .context("transaction materialization source row count is missing")?,
        source_max_txn_ts: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
        source_max_id: row
            .get::<_, Option<i64>>(7)?
            .context("transaction materialization source max id is missing")?,
        source_signature: row
            .get::<_, Option<String>>(8)?
            .context("transaction materialization source signature is missing")?,
        result_signature: row
            .get::<_, Option<String>>(9)?
            .context("transaction materialization result signature is missing")?,
        row_count: row
            .get::<_, Option<i64>>(10)?
            .context("transaction materialization result row count is missing")?,
        built_at: row
            .get::<_, Option<String>>(11)?
            .context("transaction materialization build timestamp is missing")?,
    };
    if rows.next()?.is_some() {
        bail!("transaction materialization metadata is ambiguous");
    }
    Ok(meta)
}

fn validate_materialization_meta(meta: &PersistedMaterializationMeta, case_id: &str) -> Result<()> {
    if meta.agg_version != AGG_VERSION
        || meta.case_id != case_id
        || meta.case_id != meta.case_id.trim()
        || meta.identity_schema_version != MATERIALIZATION_IDENTITY_SCHEMA_VERSION
        || meta.source_revision <= 0
        || meta.source_row_count < 0
        || meta.source_max_id < 0
        || meta.row_count < 0
        || meta.built_at.is_empty()
        || !crate::is_sha256_hex(&meta.source_signature)
        || !crate::is_sha256_hex(&meta.result_signature)
        || meta.agg_name != format!("{MATERIALIZATION_IDENTITY_PREFIX}{}", meta.source_signature)
    {
        bail!("transaction materialization metadata is invalid");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use duckdb::Connection;
    use sha2::{Digest, Sha256};
    use std::fs;
    use std::path::Path;
    use std::time::{SystemTime, UNIX_EPOCH};

    struct TempDb(PathBuf);

    impl TempDb {
        fn new(label: &str) -> Self {
            let unique = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock after UNIX epoch")
                .as_nanos();
            Self(std::env::temp_dir().join(format!(
                "analytix-verify-{label}-{}-{unique}.duckdb",
                std::process::id()
            )))
        }
    }

    impl Drop for TempDb {
        fn drop(&mut self) {
            let _ = fs::remove_file(&self.0);
            let mut wal = self.0.as_os_str().to_os_string();
            wal.push(".wal");
            let _ = fs::remove_file(PathBuf::from(wal));
        }
    }

    fn seed(path: &Path) -> MaterializeArgs {
        let conn = Connection::open(path).expect("open verifier fixture");
        conn.execute_batch(
            "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
             INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, row_no BIGINT, case_id TEXT, txn_ts TIMESTAMP, acct_no TEXT, amount DOUBLE, clean_amount TEXT, \
               dc_flag TEXT, summary TEXT, remark TEXT, file_id TEXT, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1, 1, 'case-a', TIMESTAMP '2026-01-01 01:02:03', 'A-1', 12.5, '12.50', \
                '进', '工资入账', '', 'file-1', 0, 0, 0), \
               (2, 2, 'case-a', TIMESTAMP '2026-01-02 01:02:03', 'A-1', 0.0, '0.00', \
                '出', '转出', 'POS', 'file-1', 0, 0, 0); \
             CREATE TABLE import_file_log( \
               file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, \
               rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT \
             ); \
             INSERT INTO import_file_log VALUES ( \
               'file-1', 'case-a', 'fc_transaction', \
               'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', \
               2, '已完成', 'done' \
             );",
        )
        .expect("seed verifier fixture");
        MaterializeArgs {
            case_id: "case-a".to_string(),
            db_path: path.to_path_buf(),
            source_revision: 7,
            source_row_count: 2,
            source_max_txn_ts: "2026-01-02 01:02:03".to_string(),
            source_max_id: 2,
        }
    }

    fn materialized(label: &str) -> TempDb {
        let db = TempDb::new(label);
        let args = seed(&db.0);
        super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
            &args,
            &"1".repeat(64),
        )
        .expect("materialize verifier fixture");
        db
    }

    fn verify_args(path: &Path) -> VerifyArgs {
        VerifyArgs {
            case_id: "case-a".to_string(),
            db_path: path.to_path_buf(),
        }
    }

    fn file_sha256(path: &Path) -> String {
        format!(
            "{:x}",
            Sha256::digest(fs::read(path).expect("read fixture DB"))
        )
    }

    #[test]
    fn verifier_uses_a_read_only_snapshot_and_returns_only_bounded_metadata() {
        let db = materialized("readonly");
        let before = file_sha256(&db.0);

        let result = verify_txn_daily(&verify_args(&db.0)).expect("verify materialization");

        assert_eq!(file_sha256(&db.0), before);
        assert_eq!(result.case_id, "case-a");
        assert_eq!(result.source_revision, 7);
        assert_eq!(result.source_row_count, 2);
        assert_eq!(result.accepted_row_count, 2);
        assert_eq!(result.rejected_row_count, 0);
        assert_eq!(result.duplicate_row_count, 0);
        assert_eq!(result.aggregate_row_count, 2);
        assert_eq!(
            result.producer_content_contract,
            "analytix.funds-producer-content-manifest/v1"
        );
        assert!(result.producer_content_id.starts_with("fpc1_"));
        assert!(!result.producer_content_id.starts_with("dsv2_"));
        assert!(crate::is_sha256_hex(&result.producer_manifest_sha256));
        assert_eq!(result.raw_artifact_manifest_sha256, "1".repeat(64));
        assert!(crate::is_sha256_hex(&result.result_signature));
        assert!(crate::is_sha256_hex(&result.duckdb_content_snapshot_digest));
        assert!(crate::is_sha256_hex(
            &result.duckdb_snapshot_manifest_sha256
        ));
        assert_eq!(result.schema_digest, funds_analytical_schema_digest_v1());
        assert_eq!(result.duckdb_version, "v1.5.4");
        assert_eq!(
            serde_json::to_value(&result)
                .expect("serialize result")
                .as_object()
                .expect("metadata object")
                .len(),
            17
        );

        crate::run_cli(
            [
                "verify-txn-daily",
                "--case-id",
                "case-a",
                "--db-path",
                db.0.to_str().expect("UTF-8 fixture path"),
            ]
            .into_iter()
            .map(str::to_string),
        )
        .expect("CLI verifier route");
        let data_engine = crate::run_data_engine_analysis_verify_command(
            "verify-txn-daily",
            serde_json::json!({
                "argv": [
                    "--case-id", "case-a",
                    "--db-path", db.0.to_str().expect("UTF-8 fixture path")
                ]
            }),
        )
        .expect("data-engine verifier route");
        assert_eq!(data_engine.get("ok"), Some(&serde_json::Value::Bool(true)));
        assert_eq!(
            data_engine
                .get("producer_content_id")
                .and_then(serde_json::Value::as_str),
            Some(result.producer_content_id.as_str())
        );
        assert_eq!(file_sha256(&db.0), before);
    }

    #[test]
    fn verifier_rejects_source_meta_result_and_manifest_tampering() {
        for (label, mutation, expected) in [
            (
                "source",
                "UPDATE fc_transaction_norm SET amount=13.5 WHERE id=1",
                "source identity",
            ),
            (
                "meta",
                "UPDATE analysis_materialization_meta SET source_row_count=3",
                "source content changed",
            ),
            (
                "result",
                "UPDATE analysis_txn_detail_idx SET amount=13.5 WHERE id=1",
                "result signature",
            ),
            (
                "manifest",
                "UPDATE analysis_funds_content_manifest_v1 SET producer_content_id='fpc1_invalid'",
                "manifest v1",
            ),
            (
                "dataset-manifest",
                "UPDATE analysis_dataset_snapshot_manifest_v2 SET manifest_sha256='invalid'",
                "manifest v2",
            ),
            (
                "acceptance-state",
                "UPDATE fc_transaction_norm SET clean_invalid=1 WHERE id=1",
                "source identity",
            ),
            (
                "index-version",
                "UPDATE analysis_materialization_meta SET agg_version=11",
                "metadata is invalid",
            ),
        ] {
            let db = materialized(label);
            Connection::open(&db.0)
                .expect("open tamper fixture")
                .execute_batch(mutation)
                .expect("tamper fixture");
            let error = verify_txn_daily(&verify_args(&db.0))
                .expect_err("tampered fixture must fail closed");
            assert!(
                format!("{error:#}").contains(expected),
                "label={label} error={error:#}"
            );
        }
    }

    #[test]
    fn production_manifest_explicitly_binds_accepted_and_rejected_normalized_rows() {
        let db = TempDb::new("accepted-rejected");
        let args = seed(&db.0);
        Connection::open(&db.0)
            .expect("open rejection fixture")
            .execute_batch("UPDATE fc_transaction_norm SET clean_invalid=1 WHERE id=2")
            .expect("mark one normalized row rejected");
        super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
            &args,
            &"1".repeat(64),
        )
        .expect("materialize accepted/rejected fixture");

        let result =
            verify_txn_daily(&verify_args(&db.0)).expect("verify accepted/rejected fixture");
        assert_eq!(result.source_row_count, 2);
        assert_eq!(result.accepted_row_count, 1);
        assert_eq!(result.rejected_row_count, 1);
        assert_eq!(result.duplicate_row_count, 0);

        let conn = Connection::open(&db.0).expect("open manifest fixture");
        let (count, body): (i64, Vec<u8>) = conn
            .query_row(
                "SELECT COUNT(*) OVER (), manifest_bytes \
                   FROM analysis_dataset_snapshot_manifest_v2",
                [],
                |row| Ok((row.get(0)?, row.get(1)?)),
            )
            .expect("read produced dataset manifest");
        assert_eq!(count, 1);
        let manifest: serde_json::Value =
            serde_json::from_slice(&body).expect("parse produced dataset manifest");
        assert_eq!(manifest["schemaVersion"], 2);
        assert_eq!(manifest["materializationVersion"], AGG_VERSION);
        assert_eq!(
            manifest["identitySchemaVersion"],
            MATERIALIZATION_IDENTITY_SCHEMA_VERSION
        );
        assert_eq!(manifest["acceptedRowCount"], 1);
        assert_eq!(manifest["rejectedRowCount"], 1);
        assert_eq!(manifest["duplicateRowCount"], 0);
        assert_ne!(
            manifest["acceptedContentSha256"],
            manifest["rejectedContentSha256"]
        );
    }

    #[test]
    fn verifier_argument_contract_rejects_duplicates_and_extra_fields() {
        assert!(parse_verify_args(
            ["--case-id", "case-a", "--db-path", "/tmp/a.duckdb"]
                .into_iter()
                .map(str::to_string)
        )
        .is_ok());
        for invalid in [
            vec!["--case-id", "case-a"],
            vec!["--db-path", "/tmp/a.duckdb"],
            vec![
                "--case-id",
                "case-a",
                "--case-id",
                "case-b",
                "--db-path",
                "/tmp/a.duckdb",
            ],
            vec![
                "--case-id",
                "case-a",
                "--db-path",
                "/tmp/a.duckdb",
                "--extra",
            ],
        ] {
            assert!(parse_verify_args(invalid.into_iter().map(str::to_string)).is_err());
        }
        for invalid_case in ["case\ncontrol".to_string(), "x".repeat(513)] {
            assert!(parse_verify_args(
                [
                    "--case-id".to_string(),
                    invalid_case,
                    "--db-path".to_string(),
                    "/tmp/a.duckdb".to_string(),
                ]
                .into_iter()
            )
            .is_err());
        }
    }
}
