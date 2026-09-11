use anyhow::{bail, Context, Result};
use duckdb::Connection;
use serde::Serialize;
use std::collections::BTreeMap;
use std::time::Instant;

use super::account_dim::create_account_dim;
use super::args::MaterializeArgs;
use super::dataset_snapshot_manifest_v2::replace_dataset_snapshot_manifest_v2;
use super::detail_tables::create_txn_daily_staging_tables;
use super::funds_producer_content_manifest_v1::{
    funds_analytical_schema_digest_v1, replace_funds_producer_content_manifest_v1,
};
use super::identity::{
    build_materialization_identity, ensure_identity_columns, MaterializationIdentity,
};
use super::keyword_index::create_keyword_index;
use super::meta_swap::{swap_tables_and_index, upsert_meta};
use super::profile::{round_seconds, PhaseProfile};

#[derive(Serialize)]
pub(crate) struct MaterializeResult {
    pub(crate) row_count: i64,
    pub(crate) identity: MaterializationIdentity,
    pub(crate) producer_content_id: String,
    #[serde(skip)]
    pub(crate) producer_content_manifest_bytes: Vec<u8>,
    #[serde(skip)]
    pub(crate) raw_source_manifest_bytes: Vec<u8>,
    pub(crate) duckdb_content_snapshot_digest: String,
    pub(crate) duckdb_snapshot_manifest_sha256: String,
    pub(crate) schema_digest: String,
    pub(crate) phases_s: BTreeMap<String, f64>,
    pub(crate) phase_groups_s: BTreeMap<String, f64>,
}

pub(crate) fn materialize_txn_daily(args: &MaterializeArgs) -> Result<MaterializeResult> {
    let _ = args;
    bail!("typed raw artifact manifest binding is required")
}

impl MaterializeArgs {
    pub(crate) fn materialize_with_raw_artifact_manifest_sha256(
        &self,
        raw_artifact_manifest_sha256: &str,
    ) -> Result<MaterializeResult> {
        materialize_txn_daily_with_raw_artifact_manifest_sha256(self, raw_artifact_manifest_sha256)
    }

    pub(crate) fn materialize_with_raw_artifact_manifest_sha256_on_connection(
        &self,
        raw_artifact_manifest_sha256: &str,
        conn: &Connection,
    ) -> Result<MaterializeResult> {
        materialize_txn_daily_with_raw_artifact_manifest_sha256_on_connection(
            self,
            raw_artifact_manifest_sha256,
            conn,
        )
    }
}

pub(crate) fn materialize_txn_daily_with_raw_artifact_manifest_sha256(
    args: &MaterializeArgs,
    raw_artifact_manifest_sha256: &str,
) -> Result<MaterializeResult> {
    if !crate::is_sha256_hex(raw_artifact_manifest_sha256) {
        bail!("typed raw artifact manifest binding is invalid");
    }
    let mut profile = PhaseProfile::default();
    let started = Instant::now();
    let conn = Connection::open(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    profile.record_since("open_db", started);
    materialize_txn_daily_on_connection(args, raw_artifact_manifest_sha256, &conn, profile)
}

// materialize_txn_daily_with_raw_artifact_manifest_sha256_on_connection keeps
// a host-pinned canonical build on the one DuckDB connection that created its
// private database. It deliberately does not reopen args.db_path.
pub(crate) fn materialize_txn_daily_with_raw_artifact_manifest_sha256_on_connection(
    args: &MaterializeArgs,
    raw_artifact_manifest_sha256: &str,
    conn: &Connection,
) -> Result<MaterializeResult> {
    if !crate::is_sha256_hex(raw_artifact_manifest_sha256) {
        bail!("typed raw artifact manifest binding is invalid");
    }
    materialize_txn_daily_on_connection(
        args,
        raw_artifact_manifest_sha256,
        conn,
        PhaseProfile::default(),
    )
}

fn materialize_txn_daily_on_connection(
    args: &MaterializeArgs,
    raw_artifact_manifest_sha256: &str,
    conn: &Connection,
    mut profile: PhaseProfile,
) -> Result<MaterializeResult> {
    let started = Instant::now();
    crate::configure_connection(conn)?;
    profile.record_since("configure_connection", started);
    let started = Instant::now();
    crate::ensure_required_table(conn, "fc_transaction_norm")?;
    profile.record_since("ensure_transaction_table", started);
    let identity = build_materialization_identity(conn, args)?;

    let started = Instant::now();
    let cols = crate::table_columns(conn, "fc_transaction_norm")?;
    profile.record_since("load_columns", started);
    conn.execute_batch("BEGIN TRANSACTION")?;
    let publish_result = (|| -> Result<(i64, String, Vec<u8>, Vec<u8>, String, String)> {
        let started = Instant::now();
        crate::ensure_meta_table(conn)?;
        ensure_identity_columns(conn)?;
        profile.record_since("ensure_meta_table", started);
        let current_identity = build_materialization_identity(conn, args)?;
        if current_identity != identity {
            anyhow::bail!("materialization source identity changed during rebuild");
        }
        profile.extend(create_txn_daily_staging_tables(conn, &args.case_id, &cols)?);
        let started = Instant::now();
        create_keyword_index(conn)?;
        profile.record_since("create_keyword_index", started);
        let started = Instant::now();
        create_account_dim(conn, &args.case_id)?;
        profile.record_since("create_account_dim", started);
        let started = Instant::now();
        swap_tables_and_index(conn)?;
        profile.record_since("swap_tables_and_indexes", started);
        let started = Instant::now();
        let row_count =
            crate::scalar_i64(conn, &format!("SELECT COUNT(1) FROM {}", crate::AGG_TABLE))?;
        profile.record_since("count_rows", started);
        let started = Instant::now();
        upsert_meta(conn, args, &identity, row_count)?;
        profile.record_since("upsert_meta", started);
        let started = Instant::now();
        let producer_manifest =
            replace_funds_producer_content_manifest_v1(conn, args, raw_artifact_manifest_sha256)?;
        profile.record_since("publish_funds_producer_content_manifest_v1", started);
        let started = Instant::now();
        let snapshot =
            replace_dataset_snapshot_manifest_v2(conn, args, &identity, &producer_manifest)?;
        profile.record_since("publish_dataset_snapshot_manifest_v2", started);
        Ok((
            row_count,
            producer_manifest.producer_content_id.clone(),
            producer_manifest.manifest_bytes().to_vec(),
            producer_manifest.raw_source_manifest_bytes().to_vec(),
            snapshot.content_snapshot_digest().to_string(),
            snapshot.manifest_sha256().to_string(),
        ))
    })();
    let (
        row_count,
        producer_content_id,
        producer_content_manifest_bytes,
        raw_source_manifest_bytes,
        duckdb_content_snapshot_digest,
        duckdb_snapshot_manifest_sha256,
    ) = match publish_result {
        Ok(result) => {
            if let Err(error) = conn.execute_batch("COMMIT") {
                return Err(rollback_materialization(conn, error.into()));
            }
            result
        }
        Err(error) => return Err(rollback_materialization(conn, error)),
    };
    let phase_groups_s = materialization_groups(&profile);
    Ok(MaterializeResult {
        row_count,
        identity,
        producer_content_id,
        producer_content_manifest_bytes,
        raw_source_manifest_bytes,
        duckdb_content_snapshot_digest,
        duckdb_snapshot_manifest_sha256,
        schema_digest: funds_analytical_schema_digest_v1(),
        phase_groups_s,
        phases_s: profile.into_phases(),
    })
}

fn rollback_materialization(conn: &Connection, error: anyhow::Error) -> anyhow::Error {
    match conn.execute_batch("ROLLBACK") {
        Ok(()) => error,
        Err(rollback_error) => {
            anyhow::anyhow!("{error:#}; materialization rollback failed: {rollback_error}")
        }
    }
}

fn materialization_groups(profile: &PhaseProfile) -> BTreeMap<String, f64> {
    let setup = profile.sum(&[
        "open_db",
        "configure_connection",
        "ensure_transaction_table",
        "ensure_meta_table",
        "load_columns",
    ]);
    let materialization_sql = profile.sum(&[
        "prepare_staging_expressions",
        "drop_staging_tables",
        "build_daily_agg_staging",
        "build_detail_idx_staging",
        "create_keyword_index",
        "create_account_dim",
    ]);
    let index = profile.sum(&["swap_tables_and_indexes"]);
    let metadata = profile.sum(&[
        "count_rows",
        "upsert_meta",
        "publish_funds_producer_content_manifest_v1",
        "publish_dataset_snapshot_manifest_v2",
    ]);
    let known = setup + materialization_sql + index + metadata;
    let other = (profile.total() - known).max(0.0);
    let mut groups = BTreeMap::new();
    for (key, value) in [
        ("setup", setup),
        ("materialization_sql", materialization_sql),
        ("index", index),
        ("metadata", metadata),
        ("other", other),
    ] {
        let rounded = round_seconds(value);
        if rounded > 0.0 {
            groups.insert(key.to_string(), rounded);
        }
    }
    groups
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    use std::time::{SystemTime, UNIX_EPOCH};

    #[test]
    fn legacy_materializer_without_typed_raw_manifest_fails_before_io() {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock after UNIX epoch")
            .as_nanos();
        let path = PathBuf::from(std::env::temp_dir()).join(format!(
            "analytix-legacy-materializer-must-not-create-{}-{unique}.duckdb",
            std::process::id()
        ));
        let args = MaterializeArgs {
            case_id: "case-a".to_string(),
            db_path: path.clone(),
            source_revision: 1,
            source_row_count: 0,
            source_max_txn_ts: String::new(),
            source_max_id: 0,
        };
        let error = match materialize_txn_daily(&args) {
            Ok(_) => panic!("legacy path must fail closed"),
            Err(error) => error,
        };
        assert!(error
            .to_string()
            .contains("typed raw artifact manifest binding is required"));
        assert!(!path.exists(), "legacy failure created a DuckDB file");
    }
}
