use anyhow::{bail, Result};
use base64::engine::general_purpose::STANDARD as BASE64_STANDARD;
use base64::Engine as _;
use duckdb::Connection;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::path::Path;

use crate::txn_daily_store::{MaterializeArgs, MaterializeResult};

pub const FUNDS_MATERIALIZE_TXN_DAILY_V1_COMMAND: &str = "funds.materialize_txn_daily_v1";

const MAX_SAFE_JSON_INTEGER: i64 = 9_007_199_254_740_991;
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_SOURCE_TIMESTAMP_BYTES: usize = 64;

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct FundsMaterializeTxnDailyV1Arguments {
    pub case_id: String,
    pub source_revision: i64,
    pub source_row_count: i64,
    pub source_max_txn_ts: String,
    pub source_max_id: i64,
    pub raw_artifact_manifest_sha256: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsMaterializeTxnDailyV1Result {
    pub case_id: String,
    pub row_count: i64,
    pub agg_name: String,
    pub agg_version: i64,
    pub materialization_identity: String,
    pub materialization_identity_schema_version: i64,
    pub producer_content_id: String,
    pub producer_content_manifest_base64: String,
    pub producer_content_manifest_sha256: String,
    pub producer_content_manifest_byte_length: usize,
    pub raw_artifact_manifest_sha256: String,
    pub raw_source_manifest_base64: String,
    pub raw_source_manifest_sha256: String,
    pub raw_source_manifest_byte_length: usize,
    pub duckdb_content_snapshot_digest: String,
    pub duckdb_snapshot_manifest_sha256: String,
    pub schema_digest: String,
}

pub fn validate_funds_materialize_txn_daily_v1_arguments(
    arguments: &FundsMaterializeTxnDailyV1Arguments,
    authoritative_case_id: &str,
) -> Result<()> {
    if authoritative_case_id.is_empty()
        || authoritative_case_id != authoritative_case_id.trim()
        || authoritative_case_id.len() > MAX_CASE_ID_BYTES
        || authoritative_case_id.chars().any(char::is_control)
        || arguments.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    if arguments.source_revision <= 0
        || arguments.source_revision > MAX_SAFE_JSON_INTEGER
        || arguments.source_row_count < 0
        || arguments.source_row_count > MAX_SAFE_JSON_INTEGER
        || arguments.source_max_id < 0
        || arguments.source_max_id > MAX_SAFE_JSON_INTEGER
        || !valid_source_timestamp(&arguments.source_max_txn_ts)
        || !crate::is_sha256_hex(&arguments.raw_artifact_manifest_sha256)
    {
        bail!("data_engine_request_contract_invalid");
    }
    Ok(())
}

pub fn run_funds_materialize_txn_daily_v1(
    arguments: FundsMaterializeTxnDailyV1Arguments,
    db_path: &Path,
) -> Result<FundsMaterializeTxnDailyV1Result> {
    validate_funds_materialize_txn_daily_v1_arguments(&arguments, &arguments.case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let args = MaterializeArgs {
        case_id: arguments.case_id.clone(),
        db_path: db_path.to_path_buf(),
        source_revision: arguments.source_revision,
        source_row_count: arguments.source_row_count,
        source_max_txn_ts: arguments.source_max_txn_ts.clone(),
        source_max_id: arguments.source_max_id,
    };
    let result = args
        .materialize_with_raw_artifact_manifest_sha256(&arguments.raw_artifact_manifest_sha256)?;
    build_funds_materialize_txn_daily_v1_result(args, arguments, result)
}

pub(crate) fn run_funds_materialize_txn_daily_v1_on_connection(
    arguments: FundsMaterializeTxnDailyV1Arguments,
    db_path: &Path,
    connection: &Connection,
) -> Result<FundsMaterializeTxnDailyV1Result> {
    validate_funds_materialize_txn_daily_v1_arguments(&arguments, &arguments.case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let args = MaterializeArgs {
        case_id: arguments.case_id.clone(),
        db_path: db_path.to_path_buf(),
        source_revision: arguments.source_revision,
        source_row_count: arguments.source_row_count,
        source_max_txn_ts: arguments.source_max_txn_ts.clone(),
        source_max_id: arguments.source_max_id,
    };
    let result = args.materialize_with_raw_artifact_manifest_sha256_on_connection(
        &arguments.raw_artifact_manifest_sha256,
        connection,
    )?;
    build_funds_materialize_txn_daily_v1_result(args, arguments, result)
}

fn build_funds_materialize_txn_daily_v1_result(
    args: MaterializeArgs,
    arguments: FundsMaterializeTxnDailyV1Arguments,
    result: MaterializeResult,
) -> Result<FundsMaterializeTxnDailyV1Result> {
    let producer_content_manifest_sha256 = sha256_hex(&result.producer_content_manifest_bytes);
    let raw_source_manifest_sha256 = sha256_hex(&result.raw_source_manifest_bytes);
    Ok(FundsMaterializeTxnDailyV1Result {
        case_id: args.case_id,
        row_count: result.row_count,
        agg_name: result.identity.value.clone(),
        agg_version: crate::AGG_VERSION,
        materialization_identity: result.identity.value,
        materialization_identity_schema_version: crate::MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
        producer_content_id: result.producer_content_id,
        producer_content_manifest_base64: BASE64_STANDARD
            .encode(&result.producer_content_manifest_bytes),
        producer_content_manifest_sha256,
        producer_content_manifest_byte_length: result.producer_content_manifest_bytes.len(),
        raw_artifact_manifest_sha256: arguments.raw_artifact_manifest_sha256,
        raw_source_manifest_base64: BASE64_STANDARD.encode(&result.raw_source_manifest_bytes),
        raw_source_manifest_sha256,
        raw_source_manifest_byte_length: result.raw_source_manifest_bytes.len(),
        duckdb_content_snapshot_digest: result.duckdb_content_snapshot_digest,
        duckdb_snapshot_manifest_sha256: result.duckdb_snapshot_manifest_sha256,
        schema_digest: result.schema_digest,
    })
}

fn sha256_hex(value: &[u8]) -> String {
    format!("{:x}", Sha256::digest(value))
}

fn valid_source_timestamp(value: &str) -> bool {
    if value.len() > MAX_SOURCE_TIMESTAMP_BYTES || value != value.trim() {
        return false;
    }
    value.bytes().all(|byte| {
        byte.is_ascii_digit() || matches!(byte, b'-' | b':' | b'.' | b' ' | b'+' | b'T' | b'Z')
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use duckdb::Connection;
    use std::fs;
    use std::path::PathBuf;
    use std::time::{SystemTime, UNIX_EPOCH};

    struct TempDb(PathBuf);

    impl TempDb {
        fn new() -> Self {
            let unique = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock after UNIX epoch")
                .as_nanos();
            Self(std::env::temp_dir().join(format!(
                "analytix-fpc1-command-{}-{unique}.duckdb",
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

    fn arguments() -> FundsMaterializeTxnDailyV1Arguments {
        FundsMaterializeTxnDailyV1Arguments {
            case_id: "case-a".to_string(),
            source_revision: 7,
            source_row_count: 2,
            source_max_txn_ts: "2026-01-02 01:02:03".to_string(),
            source_max_id: 2,
            raw_artifact_manifest_sha256: "1".repeat(64),
        }
    }

    #[test]
    fn typed_arguments_are_case_bound_and_json_safe() {
        validate_funds_materialize_txn_daily_v1_arguments(&arguments(), "case-a")
            .expect("exact typed arguments");

        let mut mismatched = arguments();
        mismatched.case_id = "case-b".to_string();
        assert!(format!(
            "{:#}",
            validate_funds_materialize_txn_daily_v1_arguments(&mismatched, "case-a")
                .expect_err("case mismatch")
        )
        .contains("data_engine_context_mismatch"));

        for invalid in [
            FundsMaterializeTxnDailyV1Arguments {
                source_revision: 0,
                ..arguments()
            },
            FundsMaterializeTxnDailyV1Arguments {
                source_row_count: -1,
                ..arguments()
            },
            FundsMaterializeTxnDailyV1Arguments {
                source_max_id: MAX_SAFE_JSON_INTEGER + 1,
                ..arguments()
            },
            FundsMaterializeTxnDailyV1Arguments {
                source_max_txn_ts: "2026-01-02'; DROP TABLE x; --".to_string(),
                ..arguments()
            },
            FundsMaterializeTxnDailyV1Arguments {
                raw_artifact_manifest_sha256: String::new(),
                ..arguments()
            },
            FundsMaterializeTxnDailyV1Arguments {
                raw_artifact_manifest_sha256: "A".repeat(64),
                ..arguments()
            },
        ] {
            assert!(format!(
                "{:#}",
                validate_funds_materialize_txn_daily_v1_arguments(&invalid, "case-a")
                    .expect_err("invalid typed arguments")
            )
            .contains("data_engine_request_contract_invalid"));
        }

        let missing_typed_manifest = serde_json::json!({
            "caseId": "case-a",
            "sourceRevision": 7,
            "sourceRowCount": 2,
            "sourceMaxTxnTs": "2026-01-02 01:02:03",
            "sourceMaxId": 2
        });
        assert!(
            serde_json::from_value::<FundsMaterializeTxnDailyV1Arguments>(missing_typed_manifest)
                .is_err()
        );
    }

    #[test]
    fn typed_result_separates_raw_inventory_from_typed_manifest_and_binds_snapshot() {
        let db = TempDb::new();
        let conn = Connection::open(&db.0).expect("open typed materialization fixture");
        conn.execute_batch(
            "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
             INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, row_no BIGINT, case_id TEXT, txn_ts TIMESTAMP, acct_no TEXT, \
               amount DOUBLE, clean_amount TEXT, dc_flag TEXT, summary TEXT, \
               remark TEXT, file_id TEXT, clean_invalid INTEGER, clean_failed INTEGER, \
               clean_reversal INTEGER \
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
        .expect("seed typed materialization fixture");
        drop(conn);

        let simplified_raw_source = serde_json::to_vec(&serde_json::json!([{
            "cleanedStatus": "done",
            "fileId": "file-1",
            "rowsImportedNorm": 2,
            "sha256": "a".repeat(64),
            "status": "已完成"
        }]))
        .expect("encode simplified raw-source inventory");
        let mut conflated = arguments();
        conflated.raw_artifact_manifest_sha256 = sha256_hex(&simplified_raw_source);
        let error = match run_funds_materialize_txn_daily_v1(conflated, &db.0) {
            Ok(_) => panic!("simplified inventory digest acquired typed manifest authority"),
            Err(error) => error,
        };
        assert!(error
            .to_string()
            .contains("cannot be a simplified raw-source inventory"));

        let result = run_funds_materialize_txn_daily_v1(arguments(), &db.0)
            .expect("run typed materialization");
        assert_eq!(result.raw_artifact_manifest_sha256, "1".repeat(64));
        assert_ne!(
            result.raw_source_manifest_sha256,
            result.raw_artifact_manifest_sha256
        );
        assert_eq!(
            result.schema_digest,
            "81a474553b16799c12d8d85c28d99f25ba6c9b40ae2073701443d2898f91e11e"
        );
        for digest in [
            result.producer_content_manifest_sha256.as_str(),
            result.raw_source_manifest_sha256.as_str(),
            result.duckdb_content_snapshot_digest.as_str(),
            result.duckdb_snapshot_manifest_sha256.as_str(),
        ] {
            assert!(crate::is_sha256_hex(digest));
        }
        let producer_bytes = BASE64_STANDARD
            .decode(&result.producer_content_manifest_base64)
            .expect("decode producer manifest");
        let producer: serde_json::Value =
            serde_json::from_slice(&producer_bytes).expect("parse producer manifest");
        assert_eq!(
            producer
                .get("rawManifestSha256")
                .and_then(serde_json::Value::as_str),
            Some(result.raw_artifact_manifest_sha256.as_str())
        );
        assert_eq!(
            serde_json::to_value(&result)
                .expect("serialize typed result")
                .as_object()
                .expect("typed result object")
                .len(),
            17
        );
    }
}
