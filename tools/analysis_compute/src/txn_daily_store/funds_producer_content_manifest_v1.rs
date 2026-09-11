use anyhow::{bail, Context, Result};
use chrono::{DateTime, NaiveDate, NaiveTime, Timelike, Utc};
use duckdb::types::{TimeUnit, ValueRef};
use duckdb::{params, Connection};
use serde::Serialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::collections::BTreeSet;

use super::args::MaterializeArgs;
use crate::{sql_literal, ACCOUNT_DIM_TABLE, AGG_TABLE, DETAIL_TABLE, KEYWORD_TABLE};

pub(super) const FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE: &str =
    "analysis_funds_content_manifest_v1";
const FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE: &str =
    "analysis_funds_content_manifest_v1__staging";
const FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION: i64 = 1;
const FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT: &str =
    "analytix.funds-producer-content-manifest/v1";
const FUNDS_PRODUCER_CONTENT_ID_PREFIX: &str = "fpc1_";
const FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN: &[u8] = b"AnalytixFundsProducerContentManifestV1\0";
const DUCKDB_CONTENT_MANIFEST_ALGORITHM: &str = "analytix.duckdb-content-manifest/v1";
const REQUIRED_DUCKDB_VERSION: &str = "v1.5.4";
const PRODUCER_COMPONENT_ID: &str = "analysis-compute";
const PRODUCER_COMPONENT_VERSION: &str = env!("CARGO_PKG_VERSION");
const PRODUCER_OPERATION: &str = "materialize-txn-daily";
const PRODUCER_OPERATION_SCHEMA: &str = concat!(
    "{\"additionalProperties\":false,\"properties\":{",
    "\"caseId\":{\"type\":\"string\"},",
    "\"sourceMaxId\":{\"minimum\":0,\"type\":\"integer\"},",
    "\"sourceMaxTxnTs\":{\"type\":\"string\"},",
    "\"sourceRevision\":{\"minimum\":1,\"type\":\"integer\"},",
    "\"sourceRowCount\":{\"minimum\":0,\"type\":\"integer\"}},",
    "\"required\":[\"caseId\",\"sourceMaxId\",\"sourceMaxTxnTs\",",
    "\"sourceRevision\",\"sourceRowCount\"],\"type\":\"object\"}"
);
pub(crate) const FUNDS_ANALYTICAL_SCHEMA_CONTRACT_V1: &str = "analytix.funds-analytical-schema/v1";
pub(crate) const FUNDS_ANALYTICAL_SCHEMA_CANONICAL_V1: &str =
    "{\"contract\":\"analytix.funds-analytical-schema/v1\",\"exactTypes\":{\"fc_transaction_norm.clean_amount\":\"VARCHAR\",\"fc_transaction_norm.file_id\":\"VARCHAR\",\"fc_transaction_norm.row_no\":\"BIGINT\"},\"operations\":[{\"name\":\"analyze_account_flows\",\"relations\":[{\"columns\":[\"case_id\",\"clean_amount\",\"file_id\",\"id\",\"row_no\"],\"name\":\"fc_transaction_norm\"},{\"columns\":[\"acct_key\",\"amount_parse_failed\",\"amount_source_present\",\"counterparty_bank\",\"counterparty_name\",\"cp_key\",\"currency\",\"dc_val\",\"file_id\",\"id\",\"txn_ts\"],\"name\":\"analysis_txn_detail_idx\"}]},{\"name\":\"resolve_account_ingress\",\"relations\":[{\"columns\":[\"account_key\",\"acct_type\",\"bank_name\"],\"name\":\"analysis_account_dim\"},{\"columns\":[\"acct_key\",\"acct_no\",\"card_no\"],\"name\":\"analysis_txn_detail_idx\"}]}],\"schemaVersion\":1}";

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct RawSourceIdentity {
    cleaned_status: String,
    file_id: String,
    rows_imported_norm: i64,
    sha256: String,
    status: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
struct ColumnManifest {
    name: String,
    #[serde(rename = "type")]
    data_type: String,
}

#[derive(Serialize)]
struct TableManifestHeader<'a> {
    columns: &'a [ColumnManifest],
    #[serde(rename = "schemaVersion")]
    schema_version: i64,
    table: &'a str,
}

#[derive(Serialize)]
struct TableManifestFooter {
    #[serde(rename = "rowCount")]
    row_count: i64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) struct TableContentManifest {
    pub(super) content_sha256: String,
    pub(super) row_count: i64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
struct FundsProducerContentManifestV1Payload {
    accepted_row_count: i64,
    account_content_sha256: String,
    account_row_count: i64,
    aggregate_content_sha256: String,
    aggregate_row_count: i64,
    canonical_encoder: String,
    case_id: String,
    contract: String,
    detail_content_sha256: String,
    detail_row_count: i64,
    duckdb_version: String,
    duplicate_row_count: i64,
    keyword_content_sha256: String,
    keyword_row_count: i64,
    normalized_content_sha256: String,
    normalized_row_count: i64,
    producer_component_id: String,
    producer_component_version: String,
    producer_operation: String,
    producer_operation_schema_hash: String,
    raw_manifest_sha256: String,
    rejected_row_count: i64,
    schema_version: i64,
    source_revision: i64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FundsProducerContentManifestV1Record {
    payload: FundsProducerContentManifestV1Payload,
    manifest_sha256: String,
    producer_content_digest: String,
    pub(crate) producer_content_id: String,
    manifest_bytes: Vec<u8>,
    raw_source_manifest_bytes: Vec<u8>,
}

impl FundsProducerContentManifestV1Record {
    pub(super) fn contract(&self) -> &str {
        &self.payload.contract
    }

    pub(super) fn duckdb_version(&self) -> &str {
        &self.payload.duckdb_version
    }

    pub(super) fn manifest_sha256(&self) -> &str {
        &self.manifest_sha256
    }

    pub(super) fn manifest_bytes(&self) -> &[u8] {
        &self.manifest_bytes
    }

    pub(super) fn raw_source_manifest_bytes(&self) -> &[u8] {
        &self.raw_source_manifest_bytes
    }

    pub(super) fn normalized_row_count(&self) -> i64 {
        self.payload.normalized_row_count
    }

    pub(super) fn accepted_row_count(&self) -> i64 {
        self.payload.accepted_row_count
    }

    pub(super) fn rejected_row_count(&self) -> i64 {
        self.payload.rejected_row_count
    }

    pub(super) fn duplicate_row_count(&self) -> i64 {
        self.payload.duplicate_row_count
    }

    pub(super) fn aggregate_row_count(&self) -> i64 {
        self.payload.aggregate_row_count
    }

    pub(super) fn aggregate_content_sha256(&self) -> &str {
        &self.payload.aggregate_content_sha256
    }

    pub(super) fn normalized_content_sha256(&self) -> &str {
        &self.payload.normalized_content_sha256
    }

    pub(super) fn raw_manifest_sha256(&self) -> &str {
        &self.payload.raw_manifest_sha256
    }

    pub(super) fn source_revision(&self) -> i64 {
        self.payload.source_revision
    }
}

pub(super) fn replace_funds_producer_content_manifest_v1(
    conn: &Connection,
    args: &MaterializeArgs,
    raw_artifact_manifest_sha256: &str,
) -> Result<FundsProducerContentManifestV1Record> {
    let expected =
        build_funds_producer_content_manifest_v1(conn, args, raw_artifact_manifest_sha256)?;
    conn.execute_batch(&format!(
        "DROP TABLE IF EXISTS {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE}; \
         CREATE TABLE {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE}( \
           schema_version INTEGER NOT NULL, \
           contract TEXT NOT NULL, \
           case_id TEXT NOT NULL, \
           source_revision BIGINT NOT NULL, \
           producer_component_id TEXT NOT NULL, \
           producer_component_version TEXT NOT NULL, \
           producer_operation TEXT NOT NULL, \
           producer_operation_schema_hash TEXT NOT NULL, \
           duckdb_version TEXT NOT NULL, \
           canonical_encoder TEXT NOT NULL, \
           raw_manifest_sha256 TEXT NOT NULL, \
           normalized_content_sha256 TEXT NOT NULL, \
           detail_content_sha256 TEXT NOT NULL, \
           aggregate_content_sha256 TEXT NOT NULL, \
           keyword_content_sha256 TEXT NOT NULL, \
           account_content_sha256 TEXT NOT NULL, \
           normalized_row_count BIGINT NOT NULL, \
           accepted_row_count BIGINT NOT NULL, \
           rejected_row_count BIGINT NOT NULL, \
           duplicate_row_count BIGINT NOT NULL, \
           detail_row_count BIGINT NOT NULL, \
           aggregate_row_count BIGINT NOT NULL, \
           keyword_row_count BIGINT NOT NULL, \
           account_row_count BIGINT NOT NULL, \
           manifest_sha256 TEXT NOT NULL, \
           producer_content_digest TEXT NOT NULL, \
           producer_content_id TEXT NOT NULL \
         );"
    ))?;
    let payload = &expected.payload;
    conn.execute(
        &format!(
            "INSERT INTO {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE} \
             VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
        ),
        params![
            payload.schema_version,
            payload.contract,
            payload.case_id,
            payload.source_revision,
            payload.producer_component_id,
            payload.producer_component_version,
            payload.producer_operation,
            payload.producer_operation_schema_hash,
            payload.duckdb_version,
            payload.canonical_encoder,
            payload.raw_manifest_sha256,
            payload.normalized_content_sha256,
            payload.detail_content_sha256,
            payload.aggregate_content_sha256,
            payload.keyword_content_sha256,
            payload.account_content_sha256,
            payload.normalized_row_count,
            payload.accepted_row_count,
            payload.rejected_row_count,
            payload.duplicate_row_count,
            payload.detail_row_count,
            payload.aggregate_row_count,
            payload.keyword_row_count,
            payload.account_row_count,
            expected.manifest_sha256,
            expected.producer_content_digest,
            expected.producer_content_id,
        ],
    )?;
    conn.execute_batch(&format!(
        "DROP TABLE IF EXISTS {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE}; \
         ALTER TABLE {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE} \
         RENAME TO {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE};"
    ))?;
    let observed = read_manifest_record(conn)?;
    if !persisted_record_matches(&observed, &expected) {
        bail!("funds producer content manifest v1 persistence verification failed");
    }
    Ok(expected)
}

pub(super) fn cleanup_funds_producer_content_manifest_v1_staging(conn: &Connection) -> Result<()> {
    conn.execute_batch(&format!(
        "DROP TABLE IF EXISTS {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE}"
    ))
    .context("drop funds producer content manifest staging table")?;
    Ok(())
}

pub(super) fn verify_funds_producer_content_manifest_v1(
    conn: &Connection,
    args: &MaterializeArgs,
) -> Result<FundsProducerContentManifestV1Record> {
    let observed = read_manifest_record(conn)?;
    let expected =
        build_funds_producer_content_manifest_v1(conn, args, observed.raw_manifest_sha256())?;
    if !persisted_record_matches(&observed, &expected) {
        bail!("funds producer content manifest v1 is stale");
    }
    Ok(expected)
}

fn build_funds_producer_content_manifest_v1(
    conn: &Connection,
    args: &MaterializeArgs,
    raw_artifact_manifest_sha256: &str,
) -> Result<FundsProducerContentManifestV1Record> {
    if args.case_id.is_empty()
        || args.case_id != args.case_id.trim()
        || args.source_revision <= 0
        || args.source_row_count < 0
        || !crate::is_sha256_hex(raw_artifact_manifest_sha256)
    {
        bail!("funds producer content manifest v1 input is invalid");
    }
    for table in [
        "fc_transaction_norm",
        "import_file_log",
        DETAIL_TABLE,
        AGG_TABLE,
        KEYWORD_TABLE,
        ACCOUNT_DIM_TABLE,
    ] {
        require_base_table(conn, table)?;
    }
    require_funds_analytical_schema_v1(conn)?;
    let duckdb_version = conn.query_row("SELECT version()", [], |row| row.get::<_, String>(0))?;
    if duckdb_version != REQUIRED_DUCKDB_VERSION {
        bail!("funds producer DuckDB version is not admitted");
    }

    let source_scope = conn.query_row(
        "SELECT COUNT(*), \
                SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END), \
                SUM(CASE WHEN case_id IS NULL OR TRIM(CAST(case_id AS VARCHAR))='' THEN 1 ELSE 0 END) \
           FROM fc_transaction_norm",
        [args.case_id.as_str()],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, Option<i64>>(1)?.unwrap_or(0),
                row.get::<_, Option<i64>>(2)?.unwrap_or(0),
            ))
        },
    )?;
    if source_scope != (args.source_row_count, args.source_row_count, 0) {
        bail!("funds producer content contains cross-case transaction source rows");
    }

    let (raw_sources, raw_source_row_count) = load_raw_sources(conn, &args.case_id)?;
    let import_scope = conn.query_row(
        "SELECT COUNT(*), SUM(CASE WHEN case_id=? THEN 1 ELSE 0 END) \
           FROM import_file_log WHERE kind='fc_transaction'",
        [args.case_id.as_str()],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, Option<i64>>(1)?.unwrap_or(0),
            ))
        },
    )?;
    if import_scope.0 != raw_sources.len() as i64 || import_scope.1 != raw_sources.len() as i64 {
        bail!("funds producer content contains cross-case transaction source manifests");
    }
    if raw_source_row_count != args.source_row_count {
        bail!("funds producer transaction source manifest row count is unverified");
    }

    let rejected_row_count = conn.query_row(
        "SELECT SUM(CASE WHEN clean_invalid=1 OR clean_failed=1 OR clean_reversal=1 \
                         THEN 1 ELSE 0 END) \
           FROM fc_transaction_norm WHERE case_id=?",
        [args.case_id.as_str()],
        |row| Ok(row.get::<_, Option<i64>>(0)?.unwrap_or(0)),
    )?;
    if rejected_row_count < 0 || rejected_row_count > args.source_row_count {
        bail!("funds producer transaction rejection state is invalid");
    }

    require_unique_content_keys(conn, args)?;

    let normalized = table_content_manifest(
        conn,
        "fc_transaction_norm",
        &format!("\"case_id\"={}", sql_literal(&args.case_id)),
    )?;
    let detail = table_content_manifest(conn, DETAIL_TABLE, "")?;
    let aggregate = table_content_manifest(conn, AGG_TABLE, "")?;
    let keyword = table_content_manifest(conn, KEYWORD_TABLE, "")?;
    let account = table_content_manifest(conn, ACCOUNT_DIM_TABLE, "")?;
    let aggregate_row_count =
        crate::scalar_i64(conn, &format!("SELECT COUNT(*) FROM {AGG_TABLE}"))?;
    if normalized.row_count != args.source_row_count
        || detail.row_count != args.source_row_count - rejected_row_count
        || aggregate.row_count != aggregate_row_count
    {
        bail!("funds producer content row coverage is inconsistent");
    }

    let raw_source_manifest_bytes = serde_json::to_vec(&raw_sources)?;
    if raw_artifact_manifest_sha256 == sha256_bytes(&raw_source_manifest_bytes) {
        bail!("typed raw artifact manifest binding cannot be a simplified raw-source inventory");
    }
    let payload = FundsProducerContentManifestV1Payload {
        accepted_row_count: detail.row_count,
        account_content_sha256: account.content_sha256,
        account_row_count: account.row_count,
        aggregate_content_sha256: aggregate.content_sha256,
        aggregate_row_count: aggregate.row_count,
        canonical_encoder: DUCKDB_CONTENT_MANIFEST_ALGORITHM.to_string(),
        case_id: args.case_id.clone(),
        contract: FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT.to_string(),
        detail_content_sha256: detail.content_sha256,
        detail_row_count: detail.row_count,
        duckdb_version,
        duplicate_row_count: 0,
        keyword_content_sha256: keyword.content_sha256,
        keyword_row_count: keyword.row_count,
        normalized_content_sha256: normalized.content_sha256,
        normalized_row_count: normalized.row_count,
        producer_component_id: PRODUCER_COMPONENT_ID.to_string(),
        producer_component_version: PRODUCER_COMPONENT_VERSION.to_string(),
        producer_operation: PRODUCER_OPERATION.to_string(),
        producer_operation_schema_hash: sha256_bytes(PRODUCER_OPERATION_SCHEMA.as_bytes()),
        raw_manifest_sha256: raw_artifact_manifest_sha256.to_string(),
        rejected_row_count,
        schema_version: FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION,
        source_revision: args.source_revision,
    };
    let canonical = serde_json::to_vec(&payload)?;
    let manifest_sha256 = sha256_bytes(&canonical);
    let mut content_material =
        Vec::with_capacity(FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN.len() + canonical.len());
    content_material.extend_from_slice(FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN);
    content_material.extend_from_slice(&canonical);
    let producer_content_digest = sha256_bytes(&content_material);
    let producer_content_id =
        format!("{FUNDS_PRODUCER_CONTENT_ID_PREFIX}{producer_content_digest}");
    Ok(FundsProducerContentManifestV1Record {
        payload,
        manifest_sha256,
        producer_content_digest,
        producer_content_id,
        manifest_bytes: canonical,
        raw_source_manifest_bytes,
    })
}

pub(crate) fn funds_analytical_schema_digest_v1() -> String {
    sha256_bytes(FUNDS_ANALYTICAL_SCHEMA_CANONICAL_V1.as_bytes())
}

fn require_funds_analytical_schema_v1(conn: &Connection) -> Result<()> {
    let required_relations: [(&str, &[&str]); 3] = [
        (
            "fc_transaction_norm",
            &["case_id", "clean_amount", "file_id", "id", "row_no"],
        ),
        (
            crate::DETAIL_TABLE,
            &[
                "acct_key",
                "acct_no",
                "amount_parse_failed",
                "amount_source_present",
                "card_no",
                "counterparty_bank",
                "counterparty_name",
                "cp_key",
                "currency",
                "dc_val",
                "file_id",
                "id",
                "txn_ts",
            ],
        ),
        (
            crate::ACCOUNT_DIM_TABLE,
            &["account_key", "acct_type", "bank_name"],
        ),
    ];
    for (relation, required_columns) in required_relations {
        require_base_table(conn, relation)?;
        let columns = crate::table_columns(conn, relation)?
            .into_iter()
            .collect::<BTreeSet<_>>();
        if required_columns
            .iter()
            .any(|column| !columns.contains(*column))
        {
            bail!("funds analytical schema v1 is unavailable");
        }
    }
    for (relation, column, expected_type) in [
        ("fc_transaction_norm", "clean_amount", "VARCHAR"),
        ("fc_transaction_norm", "file_id", "VARCHAR"),
        ("fc_transaction_norm", "row_no", "BIGINT"),
    ] {
        require_exact_column_type_v1(conn, relation, column, expected_type)?;
    }
    if FUNDS_ANALYTICAL_SCHEMA_CONTRACT_V1 != "analytix.funds-analytical-schema/v1"
        || !crate::is_sha256_hex(&funds_analytical_schema_digest_v1())
    {
        bail!("funds analytical schema v1 identity is invalid");
    }
    Ok(())
}

fn require_exact_column_type_v1(
    conn: &Connection,
    relation: &str,
    column: &str,
    expected_type: &str,
) -> Result<()> {
    let current_catalog = conn.query_row("SELECT current_database()", [], |row| {
        row.get::<_, String>(0)
    })?;
    let mut statement = conn.prepare(
        "SELECT table_catalog, table_schema, data_type, COALESCE(collation_name, '') \
           FROM information_schema.columns \
          WHERE table_schema='main' AND table_name=? AND column_name=? \
          ORDER BY table_catalog, data_type, collation_name",
    )?;
    let rows = statement
        .query_map([relation, column], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, String>(3)?,
            ))
        })?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    if rows.len() != 1
        || rows[0].0 != current_catalog
        || rows[0].1 != "main"
        || !rows[0].2.eq_ignore_ascii_case(expected_type)
        || !rows[0].3.is_empty()
    {
        bail!("funds analytical schema v1 exact type is unavailable");
    }
    Ok(())
}

fn load_raw_sources(conn: &Connection, case_id: &str) -> Result<(Vec<RawSourceIdentity>, i64)> {
    let mut statement = conn.prepare(
        "SELECT file_id, sha256, rows_imported_norm, status, cleaned_status \
           FROM import_file_log \
          WHERE case_id=? AND kind='fc_transaction' \
          ORDER BY file_id",
    )?;
    let rows = statement.query_map([case_id], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?,
            row.get::<_, Option<String>>(1)?,
            row.get::<_, Option<i64>>(2)?,
            row.get::<_, Option<String>>(3)?,
            row.get::<_, Option<String>>(4)?,
        ))
    })?;
    let mut out = Vec::new();
    let mut seen = BTreeSet::new();
    let mut row_count = 0_i64;
    for row in rows {
        let (file_id, sha256, rows_imported_norm, status, cleaned_status) = row?;
        let file_id = file_id.unwrap_or_default();
        let sha256 = sha256.unwrap_or_default();
        let status = status.unwrap_or_default();
        let cleaned_status = cleaned_status.unwrap_or_default();
        let Some(rows_imported_norm) = rows_imported_norm else {
            bail!("funds producer transaction source manifest is incomplete");
        };
        if file_id.is_empty()
            || file_id != file_id.trim()
            || !seen.insert(file_id.clone())
            || !crate::is_sha256_hex(&sha256)
            || rows_imported_norm < 0
            || status != "已完成"
            || cleaned_status != "done"
        {
            bail!("funds producer transaction source manifest is incomplete");
        }
        row_count = row_count
            .checked_add(rows_imported_norm)
            .context("dataset snapshot transaction source row count overflow")?;
        out.push(RawSourceIdentity {
            cleaned_status,
            file_id,
            rows_imported_norm,
            sha256,
            status,
        });
    }
    if out.is_empty() {
        bail!("funds producer content has no transaction source manifest");
    }
    Ok((out, row_count))
}

pub(crate) fn require_base_table(conn: &Connection, table: &str) -> Result<()> {
    if !valid_table_identifier(table) {
        bail!("funds producer table identity is invalid");
    }
    let mut statement = conn.prepare(
        "SELECT table_catalog, table_schema, table_type \
           FROM information_schema.tables \
          WHERE table_name=? \
          ORDER BY table_catalog, table_schema, table_type",
    )?;
    let rows = statement
        .query_map([table], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
            ))
        })?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    let current_catalog = conn.query_row("SELECT current_database()", [], |row| {
        row.get::<_, String>(0)
    })?;
    if rows.len() != 1
        || rows[0].0 != current_catalog
        || rows[0].1 != "main"
        || rows[0].2.to_ascii_uppercase() != "BASE TABLE"
    {
        bail!("funds producer requires an immutable base table: {table}");
    }
    Ok(())
}

fn require_unique_content_keys(conn: &Connection, args: &MaterializeArgs) -> Result<()> {
    let source_ids = conn.query_row(
        "SELECT COUNT(*), COUNT(TRY_CAST(id AS BIGINT)), \
                COUNT(DISTINCT TRY_CAST(id AS BIGINT)) \
           FROM fc_transaction_norm WHERE case_id=?",
        [args.case_id.as_str()],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        },
    )?;
    if source_ids
        != (
            args.source_row_count,
            args.source_row_count,
            args.source_row_count,
        )
    {
        bail!("funds producer normalized row key is ambiguous");
    }
    require_unique_key(
        conn,
        DETAIL_TABLE,
        "id IS NULL OR TRY_CAST(id AS BIGINT) IS NULL",
        "TRY_CAST(id AS BIGINT)",
    )?;
    require_unique_key(
        conn,
        AGG_TABLE,
        "FALSE",
        "acct_key, txn_day, cp_key, dc_val",
    )?;
    require_unique_key(
        conn,
        KEYWORD_TABLE,
        "txn_row_id IS NULL OR TRY_CAST(txn_row_id AS BIGINT) IS NULL \
         OR kind IS NULL OR token IS NULL OR token_order IS NULL",
        "TRY_CAST(txn_row_id AS BIGINT), kind, token, token_order",
    )?;
    require_unique_key(
        conn,
        ACCOUNT_DIM_TABLE,
        "account_key IS NULL",
        "account_key",
    )?;
    Ok(())
}

fn require_unique_key(
    conn: &Connection,
    table: &str,
    invalid_key_sql: &str,
    group_by_sql: &str,
) -> Result<()> {
    let invalid = crate::scalar_i64(
        conn,
        &format!("SELECT COUNT(*) FROM {table} WHERE {invalid_key_sql}"),
    )?;
    let duplicates = crate::scalar_i64(
        conn,
        &format!(
            "SELECT COUNT(*) FROM (SELECT 1 FROM {table} \
              GROUP BY {group_by_sql} HAVING COUNT(*)<>1) AS duplicate_keys"
        ),
    )?;
    if invalid != 0 || duplicates != 0 {
        bail!("funds producer content ordering key is invalid: {table}");
    }
    Ok(())
}

pub(super) fn table_content_manifest(
    conn: &Connection,
    table: &str,
    where_sql: &str,
) -> Result<TableContentManifest> {
    if !valid_table_identifier(table) {
        bail!("funds producer table identity is invalid");
    }
    require_base_table(conn, table)?;
    let mut schema = conn.prepare(
        "SELECT column_name, data_type, collation_name \
           FROM information_schema.columns \
          WHERE table_schema='main' AND table_name=? \
          ORDER BY ordinal_position",
    )?;
    let raw_columns = schema
        .query_map([table], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, Option<String>>(2)?,
            ))
        })?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    let mut columns = Vec::with_capacity(raw_columns.len());
    for (name, data_type, collation_name) in raw_columns {
        if name.is_empty()
            || name != name.trim()
            || collation_name
                .as_deref()
                .is_some_and(|value| !value.is_empty())
            || !supported_content_type(&data_type)
        {
            bail!("funds producer table schema is not admitted: {table}");
        }
        columns.push(ColumnManifest { name, data_type });
    }
    if columns.is_empty() {
        bail!("funds producer table schema is unavailable: {table}");
    }
    let quoted_columns = columns
        .iter()
        .map(|column| quote_identifier(&column.name))
        .collect::<Vec<_>>();
    let order_sql = quoted_columns
        .iter()
        .map(|column| format!("{column} NULLS FIRST"))
        .collect::<Vec<_>>()
        .join(", ");
    let mut sql = format!(
        "SELECT {} FROM {} AS r",
        quoted_columns.join(", "),
        quote_identifier(table)
    );
    if !where_sql.is_empty() {
        sql.push_str(" WHERE ");
        sql.push_str(where_sql);
    }
    sql.push_str(" ORDER BY ");
    sql.push_str(&order_sql);
    sql.push_str(", to_json(r)");

    let mut digest = Sha256::new();
    hash_json_line(
        &mut digest,
        &TableManifestHeader {
            columns: &columns,
            schema_version: 1,
            table,
        },
    )?;
    let mut statement = conn.prepare(&sql)?;
    let mut rows = statement.query([])?;
    let mut row_count = 0_i64;
    while let Some(row) = rows.next()? {
        let mut values = Vec::with_capacity(columns.len());
        for (index, column) in columns.iter().enumerate() {
            values.push(canonical_hash_value(
                row.get_ref(index)?,
                &column.data_type,
            )?);
        }
        hash_json_line(&mut digest, &values)?;
        row_count = row_count
            .checked_add(1)
            .context("funds producer table row count overflow")?;
    }
    hash_json_line(&mut digest, &TableManifestFooter { row_count })?;
    Ok(TableContentManifest {
        content_sha256: format!("{:x}", digest.finalize()),
        row_count,
    })
}

fn valid_table_identifier(value: &str) -> bool {
    value.bytes().enumerate().all(|(index, byte)| {
        if index == 0 {
            byte.is_ascii_lowercase() || byte == b'_'
        } else {
            byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'_'
        }
    }) && !value.is_empty()
}

fn supported_content_type(value: &str) -> bool {
    let data_type = value.trim().to_ascii_uppercase();
    if matches!(
        data_type.as_str(),
        "BOOLEAN"
            | "TINYINT"
            | "SMALLINT"
            | "INTEGER"
            | "BIGINT"
            | "HUGEINT"
            | "UTINYINT"
            | "USMALLINT"
            | "UINTEGER"
            | "UBIGINT"
            | "FLOAT"
            | "DOUBLE"
            | "DATE"
            | "TIME"
            | "TIMESTAMP"
            | "TIMESTAMP_S"
            | "TIMESTAMP_MS"
            | "VARCHAR"
            | "BLOB"
    ) {
        return true;
    }
    decimal_precision_and_scale(&data_type)
        .is_some_and(|(precision, scale)| precision <= 28 && scale <= 28 && scale <= precision)
}

fn decimal_precision_and_scale(value: &str) -> Option<(u32, u32)> {
    let body = value.strip_prefix("DECIMAL(")?.strip_suffix(')')?;
    let (precision, scale) = body.split_once(',')?;
    Some((precision.trim().parse().ok()?, scale.trim().parse().ok()?))
}

fn canonical_hash_value(value: ValueRef<'_>, declared_type: &str) -> Result<Value> {
    if matches!(value, ValueRef::Null) {
        return Ok(json!({"type": "null"}));
    }
    let declared_type = declared_type.trim().to_ascii_uppercase();
    let tagged = match value {
        ValueRef::Boolean(value) => json!({"type": "boolean", "value": value}),
        ValueRef::TinyInt(value) => integer_value(value),
        ValueRef::SmallInt(value) => integer_value(value),
        ValueRef::Int(value) => integer_value(value),
        ValueRef::BigInt(value) => integer_value(value),
        ValueRef::HugeInt(value) => integer_value(value),
        ValueRef::UTinyInt(value) => integer_value(value),
        ValueRef::USmallInt(value) => integer_value(value),
        ValueRef::UInt(value) => integer_value(value),
        ValueRef::UBigInt(value) => integer_value(value),
        ValueRef::Float(value) => float_value(value as f64)?,
        ValueRef::Double(value) => float_value(value)?,
        ValueRef::Decimal(value) => json!({"type": "decimal", "value": value.to_string()}),
        ValueRef::Timestamp(unit, value) => {
            let with_timezone = declared_type == "TIMESTAMP WITH TIME ZONE";
            json!({
                "type": "temporal",
                "value": timestamp_iso(unit, value, with_timezone)?,
            })
        }
        ValueRef::Date32(value) => json!({"type": "temporal", "value": date_iso(value)?}),
        ValueRef::Time64(unit, value) => {
            if declared_type == "TIME WITH TIME ZONE" {
                bail!("funds producer content type is not admitted");
            }
            json!({"type": "temporal", "value": time_iso(unit, value)?})
        }
        ValueRef::Text(value) => json!({
            "type": "text",
            "value": std::str::from_utf8(value)
                .context("dataset snapshot contains invalid UTF-8")?,
        }),
        ValueRef::Blob(value) => json!({"type": "bytes", "value": hex_bytes(value)}),
        ValueRef::Enum(_, _) => json!({
            "type": "text",
            "value": value.as_str().context("dataset snapshot enum is invalid")?,
        }),
        _ => bail!("funds producer content type is not admitted"),
    };
    Ok(tagged)
}

fn integer_value(value: impl ToString) -> Value {
    json!({"type": "integer", "value": value.to_string()})
}

fn float_value(value: f64) -> Result<Value> {
    if !value.is_finite() {
        bail!("funds producer content contains a non-finite numeric value");
    }
    let normalized = if value == 0.0 { 0.0 } else { value };
    Ok(json!({"type": "float", "value": python_float_hex(normalized)}))
}

fn python_float_hex(value: f64) -> String {
    let bits = value.to_bits();
    let sign = if bits >> 63 == 1 { "-" } else { "" };
    let exponent_bits = ((bits >> 52) & 0x7ff) as i32;
    let fraction = bits & ((1_u64 << 52) - 1);
    if exponent_bits == 0 && fraction == 0 {
        return format!("{sign}0x0.0p+0");
    }
    let (leading, exponent) = if exponent_bits == 0 {
        (0, -1022)
    } else {
        (1, exponent_bits - 1023)
    };
    format!("{sign}0x{leading:x}.{fraction:013x}p{exponent:+}")
}

fn timestamp_iso(unit: TimeUnit, value: i64, with_timezone: bool) -> Result<String> {
    let micros = time_unit_micros(unit, value)?;
    let datetime = DateTime::<Utc>::from_timestamp_micros(micros)
        .context("dataset snapshot timestamp is out of range")?;
    let mut rendered = format_naive_datetime(datetime.naive_utc());
    if with_timezone {
        rendered.push_str("+00:00");
    }
    Ok(rendered)
}

fn date_iso(value: i32) -> Result<String> {
    let ce_days = 719_163_i32
        .checked_add(value)
        .context("dataset snapshot date is out of range")?;
    let date = NaiveDate::from_num_days_from_ce_opt(ce_days)
        .context("dataset snapshot date is out of range")?;
    Ok(date.format("%Y-%m-%d").to_string())
}

fn time_iso(unit: TimeUnit, value: i64) -> Result<String> {
    let micros = time_unit_micros(unit, value)?;
    if !(0..86_400_000_000).contains(&micros) {
        bail!("dataset snapshot time is out of range");
    }
    let seconds = (micros / 1_000_000) as u32;
    let nanos = ((micros % 1_000_000) * 1_000) as u32;
    let time = NaiveTime::from_num_seconds_from_midnight_opt(seconds, nanos)
        .context("dataset snapshot time is out of range")?;
    Ok(format_naive_time(time))
}

fn time_unit_micros(unit: TimeUnit, value: i64) -> Result<i64> {
    match unit {
        TimeUnit::Second => value
            .checked_mul(1_000_000)
            .context("dataset snapshot temporal value is out of range"),
        TimeUnit::Millisecond => value
            .checked_mul(1_000)
            .context("dataset snapshot temporal value is out of range"),
        TimeUnit::Microsecond => Ok(value),
        TimeUnit::Nanosecond => Ok(value / 1_000),
    }
}

fn format_naive_datetime(value: chrono::NaiveDateTime) -> String {
    format!(
        "{} {}",
        value.date().format("%Y-%m-%d"),
        format_naive_time(value.time())
    )
}

fn format_naive_time(value: NaiveTime) -> String {
    if value.nanosecond() == 0 {
        value.format("%H:%M:%S").to_string()
    } else {
        value.format("%H:%M:%S%.6f").to_string()
    }
}

fn read_manifest_record(conn: &Connection) -> Result<FundsProducerContentManifestV1Record> {
    require_base_table(conn, FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE)?;
    let mut statement = conn.prepare(&format!(
        "SELECT schema_version, contract, case_id, source_revision, \
                producer_component_id, producer_component_version, producer_operation, \
                producer_operation_schema_hash, duckdb_version, canonical_encoder, \
                raw_manifest_sha256, normalized_content_sha256, detail_content_sha256, \
                aggregate_content_sha256, keyword_content_sha256, account_content_sha256, \
                normalized_row_count, accepted_row_count, rejected_row_count, duplicate_row_count, \
                detail_row_count, aggregate_row_count, keyword_row_count, account_row_count, \
                manifest_sha256, producer_content_digest, producer_content_id \
           FROM {FUNDS_PRODUCER_CONTENT_MANIFEST_V1_TABLE} ORDER BY case_id"
    ))?;
    let mut rows = statement.query([])?;
    let Some(row) = rows.next()? else {
        bail!("funds producer content manifest v1 is unavailable");
    };
    let record = FundsProducerContentManifestV1Record {
        payload: FundsProducerContentManifestV1Payload {
            schema_version: row.get(0)?,
            contract: row.get(1)?,
            case_id: row.get(2)?,
            source_revision: row.get(3)?,
            producer_component_id: row.get(4)?,
            producer_component_version: row.get(5)?,
            producer_operation: row.get(6)?,
            producer_operation_schema_hash: row.get(7)?,
            duckdb_version: row.get(8)?,
            canonical_encoder: row.get(9)?,
            raw_manifest_sha256: row.get(10)?,
            normalized_content_sha256: row.get(11)?,
            detail_content_sha256: row.get(12)?,
            aggregate_content_sha256: row.get(13)?,
            keyword_content_sha256: row.get(14)?,
            account_content_sha256: row.get(15)?,
            normalized_row_count: row.get(16)?,
            accepted_row_count: row.get(17)?,
            rejected_row_count: row.get(18)?,
            duplicate_row_count: row.get(19)?,
            detail_row_count: row.get(20)?,
            aggregate_row_count: row.get(21)?,
            keyword_row_count: row.get(22)?,
            account_row_count: row.get(23)?,
        },
        manifest_sha256: row.get(24)?,
        producer_content_digest: row.get(25)?,
        producer_content_id: row.get(26)?,
        manifest_bytes: Vec::new(),
        raw_source_manifest_bytes: Vec::new(),
    };
    if rows.next()?.is_some() {
        bail!("funds producer content manifest v1 is ambiguous");
    }
    let canonical = serde_json::to_vec(&record.payload)?;
    let expected_manifest_sha256 = sha256_bytes(&canonical);
    let mut content_material =
        Vec::with_capacity(FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN.len() + canonical.len());
    content_material.extend_from_slice(FUNDS_PRODUCER_CONTENT_DIGEST_DOMAIN);
    content_material.extend_from_slice(&canonical);
    let expected_digest = sha256_bytes(&content_material);
    if record.payload.schema_version != FUNDS_PRODUCER_CONTENT_MANIFEST_V1_SCHEMA_VERSION
        || record.payload.contract != FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT
        || record.payload.duckdb_version != REQUIRED_DUCKDB_VERSION
        || record.payload.canonical_encoder != DUCKDB_CONTENT_MANIFEST_ALGORITHM
        || record.payload.producer_component_id != PRODUCER_COMPONENT_ID
        || record.payload.producer_component_version != PRODUCER_COMPONENT_VERSION
        || record.payload.producer_operation != PRODUCER_OPERATION
        || record.payload.producer_operation_schema_hash
            != sha256_bytes(PRODUCER_OPERATION_SCHEMA.as_bytes())
        || record.manifest_sha256 != expected_manifest_sha256
        || record.producer_content_digest != expected_digest
        || record.producer_content_id
            != format!("{FUNDS_PRODUCER_CONTENT_ID_PREFIX}{expected_digest}")
        || !manifest_hashes_are_canonical(&record)
        || record.payload.accepted_row_count
            + record.payload.rejected_row_count
            + record.payload.duplicate_row_count
            != record.payload.normalized_row_count
        || record.payload.accepted_row_count != record.payload.detail_row_count
    {
        bail!("funds producer content manifest v1 is invalid");
    }
    Ok(FundsProducerContentManifestV1Record {
        manifest_bytes: canonical,
        ..record
    })
}

fn persisted_record_matches(
    observed: &FundsProducerContentManifestV1Record,
    expected: &FundsProducerContentManifestV1Record,
) -> bool {
    observed.payload == expected.payload
        && observed.manifest_sha256 == expected.manifest_sha256
        && observed.producer_content_digest == expected.producer_content_digest
        && observed.producer_content_id == expected.producer_content_id
}

fn manifest_hashes_are_canonical(record: &FundsProducerContentManifestV1Record) -> bool {
    [
        record.payload.raw_manifest_sha256.as_str(),
        record.payload.normalized_content_sha256.as_str(),
        record.payload.detail_content_sha256.as_str(),
        record.payload.aggregate_content_sha256.as_str(),
        record.payload.keyword_content_sha256.as_str(),
        record.payload.account_content_sha256.as_str(),
        record.payload.producer_operation_schema_hash.as_str(),
        record.manifest_sha256.as_str(),
        record.producer_content_digest.as_str(),
    ]
    .into_iter()
    .all(crate::is_sha256_hex)
}

fn sha256_bytes(value: &[u8]) -> String {
    format!("{:x}", Sha256::digest(value))
}

fn hash_json_line(digest: &mut Sha256, value: &impl Serialize) -> Result<()> {
    serde_json::to_writer(&mut *digest, value)?;
    digest.update(b"\n");
    Ok(())
}

fn quote_identifier(value: &str) -> String {
    format!("\"{}\"", value.replace('"', "\"\""))
}

fn hex_bytes(value: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(value.len() * 2);
    for byte in value {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::path::PathBuf;
    use std::time::{SystemTime, UNIX_EPOCH};

    struct TempDb(PathBuf);

    impl TempDb {
        fn new(label: &str) -> Self {
            let unique = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock after UNIX epoch")
                .as_nanos();
            Self(std::env::temp_dir().join(format!(
                "analytix-{label}-{}-{unique}.duckdb",
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

    fn seed_producer_fixture(path: &std::path::Path) {
        let conn = Connection::open(path).expect("open producer fixture");
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
        .expect("seed producer fixture");
    }

    fn producer_args(path: &std::path::Path, revision: i64) -> MaterializeArgs {
        MaterializeArgs {
            case_id: "case-a".to_string(),
            db_path: path.to_path_buf(),
            source_revision: revision,
            source_row_count: 2,
            source_max_txn_ts: "2026-01-02 01:02:03".to_string(),
            source_max_id: 2,
        }
    }

    #[test]
    fn python_float_hex_matches_python_contract_vectors() {
        for (value, expected) in [
            (0.0, "0x0.0p+0"),
            (-0.0, "-0x0.0p+0"),
            (1.0, "0x1.0000000000000p+0"),
            (1.5, "0x1.8000000000000p+0"),
            (0.1, "0x1.999999999999ap-4"),
            (f64::from_bits(1), "0x0.0000000000001p-1022"),
            (f64::MAX, "0x1.fffffffffffffp+1023"),
        ] {
            assert_eq!(python_float_hex(value), expected);
        }
        assert_eq!(
            float_value(-0.0).expect("normalize signed zero"),
            json!({"type": "float", "value": "0x0.0p+0"})
        );
    }

    #[test]
    fn production_materializer_publishes_consumer_exact_content_roots_atomically() {
        let db = TempDb::new("funds-producer-content-v1");
        seed_producer_fixture(&db.0);
        super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
            &producer_args(&db.0, 7),
            &"1".repeat(64),
        )
        .expect("materialize producer fixture");

        let conn = Connection::open(&db.0).expect("open materialized fixture");
        let baseline = read_manifest_record(&conn).expect("read persisted producer manifest");
        assert_eq!(baseline.payload.schema_version, 1);
        assert_eq!(
            baseline.payload.contract,
            FUNDS_PRODUCER_CONTENT_MANIFEST_V1_CONTRACT
        );
        assert_eq!(baseline.payload.case_id, "case-a");
        assert_eq!(baseline.payload.source_revision, 7);
        assert_eq!(baseline.payload.duckdb_version, REQUIRED_DUCKDB_VERSION);
        assert_eq!(baseline.payload.normalized_row_count, 2);
        assert_eq!(baseline.payload.accepted_row_count, 2);
        assert_eq!(baseline.payload.rejected_row_count, 0);
        assert_eq!(baseline.payload.detail_row_count, 2);
        assert_eq!(baseline.payload.aggregate_row_count, 2);
        assert!(baseline
            .producer_content_id
            .starts_with(FUNDS_PRODUCER_CONTENT_ID_PREFIX));
        assert_eq!(
            baseline.producer_content_id.len(),
            FUNDS_PRODUCER_CONTENT_ID_PREFIX.len() + 64
        );
        let baseline_identity = conn
            .query_row(
                "SELECT agg_name FROM analysis_materialization_meta",
                [],
                |row| row.get::<_, String>(0),
            )
            .expect("read baseline identity");
        let baseline_amount = conn
            .query_row(
                "SELECT amount FROM analysis_txn_detail_idx WHERE id=1",
                [],
                |row| row.get::<_, f64>(0),
            )
            .expect("read baseline detail");
        conn.execute_batch(
            "UPDATE fc_transaction_norm SET amount=99.0 WHERE id=1; \
             UPDATE analysis_revision_state SET revision=8 \
              WHERE revision_key='stats_flow_source'; \
             UPDATE import_file_log SET sha256= \
               'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'; \
             CREATE VIEW analysis_funds_content_manifest_v1__staging AS \
               SELECT 1 AS collision;",
        )
        .expect("prepare manifest publication fault");
        drop(conn);

        let error =
            match super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
                &producer_args(&db.0, 8),
                &"2".repeat(64),
            ) {
                Ok(_) => panic!("manifest staging collision must fail publication"),
                Err(error) => error,
            };
        assert!(
            error
                .to_string()
                .contains("analysis_funds_content_manifest_v1__staging")
                && error.to_string().contains("drop type Table"),
            "unexpected error: {error:#}"
        );

        let conn = Connection::open(&db.0).expect("reopen failed publication fixture");
        assert_eq!(
            read_manifest_record(&conn).expect("old producer manifest survives rollback"),
            baseline
        );
        assert_eq!(
            conn.query_row(
                "SELECT agg_name FROM analysis_materialization_meta",
                [],
                |row| row.get::<_, String>(0),
            )
            .expect("old metadata survives rollback"),
            baseline_identity
        );
        assert_eq!(
            conn.query_row(
                "SELECT amount FROM analysis_txn_detail_idx WHERE id=1",
                [],
                |row| row.get::<_, f64>(0),
            )
            .expect("old result survives rollback"),
            baseline_amount
        );
        for table in [
            crate::AGG_STAGING_TABLE,
            crate::DETAIL_STAGING_TABLE,
            crate::KEYWORD_STAGING_TABLE,
            crate::ACCOUNT_DIM_STAGING_TABLE,
        ] {
            let object_count = conn
                .query_row(
                    "SELECT COUNT(*) FROM information_schema.tables \
                      WHERE table_schema='main' AND table_name=?",
                    [table],
                    |row| row.get::<_, i64>(0),
                )
                .expect("count rolled-back materializer staging objects");
            assert_eq!(object_count, 0, "staging table must roll back: {table}");
        }
        let collision_count = conn
            .query_row(
                "SELECT COUNT(*) FROM information_schema.views \
                  WHERE table_schema='main' AND table_name=?",
                [FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE],
                |row| row.get::<_, i64>(0),
            )
            .expect("count preexisting manifest collision view");
        assert_eq!(
            collision_count, 1,
            "rollback must preserve the injected collision view"
        );
    }

    #[test]
    fn failed_producer_validation_cleans_owned_table_staging_without_replacing_active_data() {
        let db = TempDb::new("funds-producer-cleanup");
        seed_producer_fixture(&db.0);
        super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
            &producer_args(&db.0, 7),
            &"1".repeat(64),
        )
        .expect("materialize producer baseline");

        let conn = Connection::open(&db.0).expect("open producer cleanup fixture");
        let baseline = read_manifest_record(&conn).expect("read baseline producer manifest");
        let baseline_amount = conn
            .query_row(
                "SELECT amount FROM analysis_txn_detail_idx WHERE id=1",
                [],
                |row| row.get::<_, f64>(0),
            )
            .expect("read baseline detail");
        conn.execute_batch(
            "UPDATE fc_transaction_norm SET amount=99.0 WHERE id=1; \
             UPDATE analysis_revision_state SET revision=8 \
              WHERE revision_key='stats_flow_source'; \
             UPDATE import_file_log SET sha256=UPPER(sha256);",
        )
        .expect("prepare producer validation failure");
        drop(conn);

        let error =
            match super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
                &producer_args(&db.0, 8),
                &"2".repeat(64),
            ) {
                Ok(_) => panic!("noncanonical producer source manifest must fail publication"),
                Err(error) => error,
            };
        assert!(
            error
                .to_string()
                .contains("funds producer transaction source manifest is incomplete"),
            "unexpected error: {error:#}"
        );

        let conn = Connection::open(&db.0).expect("reopen producer cleanup fixture");
        assert_eq!(
            read_manifest_record(&conn).expect("old producer manifest survives rollback"),
            baseline
        );
        assert_eq!(
            conn.query_row(
                "SELECT amount FROM analysis_txn_detail_idx WHERE id=1",
                [],
                |row| row.get::<_, f64>(0),
            )
            .expect("old result survives rollback"),
            baseline_amount
        );
        for table in [
            crate::AGG_STAGING_TABLE,
            crate::DETAIL_STAGING_TABLE,
            crate::KEYWORD_STAGING_TABLE,
            crate::ACCOUNT_DIM_STAGING_TABLE,
            FUNDS_PRODUCER_CONTENT_MANIFEST_V1_STAGING_TABLE,
        ] {
            let object_count = conn
                .query_row(
                    "SELECT COUNT(*) FROM information_schema.tables \
                      WHERE table_schema='main' AND table_name=?",
                    [table],
                    |row| row.get::<_, i64>(0),
                )
                .expect("count materializer staging objects");
            assert_eq!(object_count, 0, "staging object must be removed: {table}");
        }
    }

    #[test]
    fn funds_analytical_schema_digest_matches_cross_language_golden() {
        assert_eq!(
            FUNDS_ANALYTICAL_SCHEMA_CONTRACT_V1,
            "analytix.funds-analytical-schema/v1"
        );
        assert_eq!(
            funds_analytical_schema_digest_v1(),
            "81a474553b16799c12d8d85c28d99f25ba6c9b40ae2073701443d2898f91e11e"
        );
    }

    #[test]
    fn producer_rejects_schema_label_without_exact_query_types() {
        let db = TempDb::new("funds-schema-type");
        seed_producer_fixture(&db.0);
        Connection::open(&db.0)
            .expect("open schema drift fixture")
            .execute_batch("ALTER TABLE fc_transaction_norm ALTER clean_amount TYPE DECIMAL(28,2)")
            .expect("drift exact amount type");
        let error =
            match super::super::materialize::materialize_txn_daily_with_raw_artifact_manifest_sha256(
                &producer_args(&db.0, 7),
                &"1".repeat(64),
            ) {
                Ok(_) => panic!("wrong physical schema acquired the fixed analytical digest"),
                Err(error) => error,
            };
        assert!(error
            .to_string()
            .contains("funds analytical schema v1 exact type is unavailable"));
        let conn = Connection::open(&db.0).expect("reopen rejected schema fixture");
        assert_eq!(
            crate::scalar_i64(
                &conn,
                "SELECT COUNT(*) FROM information_schema.tables \
                  WHERE table_schema='main' \
                    AND table_name='analysis_funds_content_manifest_v1'",
            )
            .expect("count rejected producer manifest"),
            0
        );
    }
}
