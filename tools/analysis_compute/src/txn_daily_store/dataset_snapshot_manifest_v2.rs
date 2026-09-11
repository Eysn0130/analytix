use anyhow::{bail, Context, Result};
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

use super::args::MaterializeArgs;
use super::funds_producer_content_manifest_v1::{
    require_base_table, table_content_manifest, FundsProducerContentManifestV1Record,
};
use super::identity::MaterializationIdentity;
use crate::{sql_literal, AGG_VERSION, MATERIALIZATION_IDENTITY_SCHEMA_VERSION, META_TABLE};

pub(super) const DATASET_SNAPSHOT_MANIFEST_V2_TABLE: &str = "analysis_dataset_snapshot_manifest_v2";
const DATASET_SNAPSHOT_MANIFEST_V2_STAGING_TABLE: &str =
    "analysis_dataset_snapshot_manifest_v2__staging";
const DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION: i64 = 2;
const DATASET_SNAPSHOT_MANIFEST_V2_CONTRACT: &str = "analytix.duckdb-dataset-snapshot-manifest/v2";
const DATASET_SNAPSHOT_MANIFEST_V2_CLASSIFICATION_POLICY: &str =
    "clean-flags-v1-source-id-unique-no-dedup";
const DATASET_SNAPSHOT_MANIFEST_V2_DIGEST_DOMAIN: &[u8] =
    b"AnalytixDuckDBDatasetSnapshotManifestV2\0";
const DATASET_SNAPSHOT_MANIFEST_V2_DUPLICATE_DOMAIN: &[u8] =
    b"AnalytixDuckDBDatasetSnapshotNoDuplicateRowsV2\0";

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct DatasetSnapshotManifestV2Payload {
    accepted_content_sha256: String,
    accepted_row_count: i64,
    aggregate_content_sha256: String,
    aggregate_row_count: i64,
    case_id: String,
    classification_policy: String,
    contract: String,
    duplicate_content_sha256: String,
    duplicate_row_count: i64,
    identity_schema_version: i64,
    materialization_name: String,
    materialization_version: i64,
    normalized_content_sha256: String,
    normalized_row_count: i64,
    producer_content_id: String,
    producer_manifest_sha256: String,
    raw_manifest_sha256: String,
    rejected_content_sha256: String,
    rejected_row_count: i64,
    result_signature: String,
    schema_version: i64,
    source_revision: i64,
    source_signature: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) struct DatasetSnapshotManifestV2Record {
    payload: DatasetSnapshotManifestV2Payload,
    manifest_sha256: String,
    snapshot_digest: String,
    manifest_bytes: Vec<u8>,
}

impl DatasetSnapshotManifestV2Record {
    pub(super) fn manifest_sha256(&self) -> &str {
        &self.manifest_sha256
    }

    // This is the analysis-compute content snapshot digest. It is not the Go
    // DatasetSnapshotAuthorityV2 identity and must never be substituted for a
    // dsv2_ id or current-authority witness.
    pub(super) fn content_snapshot_digest(&self) -> &str {
        &self.snapshot_digest
    }
}

pub(super) fn replace_dataset_snapshot_manifest_v2(
    conn: &Connection,
    args: &MaterializeArgs,
    identity: &MaterializationIdentity,
    producer: &FundsProducerContentManifestV1Record,
) -> Result<DatasetSnapshotManifestV2Record> {
    let expected = build_dataset_snapshot_manifest_v2(conn, args, identity, producer)?;
    conn.execute_batch(&format!(
        "DROP TABLE IF EXISTS {DATASET_SNAPSHOT_MANIFEST_V2_STAGING_TABLE}; \
         CREATE TABLE {DATASET_SNAPSHOT_MANIFEST_V2_STAGING_TABLE}( \
           manifest_schema_version INTEGER NOT NULL, \
           contract TEXT NOT NULL, \
           case_id TEXT NOT NULL, \
           source_revision BIGINT NOT NULL, \
           manifest_bytes BLOB NOT NULL, \
           manifest_sha256 TEXT NOT NULL, \
           snapshot_digest TEXT NOT NULL \
         );"
    ))?;
    conn.execute(
        &format!("INSERT INTO {DATASET_SNAPSHOT_MANIFEST_V2_STAGING_TABLE} VALUES (?,?,?,?,?,?,?)"),
        params![
            expected.payload.schema_version,
            expected.payload.contract,
            expected.payload.case_id,
            expected.payload.source_revision,
            expected.manifest_bytes,
            expected.manifest_sha256,
            expected.snapshot_digest,
        ],
    )?;
    conn.execute_batch(&format!(
        "DROP TABLE IF EXISTS {DATASET_SNAPSHOT_MANIFEST_V2_TABLE}; \
         ALTER TABLE {DATASET_SNAPSHOT_MANIFEST_V2_STAGING_TABLE} \
         RENAME TO {DATASET_SNAPSHOT_MANIFEST_V2_TABLE};"
    ))?;
    let observed = read_dataset_snapshot_manifest_v2(conn)?;
    if observed != expected {
        bail!("dataset snapshot manifest v2 persistence verification failed");
    }
    Ok(expected)
}

pub(super) fn verify_dataset_snapshot_manifest_v2(
    conn: &Connection,
    args: &MaterializeArgs,
    identity: &MaterializationIdentity,
    producer: &FundsProducerContentManifestV1Record,
) -> Result<DatasetSnapshotManifestV2Record> {
    let expected = build_dataset_snapshot_manifest_v2(conn, args, identity, producer)?;
    let observed = read_dataset_snapshot_manifest_v2(conn)?;
    if observed != expected {
        bail!("dataset snapshot manifest v2 is stale");
    }
    Ok(observed)
}

fn build_dataset_snapshot_manifest_v2(
    conn: &Connection,
    args: &MaterializeArgs,
    identity: &MaterializationIdentity,
    producer: &FundsProducerContentManifestV1Record,
) -> Result<DatasetSnapshotManifestV2Record> {
    if args.case_id.is_empty()
        || args.case_id != args.case_id.trim()
        || producer.source_revision() != args.source_revision
        || producer.normalized_row_count() != args.source_row_count
        || producer.accepted_row_count()
            + producer.rejected_row_count()
            + producer.duplicate_row_count()
            != producer.normalized_row_count()
        || producer.duplicate_row_count() != 0
    {
        bail!("dataset snapshot manifest v2 producer binding is invalid");
    }
    let (
        materialization_name,
        materialization_version,
        identity_schema_version,
        source_signature,
        result_signature,
        aggregate_row_count,
    ) = read_exact_materialization(conn)?;
    if materialization_name != identity.value
        || source_signature != identity.source_signature
        || materialization_version != AGG_VERSION
        || identity_schema_version != MATERIALIZATION_IDENTITY_SCHEMA_VERSION
        || aggregate_row_count != producer.aggregate_row_count()
        || !crate::is_sha256_hex(&result_signature)
    {
        bail!("dataset snapshot manifest v2 materialization binding is stale");
    }
    let case_literal = sql_literal(&args.case_id);
    let accepted = table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!(
            "\"case_id\"={case_literal} AND clean_invalid=0 AND clean_failed=0 AND clean_reversal=0"
        ),
    )?;
    let rejected = table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!(
            "\"case_id\"={case_literal} AND (clean_invalid=1 OR clean_failed=1 OR clean_reversal=1)"
        ),
    )?;
    if accepted.row_count != producer.accepted_row_count()
        || rejected.row_count != producer.rejected_row_count()
        || accepted.row_count + rejected.row_count != producer.normalized_row_count()
    {
        bail!("dataset snapshot manifest v2 classification coverage is stale");
    }
    let duplicate_content_sha256 = sha256_domain(
        DATASET_SNAPSHOT_MANIFEST_V2_DUPLICATE_DOMAIN,
        format!(
            "{}\0{}\0{}",
            identity.source_signature,
            producer.normalized_content_sha256(),
            DATASET_SNAPSHOT_MANIFEST_V2_CLASSIFICATION_POLICY
        )
        .as_bytes(),
    );
    let payload = DatasetSnapshotManifestV2Payload {
        accepted_content_sha256: accepted.content_sha256,
        accepted_row_count: accepted.row_count,
        aggregate_content_sha256: producer.aggregate_content_sha256().to_string(),
        aggregate_row_count: producer.aggregate_row_count(),
        case_id: args.case_id.clone(),
        classification_policy: DATASET_SNAPSHOT_MANIFEST_V2_CLASSIFICATION_POLICY.to_string(),
        contract: DATASET_SNAPSHOT_MANIFEST_V2_CONTRACT.to_string(),
        duplicate_content_sha256,
        duplicate_row_count: producer.duplicate_row_count(),
        identity_schema_version,
        materialization_name,
        materialization_version,
        normalized_content_sha256: producer.normalized_content_sha256().to_string(),
        normalized_row_count: producer.normalized_row_count(),
        producer_content_id: producer.producer_content_id.clone(),
        producer_manifest_sha256: producer.manifest_sha256().to_string(),
        raw_manifest_sha256: producer.raw_manifest_sha256().to_string(),
        rejected_content_sha256: rejected.content_sha256,
        rejected_row_count: rejected.row_count,
        result_signature,
        schema_version: DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION,
        source_revision: args.source_revision,
        source_signature,
    };
    let manifest_bytes = serde_json::to_vec(&payload)?;
    let manifest_sha256 = sha256_bytes(&manifest_bytes);
    let snapshot_digest =
        sha256_domain(DATASET_SNAPSHOT_MANIFEST_V2_DIGEST_DOMAIN, &manifest_bytes);
    Ok(DatasetSnapshotManifestV2Record {
        payload,
        manifest_sha256,
        snapshot_digest,
        manifest_bytes,
    })
}

fn read_exact_materialization(
    conn: &Connection,
) -> Result<(String, i64, i64, String, String, i64)> {
    require_base_table(conn, META_TABLE)?;
    let mut statement = conn.prepare(&format!(
        "SELECT agg_name, agg_version, identity_schema_version, source_signature, \
                result_signature, row_count \
           FROM {META_TABLE} WHERE starts_with(agg_name, 'txn_daily_') ORDER BY agg_name"
    ))?;
    let mut rows = statement.query([])?;
    let Some(row) = rows.next()? else {
        bail!("dataset snapshot manifest v2 materialization is unavailable");
    };
    let result = (
        row.get(0)?,
        row.get(1)?,
        row.get::<_, Option<i64>>(2)?
            .context("dataset snapshot manifest v2 identity schema is missing")?,
        row.get::<_, Option<String>>(3)?
            .context("dataset snapshot manifest v2 source signature is missing")?,
        row.get::<_, Option<String>>(4)?
            .context("dataset snapshot manifest v2 result signature is missing")?,
        row.get::<_, Option<i64>>(5)?
            .context("dataset snapshot manifest v2 aggregate row count is missing")?,
    );
    if rows.next()?.is_some() {
        bail!("dataset snapshot manifest v2 materialization is ambiguous");
    }
    Ok(result)
}

fn read_dataset_snapshot_manifest_v2(conn: &Connection) -> Result<DatasetSnapshotManifestV2Record> {
    require_base_table(conn, DATASET_SNAPSHOT_MANIFEST_V2_TABLE)?;
    let mut statement = conn.prepare(&format!(
        "SELECT manifest_schema_version, contract, case_id, source_revision, manifest_bytes, \
                manifest_sha256, snapshot_digest \
           FROM {DATASET_SNAPSHOT_MANIFEST_V2_TABLE} ORDER BY case_id"
    ))?;
    let mut rows = statement.query([])?;
    let Some(row) = rows.next()? else {
        bail!("dataset snapshot manifest v2 is unavailable");
    };
    let schema_version: i64 = row.get(0)?;
    let contract: String = row.get(1)?;
    let case_id: String = row.get(2)?;
    let source_revision: i64 = row.get(3)?;
    let manifest_bytes: Vec<u8> = row.get(4)?;
    let manifest_sha256: String = row.get(5)?;
    let snapshot_digest: String = row.get(6)?;
    if rows.next()?.is_some() {
        bail!("dataset snapshot manifest v2 is ambiguous");
    }
    let payload: DatasetSnapshotManifestV2Payload =
        serde_json::from_slice(&manifest_bytes).context("parse dataset snapshot manifest v2")?;
    let canonical = serde_json::to_vec(&payload)?;
    if canonical != manifest_bytes
        || schema_version != DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION
        || contract != DATASET_SNAPSHOT_MANIFEST_V2_CONTRACT
        || payload.schema_version != schema_version
        || payload.contract != contract
        || payload.case_id != case_id
        || payload.source_revision != source_revision
        || manifest_sha256 != sha256_bytes(&manifest_bytes)
        || snapshot_digest
            != sha256_domain(DATASET_SNAPSHOT_MANIFEST_V2_DIGEST_DOMAIN, &manifest_bytes)
        || !manifest_hashes_are_canonical(&payload)
    {
        bail!("dataset snapshot manifest v2 is invalid");
    }
    Ok(DatasetSnapshotManifestV2Record {
        payload,
        manifest_sha256,
        snapshot_digest,
        manifest_bytes,
    })
}

fn manifest_hashes_are_canonical(payload: &DatasetSnapshotManifestV2Payload) -> bool {
    [
        payload.accepted_content_sha256.as_str(),
        payload.aggregate_content_sha256.as_str(),
        payload.duplicate_content_sha256.as_str(),
        payload.normalized_content_sha256.as_str(),
        payload.producer_manifest_sha256.as_str(),
        payload.raw_manifest_sha256.as_str(),
        payload.rejected_content_sha256.as_str(),
        payload.result_signature.as_str(),
        payload.source_signature.as_str(),
    ]
    .into_iter()
    .all(crate::is_sha256_hex)
        && payload.schema_version == DATASET_SNAPSHOT_MANIFEST_V2_SCHEMA_VERSION
        && payload.contract == DATASET_SNAPSHOT_MANIFEST_V2_CONTRACT
        && payload.classification_policy == DATASET_SNAPSHOT_MANIFEST_V2_CLASSIFICATION_POLICY
        && payload.materialization_version == AGG_VERSION
        && payload.identity_schema_version == MATERIALIZATION_IDENTITY_SCHEMA_VERSION
        && payload.materialization_name
            == format!(
                "{}{}",
                crate::MATERIALIZATION_IDENTITY_PREFIX,
                payload.source_signature
            )
        && payload.normalized_row_count >= 0
        && payload.accepted_row_count >= 0
        && payload.rejected_row_count >= 0
        && payload.duplicate_row_count == 0
        && payload.accepted_row_count + payload.rejected_row_count == payload.normalized_row_count
}

fn sha256_bytes(body: &[u8]) -> String {
    format!("{:x}", Sha256::digest(body))
}

fn sha256_domain(domain: &[u8], body: &[u8]) -> String {
    let mut digest = Sha256::new();
    digest.update(domain);
    digest.update(body);
    format!("{:x}", digest.finalize())
}
