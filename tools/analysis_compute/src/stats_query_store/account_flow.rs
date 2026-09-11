use std::fmt;

use anyhow::{anyhow, bail, Result};
use chrono::{DateTime, Duration, SecondsFormat, Utc};
use duckdb::types::ValueRef;
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

use super::session::{VerifiedStatsQueryRequest, VerifiedStatsQuerySession};

pub(crate) const ANALYZE_ACCOUNT_FLOWS_COMMAND: &str = "analyze_account_flows";

const ACCOUNT_FLOW_QUERY_CONTRACT: &str = "analytix.account-flow-query/v1";
const ACCOUNT_FLOW_QUERY_HASH_DOMAIN: &[u8] = b"analytix.account-flow-query-hash/v1";
const ACCOUNT_FLOW_RESULT_HASH_DOMAIN: &[u8] = b"analytix.account-flow-result-hash/v1";
const ACCOUNT_FLOW_QUERY_SQL_HASH_DOMAIN: &[u8] = b"analytix.account-flow-query-sql/v1";
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_RESOLVED_ACCOUNT_KEY_BYTES: usize = 4_096;
const MAX_TIMESTAMP_BYTES: usize = 64;
const MAX_AMOUNT_TEXT_BYTES: usize = 128;
const MAX_SOURCE_FILE_ID_BYTES: usize = 4_096;
const MAX_DIRECTION_BYTES: usize = 16;
const MAX_CURRENCY_BYTES: usize = 16;
const MAX_AMOUNT_TYPE_BYTES: usize = 32;
const MAX_COUNTERPARTY_KEY_BYTES: usize = 4_096;
const MAX_COUNTERPARTY_SEMANTIC_BYTES: usize = 4_096;
const MAX_SCANNED_TEXT_BYTES: usize = 8 * 1024 * 1024;
const MAX_RESULT_BYTES: usize = 1024 * 1024;
const MAX_EVIDENCE_ROW_LIMIT: u32 = 1_000;
const MAX_SCAN_CAP: u32 = 100_000;
const MAX_SOURCE_ROW_NUMBER: i64 = 9_007_199_254_740_991;
const MIN_DATASET_UTC_OFFSET_MINUTES: i16 = -840;
const MAX_DATASET_UTC_OFFSET_MINUTES: i16 = 840;
const REQUIRED_MINOR_UNIT_SCALE: u8 = 2;
const ACCOUNT_FLOW_QUERY_SQL: &str = "\
WITH candidate_rows AS MATERIALIZED (
  SELECT d.id,
         d.file_id,
         d.txn_ts,
         d.dc_val,
         d.amount_source_present,
         d.amount_parse_failed,
         d.currency,
         d.cp_key,
         d.counterparty_name,
         d.counterparty_bank
    FROM analysis_txn_detail_idx d
   WHERE d.acct_key = ?
     AND d.txn_ts >= CAST(? AS TIMESTAMP)
     AND d.txn_ts <= CAST(? AS TIMESTAMP)
   ORDER BY d.txn_ts ASC,
            TRY_CAST(d.id AS BIGINT) ASC,
            CAST(d.id AS VARCHAR) ASC,
            d.file_id ASC
   LIMIT ?
)
SELECT r.file_id AS source_file_id,
       r.row_no AS source_row_number,
       strftime(
         d.txn_ts - (CAST(? AS BIGINT) * INTERVAL '1 minute'),
         '%Y-%m-%dT%H:%M:%S.%fZ'
       ) AS occurred_at,
       d.dc_val AS direction,
       r.clean_amount AS amount_text,
       d.amount_source_present,
       d.amount_parse_failed,
       d.currency,
       typeof(r.clean_amount) AS amount_type,
       d.cp_key AS counterparty_key,
       d.counterparty_name,
       d.counterparty_bank,
       COUNT(*) OVER (PARTITION BY d.file_id, d.id) AS joined_row_count
  FROM candidate_rows d
  LEFT JOIN fc_transaction_norm r
    ON r.case_id = ?
   AND TRY_CAST(r.id AS BIGINT) = TRY_CAST(d.id AS BIGINT)
   AND r.file_id = d.file_id
 ORDER BY d.txn_ts ASC,
          TRY_CAST(d.id AS BIGINT) ASC,
          CAST(d.id AS VARCHAR) ASC,
          d.file_id ASC,
          r.row_no ASC
";

/// Host-private arguments assembled only after current DSV2/case resolution.
/// This serde shape must not be advertised as the provider/MCP tool schema.
#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AnalyzeAccountFlowsHostArguments {
    pub case_id: String,
    /// Authoritative values copied by the Go host from the current
    /// TurnSecurityContextV2 and exact DSV2 selection. Rust binds them into
    /// the query/result but cannot establish their currentness on its own.
    pub dataset_snapshot_id: String,
    pub context_epoch: u64,
    pub context_digest: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub subject_ref: String,
    /// Host-only DSV2 resolution output. It must never be provider/MCP input or output.
    pub resolved_account_key: String,
    /// Installation-keyed host digest binding subject_ref, exact account key,
    /// case binding, and DSV2. The raw key itself is deliberately excluded
    /// from stable query/result hashes.
    pub subject_resolution_digest: String,
    pub start_inclusive: String,
    pub end_inclusive: String,
    pub evidence_row_limit: u32,
    /// Host-only DSV2-derived fixed offset for the exact covered snapshot/query range.
    /// IANA-only, transition-bearing, or otherwise ambiguous timezone semantics must not
    /// reach this operation; the host must not guess an offset from the current instant.
    pub dataset_utc_offset_minutes: i16,
    /// Host-only DSV2-bound currency; every selected row must match it exactly.
    pub expected_currency: String,
    /// Fixed at two for this operation; it is not inferred from source values.
    pub minor_unit_scale: u8,
    pub scan_cap: u32,
}

impl fmt::Debug for AnalyzeAccountFlowsHostArguments {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("AnalyzeAccountFlowsHostArguments")
            .field("case_id", &self.case_id)
            .field("dataset_snapshot_id", &self.dataset_snapshot_id)
            .field("context_epoch", &self.context_epoch)
            .field("context_digest", &self.context_digest)
            .field("case_binding_hash", &self.case_binding_hash)
            .field(
                "expected_producer_content_id",
                &self.expected_producer_content_id,
            )
            .field(
                "expected_producer_manifest_sha256",
                &self.expected_producer_manifest_sha256,
            )
            .field("subject_ref", &self.subject_ref)
            .field("resolved_account_key", &"[REDACTED]")
            .field("subject_resolution_digest", &self.subject_resolution_digest)
            .field("start_inclusive", &self.start_inclusive)
            .field("end_inclusive", &self.end_inclusive)
            .field("evidence_row_limit", &self.evidence_row_limit)
            .field(
                "dataset_utc_offset_minutes",
                &self.dataset_utc_offset_minutes,
            )
            .field("expected_currency", &self.expected_currency)
            .field("minor_unit_scale", &self.minor_unit_scale)
            .field("scan_cap", &self.scan_cap)
            .finish()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountFlowDirection {
    Inflow,
    Outflow,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountFlowCoverageState {
    Complete,
    Partial,
    ObservedNoHitPendingHostBinding,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountFlowCoverageGap {
    RejectedSourceRows,
    DuplicateSourceRows,
    UntimedSubjectRows,
    EvidenceRowLimit,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountFlowCurrentness {
    HostRevalidationRequired,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum AccountFlowSemanticProjectionState {
    HostCounterpartyResolutionRequired,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AccountFlowHostCoverage {
    pub state: AccountFlowCoverageState,
    pub gaps: Vec<AccountFlowCoverageGap>,
    pub normalized_snapshot_rows: u64,
    pub accepted_snapshot_rows: u64,
    pub rejected_snapshot_rows: u64,
    pub duplicate_snapshot_rows: u64,
    pub untimed_subject_rows: u64,
    pub observed_matching_rows: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AccountFlowHostProvenance {
    pub dataset_snapshot_id: String,
    pub context_epoch: u64,
    pub context_digest: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub subject_resolution_digest: String,
    pub duckdb_content_snapshot_digest: String,
    pub duckdb_snapshot_manifest_sha256: String,
    pub materialization_identity: String,
    pub source_signature: String,
    pub result_signature: String,
    pub producer_content_id: String,
    pub producer_manifest_sha256: String,
    pub query_contract: String,
    pub query_sql_hash: String,
}

#[derive(Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AccountFlowHostEvidenceRow {
    pub subject_ref: String,
    /// Private locator input only. The Go host must resolve this pair against
    /// the exact DSV2 source-row ledger and replace it with a real `srow1_`
    /// record before any EvidenceReceipt or ClaimRecord can be issued.
    pub source_file_id: String,
    pub source_row_number: u64,
    /// Private values used only by the host to derive stable case-entity
    /// references and approved semantics. They are never provider/UI fields.
    pub counterparty_key: Option<String>,
    pub counterparty_name: Option<String>,
    pub counterparty_bank: Option<String>,
    pub occurred_at: String,
    pub direction: AccountFlowDirection,
    pub amount_minor: String,
    pub currency: String,
    pub minor_unit_scale: u8,
}

impl fmt::Debug for AccountFlowHostEvidenceRow {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("AccountFlowHostEvidenceRow")
            .field("subject_ref", &self.subject_ref)
            .field("source_file_id", &"[PRIVATE]")
            .field("source_row_number", &"[PRIVATE]")
            .field("counterparty_key", &"[PRIVATE]")
            .field("counterparty_name", &"[PRIVATE]")
            .field("counterparty_bank", &"[PRIVATE]")
            .field("occurred_at", &self.occurred_at)
            .field("direction", &self.direction)
            .field("amount_minor", &self.amount_minor)
            .field("currency", &self.currency)
            .field("minor_unit_scale", &self.minor_unit_scale)
            .finish()
    }
}

/// Crate-private exact compute result. This is not a provider-safe MCP result
/// and must not be exported as a production operation before the existing
/// DSV2 `UseExact` callback owns the connection for the complete call:
/// Go must still bind evidence/claims and use a separate host-private resolution
/// step to project counterparty keys into stable references and authorized safe
/// semantics before publication. Raw counterparty values can exist only in this
/// host-private frame; they must never be forwarded to provider/public surfaces.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct AnalyzeAccountFlowsHostResult {
    pub subject_ref: String,
    pub start_inclusive: String,
    pub end_inclusive: String,
    /// The DSV2-proved fixed offset, rendered deterministically as `Z` or `±HH:MM`.
    pub timezone: String,
    pub currency: String,
    pub minor_unit_scale: u8,
    pub inflow_minor: String,
    pub outflow_minor: String,
    pub net_minor: String,
    pub transaction_count: u64,
    pub aggregate_complete: bool,
    pub evidence_rows_complete: bool,
    pub coverage: AccountFlowHostCoverage,
    pub provenance: AccountFlowHostProvenance,
    pub currentness: AccountFlowCurrentness,
    pub semantic_projection_state: AccountFlowSemanticProjectionState,
    pub evidence_rows: Vec<AccountFlowHostEvidenceRow>,
    pub query_hash: String,
    pub result_hash: String,
}

struct ValidatedAccountFlowArguments<'a> {
    arguments: &'a AnalyzeAccountFlowsHostArguments,
    canonical_start_inclusive: String,
    canonical_end_inclusive: String,
    snapshot_local_start_inclusive: String,
    snapshot_local_end_inclusive: String,
}

struct RawAccountFlowRow {
    source_file_id: String,
    source_row_number: i64,
    occurred_at: String,
    direction: Option<String>,
    amount_text: Option<String>,
    amount_source_present: i64,
    amount_parse_failed: i64,
    currency: Option<String>,
    amount_type: String,
    counterparty_key: Option<String>,
    counterparty_name: Option<String>,
    counterparty_bank: Option<String>,
    joined_row_count: i64,
}

struct AccountFlowAccumulator<'a, 'args> {
    validated: &'a ValidatedAccountFlowArguments<'args>,
    inflow_minor: i128,
    outflow_minor: i128,
    transaction_count: u64,
    evidence_rows: Vec<AccountFlowHostEvidenceRow>,
}

struct AccountFlowSnapshotCoverage {
    normalized_row_count: u64,
    accepted_row_count: u64,
    rejected_row_count: u64,
    duplicate_row_count: u64,
    untimed_subject_rows: u64,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct AccountFlowResultHashMaterial<'a> {
    subject_ref: &'a str,
    start_inclusive: &'a str,
    end_inclusive: &'a str,
    timezone: &'a str,
    currency: &'a str,
    minor_unit_scale: u8,
    inflow_minor: &'a str,
    outflow_minor: &'a str,
    net_minor: &'a str,
    transaction_count: u64,
    aggregate_complete: bool,
    evidence_rows_complete: bool,
    coverage: &'a AccountFlowHostCoverage,
    provenance: &'a AccountFlowHostProvenance,
    currentness: AccountFlowCurrentness,
    semantic_projection_state: AccountFlowSemanticProjectionState,
    evidence_rows: &'a [AccountFlowHostEvidenceRow],
    query_hash: &'a str,
}

pub(crate) fn parse_analyze_account_flows_host_arguments(
    value: &Value,
) -> Result<AnalyzeAccountFlowsHostArguments> {
    serde_json::from_value(value.clone())
        .map_err(|_| anyhow!("account_flow_request_contract_invalid"))
}

pub(crate) fn validate_analyze_account_flows_host_arguments(
    arguments: &AnalyzeAccountFlowsHostArguments,
) -> Result<()> {
    ValidatedAccountFlowArguments::new(arguments).map(|_| ())
}

pub(crate) fn analyze_account_flows(
    conn: &Connection,
    arguments: &AnalyzeAccountFlowsHostArguments,
) -> Result<AnalyzeAccountFlowsHostResult> {
    analyze_account_flows_inner(conn, arguments).map_err(sanitize_account_flow_error)
}

fn analyze_account_flows_inner(
    conn: &Connection,
    arguments: &AnalyzeAccountFlowsHostArguments,
) -> Result<AnalyzeAccountFlowsHostResult> {
    let validated = ValidatedAccountFlowArguments::new(arguments)?;
    let mut session = VerifiedStatsQuerySession::begin(conn, arguments)?;
    require_exact_account_flow_source_schema(session.conn())?;
    let snapshot_binding = session.snapshot_binding();
    if snapshot_binding.producer_content_id != arguments.expected_producer_content_id
        || snapshot_binding.producer_manifest_sha256 != arguments.expected_producer_manifest_sha256
    {
        bail!("account_flow_snapshot_authority_binding_mismatch");
    }
    let duckdb_content_snapshot_digest = snapshot_binding.duckdb_content_snapshot_digest.clone();
    let duckdb_snapshot_manifest_sha256 = snapshot_binding.duckdb_snapshot_manifest_sha256.clone();
    let materialization_identity = snapshot_binding.materialization_identity.clone();
    let source_signature = snapshot_binding.source_signature.clone();
    let result_signature = snapshot_binding.result_signature.clone();
    let producer_content_id = snapshot_binding.producer_content_id.clone();
    let producer_manifest_sha256 = snapshot_binding.producer_manifest_sha256.clone();
    let normalized_row_count = u64::try_from(snapshot_binding.normalized_row_count)
        .map_err(|_| anyhow!("account_flow_snapshot_contract_invalid"))?;
    let accepted_row_count = u64::try_from(snapshot_binding.accepted_row_count)
        .map_err(|_| anyhow!("account_flow_snapshot_contract_invalid"))?;
    let rejected_row_count = u64::try_from(snapshot_binding.rejected_row_count)
        .map_err(|_| anyhow!("account_flow_snapshot_contract_invalid"))?;
    let duplicate_row_count = u64::try_from(snapshot_binding.duplicate_row_count)
        .map_err(|_| anyhow!("account_flow_snapshot_contract_invalid"))?;
    let query_hash = build_query_hash(&duckdb_content_snapshot_digest, &validated)?;
    let untimed_subject_rows = load_untimed_subject_row_count(session.conn(), &validated)?;
    let result = load_and_aggregate_account_flow_rows(
        session.conn(),
        &validated,
        &query_hash,
        AccountFlowSnapshotCoverage {
            normalized_row_count,
            accepted_row_count,
            rejected_row_count,
            duplicate_row_count,
            untimed_subject_rows,
        },
        AccountFlowHostProvenance {
            dataset_snapshot_id: arguments.dataset_snapshot_id.clone(),
            context_epoch: arguments.context_epoch,
            context_digest: arguments.context_digest.clone(),
            case_binding_hash: arguments.case_binding_hash.clone(),
            expected_producer_content_id: arguments.expected_producer_content_id.clone(),
            expected_producer_manifest_sha256: arguments.expected_producer_manifest_sha256.clone(),
            subject_resolution_digest: arguments.subject_resolution_digest.clone(),
            duckdb_content_snapshot_digest,
            duckdb_snapshot_manifest_sha256,
            materialization_identity,
            source_signature,
            result_signature,
            producer_content_id,
            producer_manifest_sha256,
            query_contract: ACCOUNT_FLOW_QUERY_CONTRACT.to_string(),
            query_sql_hash: account_flow_query_sql_hash(),
        },
    )?;
    let row_count = i64::try_from(result.transaction_count)
        .map_err(|_| anyhow!("account_flow_scan_limit_exceeded"))?;
    session.record_observed_range(ACCOUNT_FLOW_QUERY_CONTRACT, &query_hash, row_count)?;
    session.commit()?;
    Ok(result)
}

impl<'a> ValidatedAccountFlowArguments<'a> {
    fn new(arguments: &'a AnalyzeAccountFlowsHostArguments) -> Result<Self> {
        if arguments.case_id.is_empty()
            || arguments.case_id != arguments.case_id.trim()
            || arguments.case_id.len() > MAX_CASE_ID_BYTES
            || arguments.case_id.chars().any(char::is_control)
            || !valid_prefixed_sha256(&arguments.dataset_snapshot_id, "dsv2_")
            || arguments.context_epoch == 0
            || !crate::is_sha256_hex(&arguments.context_digest)
            || !crate::is_sha256_hex(&arguments.case_binding_hash)
            || !valid_prefixed_sha256(&arguments.expected_producer_content_id, "fpc1_")
            || !crate::is_sha256_hex(&arguments.expected_producer_manifest_sha256)
            || !valid_subject_ref(&arguments.subject_ref)
            || arguments.resolved_account_key.is_empty()
            || arguments.resolved_account_key != arguments.resolved_account_key.trim()
            || arguments.resolved_account_key.len() > MAX_RESOLVED_ACCOUNT_KEY_BYTES
            || arguments.resolved_account_key.chars().any(char::is_control)
            || !crate::is_sha256_hex(&arguments.subject_resolution_digest)
            || arguments.evidence_row_limit == 0
            || arguments.evidence_row_limit > MAX_EVIDENCE_ROW_LIMIT
            || arguments.scan_cap == 0
            || arguments.scan_cap > MAX_SCAN_CAP
            || arguments.minor_unit_scale != REQUIRED_MINOR_UNIT_SCALE
            || arguments.dataset_utc_offset_minutes < MIN_DATASET_UTC_OFFSET_MINUTES
            || arguments.dataset_utc_offset_minutes > MAX_DATASET_UTC_OFFSET_MINUTES
            || !valid_currency(&arguments.expected_currency)
        {
            bail!("account_flow_request_contract_invalid");
        }
        let start_inclusive = parse_rfc3339_utc(&arguments.start_inclusive)?;
        let end_inclusive = parse_rfc3339_utc(&arguments.end_inclusive)?;
        if start_inclusive > end_inclusive {
            bail!("account_flow_request_contract_invalid");
        }
        let snapshot_offset = Duration::minutes(i64::from(arguments.dataset_utc_offset_minutes));
        let snapshot_local_start = start_inclusive
            .checked_add_signed(snapshot_offset)
            .ok_or_else(|| anyhow!("account_flow_request_contract_invalid"))?;
        let snapshot_local_end = end_inclusive
            .checked_add_signed(snapshot_offset)
            .ok_or_else(|| anyhow!("account_flow_request_contract_invalid"))?;
        Ok(Self {
            arguments,
            canonical_start_inclusive: canonical_rfc3339_utc(start_inclusive),
            canonical_end_inclusive: canonical_rfc3339_utc(end_inclusive),
            snapshot_local_start_inclusive: canonical_duckdb_timestamp(snapshot_local_start),
            snapshot_local_end_inclusive: canonical_duckdb_timestamp(snapshot_local_end),
        })
    }
}

impl VerifiedStatsQueryRequest for AnalyzeAccountFlowsHostArguments {
    fn case_id(&self) -> &str {
        &self.case_id
    }

    fn command(&self) -> &'static str {
        ANALYZE_ACCOUNT_FLOWS_COMMAND
    }

    fn canonical_scope(&self) -> Value {
        let canonical_start_inclusive = parse_rfc3339_utc(&self.start_inclusive)
            .map(canonical_rfc3339_utc)
            .unwrap_or_else(|_| self.start_inclusive.clone());
        let canonical_end_inclusive = parse_rfc3339_utc(&self.end_inclusive)
            .map(canonical_rfc3339_utc)
            .unwrap_or_else(|_| self.end_inclusive.clone());
        json!({
            "datasetSnapshotId": self.dataset_snapshot_id,
            "contextEpoch": self.context_epoch,
            "contextDigest": self.context_digest,
            "caseBindingHash": self.case_binding_hash,
            "expectedProducerContentId": self.expected_producer_content_id,
            "expectedProducerManifestSha256": self.expected_producer_manifest_sha256,
            "subjectRef": self.subject_ref,
            "subjectResolutionDigest": self.subject_resolution_digest,
            "startInclusive": canonical_start_inclusive,
            "endInclusive": canonical_end_inclusive,
            "evidenceRowLimit": self.evidence_row_limit,
            "datasetUtcOffsetMinutes": self.dataset_utc_offset_minutes,
            "expectedCurrency": self.expected_currency,
            "minorUnitScale": self.minor_unit_scale,
            "scanCap": self.scan_cap,
        })
    }

    fn has_required_scope(&self) -> bool {
        valid_subject_ref(&self.subject_ref)
    }
}

fn load_untimed_subject_row_count(
    conn: &Connection,
    validated: &ValidatedAccountFlowArguments<'_>,
) -> Result<u64> {
    let count = conn
        .query_row(
            "SELECT COUNT(*) FROM analysis_txn_detail_idx \
              WHERE acct_key=? AND txn_ts IS NULL",
            [validated.arguments.resolved_account_key.as_str()],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| anyhow!("account_flow_query_failed"))?;
    u64::try_from(count).map_err(|_| anyhow!("account_flow_result_contract_invalid"))
}

fn require_exact_account_flow_source_schema(conn: &Connection) -> Result<()> {
    let current_catalog = conn
        .query_row("SELECT current_database()", [], |row| {
            row.get::<_, String>(0)
        })
        .map_err(|_| anyhow!("account_flow_exact_source_unavailable"))?;
    let mut statement = conn
        .prepare(
            "SELECT table_catalog, table_schema, column_name, data_type, \
                    COALESCE(collation_name, '') \
               FROM information_schema.columns \
              WHERE table_name='fc_transaction_norm' \
                AND column_name IN ('clean_amount', 'file_id', 'row_no') \
              ORDER BY table_catalog, table_schema, column_name, data_type, collation_name",
        )
        .map_err(|_| anyhow!("account_flow_exact_source_unavailable"))?;
    let rows = statement
        .query_map([], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, String>(3)?,
                row.get::<_, String>(4)?,
            ))
        })
        .map_err(|_| anyhow!("account_flow_exact_source_unavailable"))?
        .collect::<std::result::Result<Vec<_>, _>>()
        .map_err(|_| anyhow!("account_flow_exact_source_unavailable"))?;
    let expected = [
        ("clean_amount", "VARCHAR"),
        ("file_id", "VARCHAR"),
        ("row_no", "BIGINT"),
    ];
    if rows.len() != expected.len()
        || rows.iter().zip(expected).any(|(row, expected)| {
            row.0 != current_catalog
                || row.1 != "main"
                || row.2 != expected.0
                || !row.3.eq_ignore_ascii_case(expected.1)
                || !row.4.is_empty()
        })
    {
        bail!("account_flow_exact_source_unavailable");
    }
    Ok(())
}

fn load_and_aggregate_account_flow_rows(
    conn: &Connection,
    validated: &ValidatedAccountFlowArguments<'_>,
    query_hash: &str,
    snapshot_coverage: AccountFlowSnapshotCoverage,
    provenance: AccountFlowHostProvenance,
) -> Result<AnalyzeAccountFlowsHostResult> {
    let scan_limit = i64::from(validated.arguments.scan_cap) + 1;
    let mut statement = conn
        .prepare(ACCOUNT_FLOW_QUERY_SQL)
        .map_err(|_| anyhow!("account_flow_query_failed"))?;
    let dataset_utc_offset_minutes = i64::from(validated.arguments.dataset_utc_offset_minutes);
    let mut query = statement
        .query(params![
            validated.arguments.resolved_account_key.as_str(),
            validated.snapshot_local_start_inclusive.as_str(),
            validated.snapshot_local_end_inclusive.as_str(),
            scan_limit,
            dataset_utc_offset_minutes,
            validated.arguments.case_id.as_str(),
        ])
        .map_err(|_| anyhow!("account_flow_query_failed"))?;
    let mut accumulator = AccountFlowAccumulator::new(validated);
    let mut scanned_text_bytes = 0_usize;
    while let Some(row) = query
        .next()
        .map_err(|_| anyhow!("account_flow_query_failed"))?
    {
        let raw = RawAccountFlowRow {
            source_file_id: required_bounded_text(
                row.get_ref(0)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_SOURCE_FILE_ID_BYTES,
                &mut scanned_text_bytes,
            )?,
            source_row_number: row
                .get(1)
                .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
            occurred_at: required_bounded_text(
                row.get_ref(2)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_TIMESTAMP_BYTES,
                &mut scanned_text_bytes,
            )?,
            direction: optional_bounded_text(
                row.get_ref(3)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_DIRECTION_BYTES,
                &mut scanned_text_bytes,
            )?,
            amount_text: optional_bounded_text(
                row.get_ref(4)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_AMOUNT_TEXT_BYTES,
                &mut scanned_text_bytes,
            )?,
            amount_source_present: row
                .get(5)
                .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
            amount_parse_failed: row
                .get(6)
                .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
            currency: optional_bounded_text(
                row.get_ref(7)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_CURRENCY_BYTES,
                &mut scanned_text_bytes,
            )?,
            amount_type: required_bounded_text(
                row.get_ref(8)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_AMOUNT_TYPE_BYTES,
                &mut scanned_text_bytes,
            )?,
            counterparty_key: optional_bounded_text(
                row.get_ref(9)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_COUNTERPARTY_KEY_BYTES,
                &mut scanned_text_bytes,
            )?,
            counterparty_name: optional_bounded_text(
                row.get_ref(10)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_COUNTERPARTY_SEMANTIC_BYTES,
                &mut scanned_text_bytes,
            )?,
            counterparty_bank: optional_bounded_text(
                row.get_ref(11)
                    .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
                MAX_COUNTERPARTY_SEMANTIC_BYTES,
                &mut scanned_text_bytes,
            )?,
            joined_row_count: row
                .get(12)
                .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?,
        };
        accumulator.push(raw)?;
    }
    accumulator.finish(query_hash, snapshot_coverage, provenance)
}

fn aggregate_account_flow_rows(
    validated: &ValidatedAccountFlowArguments<'_>,
    query_hash: &str,
    rows: Vec<RawAccountFlowRow>,
    snapshot_coverage: AccountFlowSnapshotCoverage,
    provenance: AccountFlowHostProvenance,
) -> Result<AnalyzeAccountFlowsHostResult> {
    let mut accumulator = AccountFlowAccumulator::new(validated);
    for row in rows {
        accumulator.push(row)?;
    }
    accumulator.finish(query_hash, snapshot_coverage, provenance)
}

impl<'a, 'args> AccountFlowAccumulator<'a, 'args> {
    fn new(validated: &'a ValidatedAccountFlowArguments<'args>) -> Self {
        Self {
            validated,
            inflow_minor: 0,
            outflow_minor: 0,
            transaction_count: 0,
            evidence_rows: Vec::with_capacity(validated.arguments.evidence_row_limit as usize),
        }
    }

    fn push(&mut self, row: RawAccountFlowRow) -> Result<()> {
        if self.transaction_count >= u64::from(self.validated.arguments.scan_cap) {
            bail!("account_flow_scan_limit_exceeded");
        }
        if row.joined_row_count != 1 {
            bail!("account_flow_duplicate_join");
        }
        if row.source_file_id.is_empty()
            || row.source_file_id != row.source_file_id.trim()
            || row.source_file_id.chars().any(char::is_control)
            || row.source_row_number <= 0
            || row.source_row_number > MAX_SOURCE_ROW_NUMBER
        {
            bail!("account_flow_source_locator_invalid");
        }
        let source_row_number = u64::try_from(row.source_row_number)
            .map_err(|_| anyhow!("account_flow_source_locator_invalid"))?;
        if row.amount_type != "VARCHAR" {
            bail!("account_flow_amount_source_type_invalid");
        }
        if row.amount_parse_failed == 1 {
            bail!("account_flow_amount_parse_failed");
        }
        if row.amount_source_present == 0 || row.amount_text.is_none() {
            bail!("account_flow_amount_missing");
        }
        if row.amount_source_present != 1 || row.amount_parse_failed != 0 {
            bail!("account_flow_amount_coverage_invalid");
        }
        let currency = row
            .currency
            .as_deref()
            .filter(|value| !value.is_empty() && *value == value.trim())
            .ok_or_else(|| anyhow!("account_flow_currency_missing"))?;
        if currency != self.validated.arguments.expected_currency {
            bail!("account_flow_currency_mismatch");
        }
        let direction = match row.direction.as_deref() {
            Some("进") => AccountFlowDirection::Inflow,
            Some("出") => AccountFlowDirection::Outflow,
            _ => bail!("account_flow_direction_invalid"),
        };
        let amount_minor = parse_decimal_minor_units(
            row.amount_text.as_deref().unwrap_or_default(),
            self.validated.arguments.minor_unit_scale,
        )?;
        let amount_magnitude = amount_minor
            .checked_abs()
            .ok_or_else(|| anyhow!("account_flow_amount_overflow"))?;
        match direction {
            AccountFlowDirection::Inflow => {
                self.inflow_minor = self
                    .inflow_minor
                    .checked_add(amount_magnitude)
                    .ok_or_else(|| anyhow!("account_flow_aggregate_overflow"))?;
            }
            AccountFlowDirection::Outflow => {
                self.outflow_minor = self
                    .outflow_minor
                    .checked_add(amount_magnitude)
                    .ok_or_else(|| anyhow!("account_flow_aggregate_overflow"))?;
            }
        }
        self.transaction_count = self
            .transaction_count
            .checked_add(1)
            .ok_or_else(|| anyhow!("account_flow_count_overflow"))?;
        if self.evidence_rows.len() < self.validated.arguments.evidence_row_limit as usize {
            self.evidence_rows.push(AccountFlowHostEvidenceRow {
                subject_ref: self.validated.arguments.subject_ref.clone(),
                source_file_id: row.source_file_id,
                source_row_number,
                counterparty_key: normalize_optional_private_text(row.counterparty_key)?,
                counterparty_name: normalize_optional_private_text(row.counterparty_name)?,
                counterparty_bank: normalize_optional_private_text(row.counterparty_bank)?,
                occurred_at: row.occurred_at,
                direction,
                amount_minor: amount_magnitude.to_string(),
                currency: currency.to_string(),
                minor_unit_scale: self.validated.arguments.minor_unit_scale,
            });
        }
        Ok(())
    }

    fn finish(
        self,
        query_hash: &str,
        snapshot_coverage: AccountFlowSnapshotCoverage,
        provenance: AccountFlowHostProvenance,
    ) -> Result<AnalyzeAccountFlowsHostResult> {
        let evidence_rows_complete =
            self.transaction_count <= u64::from(self.validated.arguments.evidence_row_limit);
        let aggregate_complete = snapshot_coverage.rejected_row_count == 0
            && snapshot_coverage.duplicate_row_count == 0
            && snapshot_coverage.untimed_subject_rows == 0;
        let mut gaps = Vec::new();
        if snapshot_coverage.rejected_row_count != 0 {
            gaps.push(AccountFlowCoverageGap::RejectedSourceRows);
        }
        if snapshot_coverage.duplicate_row_count != 0 {
            gaps.push(AccountFlowCoverageGap::DuplicateSourceRows);
        }
        if snapshot_coverage.untimed_subject_rows != 0 {
            gaps.push(AccountFlowCoverageGap::UntimedSubjectRows);
        }
        if !evidence_rows_complete {
            gaps.push(AccountFlowCoverageGap::EvidenceRowLimit);
        }
        let coverage_state = if !aggregate_complete || !evidence_rows_complete {
            AccountFlowCoverageState::Partial
        } else if self.transaction_count == 0 {
            AccountFlowCoverageState::ObservedNoHitPendingHostBinding
        } else {
            AccountFlowCoverageState::Complete
        };
        let coverage = AccountFlowHostCoverage {
            state: coverage_state,
            gaps,
            normalized_snapshot_rows: snapshot_coverage.normalized_row_count,
            accepted_snapshot_rows: snapshot_coverage.accepted_row_count,
            rejected_snapshot_rows: snapshot_coverage.rejected_row_count,
            duplicate_snapshot_rows: snapshot_coverage.duplicate_row_count,
            untimed_subject_rows: snapshot_coverage.untimed_subject_rows,
            observed_matching_rows: self.transaction_count,
        };
        let net_minor = self
            .inflow_minor
            .checked_sub(self.outflow_minor)
            .ok_or_else(|| anyhow!("account_flow_aggregate_overflow"))?;
        let inflow_minor = self.inflow_minor.to_string();
        let outflow_minor = self.outflow_minor.to_string();
        let net_minor = net_minor.to_string();
        let timezone =
            format_utc_offset_minutes(self.validated.arguments.dataset_utc_offset_minutes);
        let currentness = AccountFlowCurrentness::HostRevalidationRequired;
        let semantic_projection_state =
            AccountFlowSemanticProjectionState::HostCounterpartyResolutionRequired;
        let hash_material = AccountFlowResultHashMaterial {
            subject_ref: &self.validated.arguments.subject_ref,
            start_inclusive: &self.validated.canonical_start_inclusive,
            end_inclusive: &self.validated.canonical_end_inclusive,
            timezone: &timezone,
            currency: &self.validated.arguments.expected_currency,
            minor_unit_scale: self.validated.arguments.minor_unit_scale,
            inflow_minor: &inflow_minor,
            outflow_minor: &outflow_minor,
            net_minor: &net_minor,
            transaction_count: self.transaction_count,
            aggregate_complete,
            evidence_rows_complete,
            coverage: &coverage,
            provenance: &provenance,
            currentness,
            semantic_projection_state,
            evidence_rows: &self.evidence_rows,
            query_hash,
        };
        let result_hash = hash_serialized(ACCOUNT_FLOW_RESULT_HASH_DOMAIN, &hash_material)?;
        let result = AnalyzeAccountFlowsHostResult {
            subject_ref: self.validated.arguments.subject_ref.clone(),
            start_inclusive: self.validated.canonical_start_inclusive.clone(),
            end_inclusive: self.validated.canonical_end_inclusive.clone(),
            timezone,
            currency: self.validated.arguments.expected_currency.clone(),
            minor_unit_scale: self.validated.arguments.minor_unit_scale,
            inflow_minor,
            outflow_minor,
            net_minor,
            transaction_count: self.transaction_count,
            aggregate_complete,
            evidence_rows_complete,
            coverage,
            provenance,
            currentness,
            semantic_projection_state,
            evidence_rows: self.evidence_rows,
            query_hash: query_hash.to_string(),
            result_hash,
        };
        let result_bytes = serde_json::to_vec(&result)
            .map_err(|_| anyhow!("account_flow_result_contract_invalid"))?;
        if result_bytes.len() > MAX_RESULT_BYTES {
            bail!("account_flow_result_byte_limit_exceeded");
        }
        Ok(result)
    }
}

fn required_bounded_text(
    value: ValueRef<'_>,
    field_limit: usize,
    scanned_text_bytes: &mut usize,
) -> Result<String> {
    match value {
        ValueRef::Text(value) => {
            bounded_text(value, field_limit, scanned_text_bytes).map(str::to_string)
        }
        _ => bail!("account_flow_result_contract_invalid"),
    }
}

fn optional_bounded_text(
    value: ValueRef<'_>,
    field_limit: usize,
    scanned_text_bytes: &mut usize,
) -> Result<Option<String>> {
    match value {
        ValueRef::Null => Ok(None),
        ValueRef::Text(value) => bounded_text(value, field_limit, scanned_text_bytes)
            .map(|value| Some(value.to_string())),
        _ => bail!("account_flow_result_contract_invalid"),
    }
}

fn bounded_text<'a>(
    value: &'a [u8],
    field_limit: usize,
    scanned_text_bytes: &mut usize,
) -> Result<&'a str> {
    if value.len() > field_limit {
        bail!("account_flow_result_byte_limit_exceeded");
    }
    *scanned_text_bytes = scanned_text_bytes
        .checked_add(value.len())
        .filter(|value| *value <= MAX_SCANNED_TEXT_BYTES)
        .ok_or_else(|| anyhow!("account_flow_scan_byte_limit_exceeded"))?;
    std::str::from_utf8(value).map_err(|_| anyhow!("account_flow_result_contract_invalid"))
}

fn normalize_optional_private_text(value: Option<String>) -> Result<Option<String>> {
    match value {
        None => Ok(None),
        Some(value) if value.is_empty() => Ok(None),
        Some(value) if value == value.trim() && !value.chars().any(char::is_control) => {
            Ok(Some(value))
        }
        Some(_) => bail!("account_flow_private_semantic_invalid"),
    }
}

fn parse_decimal_minor_units(value: &str, scale: u8) -> Result<i128> {
    if value.len() > MAX_AMOUNT_TEXT_BYTES {
        bail!("account_flow_amount_overflow");
    }
    if value.is_empty()
        || value != value.trim()
        || value.bytes().any(|byte| matches!(byte, b'e' | b'E'))
    {
        bail!("account_flow_amount_syntax_invalid");
    }
    let (negative, unsigned) = match value.as_bytes().first() {
        Some(b'-') => (true, &value[1..]),
        Some(b'+') => (false, &value[1..]),
        _ => (false, value),
    };
    let mut parts = unsigned.split('.');
    let integer_part = parts.next().unwrap_or_default();
    let fractional_part = parts.next();
    if integer_part.is_empty()
        || !integer_part.bytes().all(|byte| byte.is_ascii_digit())
        || parts.next().is_some()
        || fractional_part
            .is_some_and(|part| part.is_empty() || !part.bytes().all(|byte| byte.is_ascii_digit()))
    {
        bail!("account_flow_amount_syntax_invalid");
    }
    let fractional_part = fractional_part.unwrap_or_default();
    if fractional_part.len() > usize::from(scale) {
        bail!("account_flow_amount_scale_invalid");
    }
    let factor = 10_u128
        .checked_pow(u32::from(scale))
        .ok_or_else(|| anyhow!("account_flow_amount_overflow"))?;
    let integer = parse_decimal_digits(integer_part)?;
    let fractional = if fractional_part.is_empty() {
        0
    } else {
        parse_decimal_digits(fractional_part)?
            .checked_mul(
                10_u128
                    .checked_pow(u32::from(scale) - fractional_part.len() as u32)
                    .ok_or_else(|| anyhow!("account_flow_amount_overflow"))?,
            )
            .ok_or_else(|| anyhow!("account_flow_amount_overflow"))?
    };
    let magnitude = integer
        .checked_mul(factor)
        .and_then(|value| value.checked_add(fractional))
        .filter(|value| *value <= i128::MAX as u128)
        .ok_or_else(|| anyhow!("account_flow_amount_overflow"))?;
    let signed = magnitude as i128;
    Ok(if negative && signed != 0 {
        -signed
    } else {
        signed
    })
}

fn parse_decimal_digits(value: &str) -> Result<u128> {
    value.bytes().try_fold(0_u128, |accumulator, byte| {
        accumulator
            .checked_mul(10)
            .and_then(|current| current.checked_add(u128::from(byte - b'0')))
            .ok_or_else(|| anyhow!("account_flow_amount_overflow"))
    })
}

fn build_query_hash(
    duckdb_content_snapshot_digest: &str,
    validated: &ValidatedAccountFlowArguments<'_>,
) -> Result<String> {
    let safe_scope = json!({
        "contract": ACCOUNT_FLOW_QUERY_CONTRACT,
        "querySqlHash": account_flow_query_sql_hash(),
        "datasetSnapshotId": validated.arguments.dataset_snapshot_id,
        "contextEpoch": validated.arguments.context_epoch,
        "contextDigest": validated.arguments.context_digest,
        "caseBindingHash": validated.arguments.case_binding_hash,
        "expectedProducerContentId": validated.arguments.expected_producer_content_id,
        "expectedProducerManifestSha256": validated.arguments.expected_producer_manifest_sha256,
        "subjectRef": validated.arguments.subject_ref,
        "subjectResolutionDigest": validated.arguments.subject_resolution_digest,
        "startInclusive": validated.canonical_start_inclusive,
        "endInclusive": validated.canonical_end_inclusive,
        "evidenceRowLimit": validated.arguments.evidence_row_limit,
        "datasetUtcOffsetMinutes": validated.arguments.dataset_utc_offset_minutes,
        "expectedCurrency": validated.arguments.expected_currency,
        "minorUnitScale": validated.arguments.minor_unit_scale,
        "scanCap": validated.arguments.scan_cap,
    });
    let safe_scope =
        serde_json::to_vec(&safe_scope).map_err(|_| anyhow!("account_flow_hash_failed"))?;
    Ok(hash_framed(
        ACCOUNT_FLOW_QUERY_HASH_DOMAIN,
        &[duckdb_content_snapshot_digest.as_bytes(), &safe_scope],
    ))
}

fn account_flow_query_sql_hash() -> String {
    hash_framed(
        ACCOUNT_FLOW_QUERY_SQL_HASH_DOMAIN,
        &[ACCOUNT_FLOW_QUERY_SQL.as_bytes()],
    )
}

fn hash_serialized<T: Serialize>(domain: &[u8], value: &T) -> Result<String> {
    let bytes = serde_json::to_vec(value).map_err(|_| anyhow!("account_flow_hash_failed"))?;
    Ok(hash_framed(domain, &[&bytes]))
}

fn hash_framed(domain: &[u8], values: &[&[u8]]) -> String {
    let mut hasher = Sha256::new();
    hash_one(&mut hasher, domain);
    for value in values {
        hash_one(&mut hasher, value);
    }
    format!("{:x}", hasher.finalize())
}

fn hash_one(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

fn sanitize_account_flow_error(error: anyhow::Error) -> anyhow::Error {
    let error_chain = format!("{error:#}");
    let code = [
        "account_flow_request_contract_invalid",
        "account_flow_snapshot_contract_invalid",
        "account_flow_snapshot_authority_binding_mismatch",
        "account_flow_exact_source_unavailable",
        "account_flow_query_failed",
        "account_flow_result_contract_invalid",
        "account_flow_scan_limit_exceeded",
        "account_flow_scan_byte_limit_exceeded",
        "account_flow_result_byte_limit_exceeded",
        "account_flow_duplicate_join",
        "account_flow_source_locator_invalid",
        "account_flow_private_semantic_invalid",
        "account_flow_amount_source_type_invalid",
        "account_flow_amount_parse_failed",
        "account_flow_amount_missing",
        "account_flow_amount_coverage_invalid",
        "account_flow_currency_missing",
        "account_flow_currency_mismatch",
        "account_flow_direction_invalid",
        "account_flow_amount_overflow",
        "account_flow_amount_syntax_invalid",
        "account_flow_amount_scale_invalid",
        "account_flow_aggregate_overflow",
        "account_flow_count_overflow",
        "account_flow_hash_failed",
    ]
    .into_iter()
    .find(|code| error_chain.contains(code))
    .unwrap_or_else(|| {
        if error_chain.contains("stats_query_connection_not_readonly") {
            "account_flow_snapshot_connection_invalid"
        } else if error_chain.contains("stats_query_external_access_not_disabled") {
            "account_flow_external_access_not_disabled"
        } else if error_chain.contains("stats_query_case_binding")
            || error_chain.contains("stats_query_source")
            || error_chain.contains("stats_query_materialization")
            || error_chain.contains("stats_query_result_")
            || error_chain.contains("stats_query_case_isolation")
            || error_chain.contains("stats_query_base_table")
            || error_chain.contains("stats_query_table_identity")
            || error_chain.contains("stats_query_dataset_snapshot_manifest")
            || error_chain.contains("stats_query_producer_manifest")
        {
            "account_flow_snapshot_verification_failed"
        } else if error_chain.contains("stats_query_range") {
            "account_flow_range_verification_failed"
        } else {
            "account_flow_internal_failed"
        }
    });
    anyhow!(code)
}

fn valid_subject_ref(value: &str) -> bool {
    value.strip_prefix("cer1_").is_some_and(|suffix| {
        suffix.len() == 64 && suffix.bytes().all(|byte| (b'a'..=b'p').contains(&byte))
    })
}

fn valid_prefixed_sha256(value: &str, prefix: &str) -> bool {
    value.strip_prefix(prefix).is_some_and(crate::is_sha256_hex)
}

fn valid_currency(value: &str) -> bool {
    value.len() == 3 && value.bytes().all(|byte| byte.is_ascii_uppercase())
}

fn format_utc_offset_minutes(offset_minutes: i16) -> String {
    if offset_minutes == 0 {
        return "Z".to_string();
    }
    let absolute = i32::from(offset_minutes).abs();
    format!(
        "{}{:02}:{:02}",
        if offset_minutes < 0 { '-' } else { '+' },
        absolute / 60,
        absolute % 60
    )
}

fn parse_rfc3339_utc(value: &str) -> Result<DateTime<Utc>> {
    if value.is_empty()
        || value.len() > MAX_TIMESTAMP_BYTES
        || value != value.trim()
        || value.chars().any(char::is_control)
    {
        bail!("account_flow_request_contract_invalid");
    }
    let timestamp = DateTime::parse_from_rfc3339(value)
        .map(|timestamp| timestamp.with_timezone(&Utc))
        .map_err(|_| anyhow!("account_flow_request_contract_invalid"))?;
    if timestamp.timestamp_subsec_nanos() % 1_000 != 0 {
        bail!("account_flow_request_contract_invalid");
    }
    Ok(timestamp)
}

fn canonical_rfc3339_utc(timestamp: DateTime<Utc>) -> String {
    timestamp.to_rfc3339_opts(SecondsFormat::Micros, true)
}

fn canonical_duckdb_timestamp(timestamp: DateTime<Utc>) -> String {
    timestamp.format("%Y-%m-%d %H:%M:%S%.6f").to_string()
}

#[cfg(test)]
mod tests {
    use std::fs;
    use std::path::{Path, PathBuf};
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::time::{SystemTime, UNIX_EPOCH};

    use super::*;
    use crate::txn_daily_store::MaterializeArgs;

    const PRIVATE_ACCOUNT_KEY: &str = "private-account-key-a";
    static TEMP_DB_SEQUENCE: AtomicU64 = AtomicU64::new(0);

    #[derive(Clone)]
    struct SeedRow {
        id: i64,
        source_row_number: i64,
        timestamp: &'static str,
        direction: &'static str,
        amount: Option<&'static str>,
        currency: Option<&'static str>,
        amount_source_present: i64,
        amount_parse_failed: i64,
        joined_row_count: i64,
    }

    struct TempDb {
        path: PathBuf,
        producer_content_id: String,
        producer_manifest_sha256: String,
    }

    impl TempDb {
        fn materialized(case_id: &str, rows: &[SeedRow]) -> Self {
            Self::materialized_with_account_key(case_id, PRIVATE_ACCOUNT_KEY, rows)
        }

        fn materialized_with_account_key(
            case_id: &str,
            account_key: &str,
            rows: &[SeedRow],
        ) -> Self {
            Self::materialized_with_source_identifiers(case_id, Some(account_key), None, rows)
        }

        fn materialized_with_source_identifiers(
            case_id: &str,
            account_no: Option<&str>,
            card_no: Option<&str>,
            rows: &[SeedRow],
        ) -> Self {
            let unique = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock after UNIX epoch")
                .as_nanos();
            let sequence = TEMP_DB_SEQUENCE.fetch_add(1, Ordering::Relaxed);
            let path = std::env::temp_dir().join(format!(
                "analytix-account-flow-{}-{unique}-{sequence}.duckdb",
                std::process::id(),
            ));
            let conn = Connection::open(&path).expect("open account-flow fixture");
            crate::configure_connection(&conn).expect("configure fixture connection");
            conn.execute_batch(
                "CREATE TABLE analysis_revision_state(revision_key TEXT, revision BIGINT); \
                 INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 1); \
                 CREATE TABLE fc_transaction_norm( \
                   id BIGINT, case_id TEXT, txn_ts TIMESTAMP, acct_no TEXT, card_no TEXT, \
                   counterparty_acct TEXT, dc_flag TEXT, clean_amount TEXT, \
                   currency TEXT, file_id TEXT, row_no BIGINT, clean_invalid INTEGER, \
                   clean_failed INTEGER, clean_reversal INTEGER \
                 ); \
                 CREATE TABLE import_file_log( \
                   file_id TEXT, case_id TEXT, kind TEXT, sha256 TEXT, \
                   rows_imported_norm BIGINT, status TEXT, cleaned_status TEXT \
                 );",
            )
            .expect("create account-flow fixture schema");
            for row in rows {
                conn.execute(
                    "INSERT INTO fc_transaction_norm VALUES ( \
                       ?, ?, CAST(? AS TIMESTAMP), ?, ?, 'counterparty-a', ?, ?, ?, \
                       'file-1', ?, 0, 0, 0 \
                     )",
                    params![
                        row.id,
                        case_id,
                        row.timestamp,
                        account_no,
                        card_no,
                        row.direction,
                        row.amount,
                        row.currency,
                        row.source_row_number,
                    ],
                )
                .expect("insert account-flow fixture row");
            }
            conn.execute(
                "INSERT INTO import_file_log VALUES ( \
                   'file-1', ?, 'fc_transaction', ?, ?, '已完成', 'done' \
                 )",
                params![case_id, "a".repeat(64), rows.len() as i64],
            )
            .expect("insert account-flow source manifest");
            let source_max_txn_ts = rows
                .iter()
                .map(|row| row.timestamp)
                .max()
                .unwrap_or_default()
                .to_string();
            let source_max_id = rows.iter().map(|row| row.id).max().unwrap_or(0);
            drop(conn);
            let materialize_args = MaterializeArgs {
                case_id: case_id.to_string(),
                db_path: path.clone(),
                source_revision: 1,
                source_row_count: rows.len() as i64,
                source_max_txn_ts,
                source_max_id,
            };
            let materialized = materialize_args
                .materialize_with_raw_artifact_manifest_sha256(&"1".repeat(64))
                .expect("materialize account-flow fixture");
            Self {
                path,
                producer_content_id: materialized.producer_content_id,
                producer_manifest_sha256: format!(
                    "{:x}",
                    Sha256::digest(&materialized.producer_content_manifest_bytes)
                ),
            }
        }

        fn readonly_connection(&self) -> Connection {
            let conn = crate::open_readonly_connection(&self.path)
                .expect("open read-only account-flow fixture");
            crate::configure_connection(&conn).expect("configure read-only fixture");
            conn
        }

        fn path(&self) -> &Path {
            &self.path
        }

        fn arguments(&self) -> AnalyzeAccountFlowsHostArguments {
            let mut arguments = arguments();
            arguments.expected_producer_content_id = self.producer_content_id.clone();
            arguments.expected_producer_manifest_sha256 = self.producer_manifest_sha256.clone();
            arguments
        }
    }

    impl Drop for TempDb {
        fn drop(&mut self) {
            let _ = fs::remove_file(&self.path);
            let mut wal = self.path.as_os_str().to_os_string();
            wal.push(".wal");
            let _ = fs::remove_file(PathBuf::from(wal));
        }
    }

    fn subject_ref() -> String {
        format!("cer1_{}", "a".repeat(64))
    }

    fn arguments() -> AnalyzeAccountFlowsHostArguments {
        AnalyzeAccountFlowsHostArguments {
            case_id: "case-a".to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 7,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: format!("fpc1_{}", "4".repeat(64)),
            expected_producer_manifest_sha256: "5".repeat(64),
            subject_ref: subject_ref(),
            resolved_account_key: PRIVATE_ACCOUNT_KEY.to_string(),
            subject_resolution_digest: "6".repeat(64),
            start_inclusive: "2026-01-01T00:00:00Z".to_string(),
            end_inclusive: "2026-01-01T00:02:00Z".to_string(),
            evidence_row_limit: 10,
            dataset_utc_offset_minutes: 0,
            expected_currency: "CNY".to_string(),
            minor_unit_scale: 2,
            scan_cap: 100,
        }
    }

    fn arguments_json() -> Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "contextEpoch": 7,
            "contextDigest": "2".repeat(64),
            "caseBindingHash": "3".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "4".repeat(64)),
            "expectedProducerManifestSha256": "5".repeat(64),
            "subjectRef": subject_ref(),
            "resolvedAccountKey": PRIVATE_ACCOUNT_KEY,
            "subjectResolutionDigest": "6".repeat(64),
            "startInclusive": "2026-01-01T00:00:00Z",
            "endInclusive": "2026-01-01T00:02:00Z",
            "evidenceRowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
            "scanCap": 100,
        })
    }

    fn seed_row(
        id: i64,
        timestamp: &'static str,
        direction: &'static str,
        amount: &'static str,
        currency: &'static str,
    ) -> SeedRow {
        SeedRow {
            id,
            source_row_number: id + 1_000,
            timestamp,
            direction,
            amount: Some(amount),
            currency: Some(currency),
            amount_source_present: 1,
            amount_parse_failed: 0,
            joined_row_count: 1,
        }
    }

    fn raw_row(row: &SeedRow) -> RawAccountFlowRow {
        RawAccountFlowRow {
            source_file_id: "file-1".to_string(),
            source_row_number: row.source_row_number,
            occurred_at: format!("{}Z", row.timestamp.replace(' ', "T")),
            direction: Some(row.direction.to_string()),
            amount_text: row.amount.map(str::to_string),
            amount_source_present: row.amount_source_present,
            amount_parse_failed: row.amount_parse_failed,
            currency: row.currency.map(str::to_string),
            amount_type: "VARCHAR".to_string(),
            counterparty_key: Some("counterparty-private-key-a".to_string()),
            counterparty_name: Some("counterparty-private-name-a".to_string()),
            counterparty_bank: Some("counterparty-bank-a".to_string()),
            joined_row_count: row.joined_row_count,
        }
    }

    fn aggregate(
        rows: &[SeedRow],
        mutate: impl FnOnce(&mut AnalyzeAccountFlowsHostArguments),
    ) -> Result<AnalyzeAccountFlowsHostResult> {
        let mut args = arguments();
        mutate(&mut args);
        let validated = ValidatedAccountFlowArguments::new(&args)?;
        let dataset_snapshot_digest = "b".repeat(64);
        let query_hash = build_query_hash(&dataset_snapshot_digest, &validated)?;
        aggregate_account_flow_rows(
            &validated,
            &query_hash,
            rows.iter().map(raw_row).collect(),
            AccountFlowSnapshotCoverage {
                normalized_row_count: rows.len() as u64,
                accepted_row_count: rows.len() as u64,
                rejected_row_count: 0,
                duplicate_row_count: 0,
                untimed_subject_rows: 0,
            },
            test_provenance(&dataset_snapshot_digest),
        )
    }

    fn test_provenance(dataset_snapshot_digest: &str) -> AccountFlowHostProvenance {
        AccountFlowHostProvenance {
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 7,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: format!("fpc1_{}", "4".repeat(64)),
            expected_producer_manifest_sha256: "5".repeat(64),
            subject_resolution_digest: "6".repeat(64),
            duckdb_content_snapshot_digest: dataset_snapshot_digest.to_string(),
            duckdb_snapshot_manifest_sha256: "7".repeat(64),
            materialization_identity: format!(
                "{}{}",
                crate::MATERIALIZATION_IDENTITY_PREFIX,
                "c".repeat(64)
            ),
            source_signature: "c".repeat(64),
            result_signature: "d".repeat(64),
            producer_content_id: format!("fpc1_{}", "e".repeat(64)),
            producer_manifest_sha256: "f".repeat(64),
            query_contract: ACCOUNT_FLOW_QUERY_CONTRACT.to_string(),
            query_sql_hash: account_flow_query_sql_hash(),
        }
    }

    fn error_code(error: anyhow::Error) -> String {
        format!("{error:#}")
    }

    fn recompute_result_hash(result: &AnalyzeAccountFlowsHostResult) -> String {
        hash_serialized(
            ACCOUNT_FLOW_RESULT_HASH_DOMAIN,
            &AccountFlowResultHashMaterial {
                subject_ref: &result.subject_ref,
                start_inclusive: &result.start_inclusive,
                end_inclusive: &result.end_inclusive,
                timezone: &result.timezone,
                currency: &result.currency,
                minor_unit_scale: result.minor_unit_scale,
                inflow_minor: &result.inflow_minor,
                outflow_minor: &result.outflow_minor,
                net_minor: &result.net_minor,
                transaction_count: result.transaction_count,
                aggregate_complete: result.aggregate_complete,
                evidence_rows_complete: result.evidence_rows_complete,
                coverage: &result.coverage,
                provenance: &result.provenance,
                currentness: result.currentness,
                semantic_projection_state: result.semantic_projection_state,
                evidence_rows: &result.evidence_rows,
                query_hash: &result.query_hash,
            },
        )
        .expect("recompute account-flow result hash")
    }

    #[test]
    fn strict_arguments_reject_unknown_sql_and_path_fields_without_echoing_private_values() {
        validate_analyze_account_flows_host_arguments(
            &parse_analyze_account_flows_host_arguments(&arguments_json())
                .expect("parse exact account-flow arguments"),
        )
        .expect("validate exact account-flow arguments");

        for (field, value) in [
            ("sql", Value::String("SELECT private-value".to_string())),
            (
                "dbPath",
                Value::String("/private/case/database.duckdb".to_string()),
            ),
            (
                "datasetTimezone",
                Value::String("America/New_York".to_string()),
            ),
            ("unknown", Value::String(PRIVATE_ACCOUNT_KEY.to_string())),
        ] {
            let mut payload = arguments_json();
            payload
                .as_object_mut()
                .expect("arguments object")
                .insert(field.to_string(), value);
            let error = error_code(
                parse_analyze_account_flows_host_arguments(&payload)
                    .expect_err("unknown fields must fail closed"),
            );
            assert_eq!(error, "account_flow_request_contract_invalid");
            assert!(!error.contains(PRIVATE_ACCOUNT_KEY));
        }
        let debug = format!("{:?}", arguments());
        assert!(!debug.contains(PRIVATE_ACCOUNT_KEY));
        assert!(debug.contains("[REDACTED]"));

        for offset in [-841, 841] {
            let mut out_of_range = arguments();
            out_of_range.dataset_utc_offset_minutes = offset;
            assert_eq!(
                error_code(
                    validate_analyze_account_flows_host_arguments(&out_of_range)
                        .expect_err("out-of-range fixed offset must fail before DuckDB")
                ),
                "account_flow_request_contract_invalid"
            );
        }
    }

    #[test]
    fn decimal_text_to_minor_units_is_exact_and_rejects_scale_exponent_syntax_and_overflow() {
        assert_eq!(parse_decimal_minor_units("0.10", 2).expect("0.10"), 10);
        assert_eq!(parse_decimal_minor_units("0.20", 2).expect("0.20"), 20);
        assert_eq!(parse_decimal_minor_units("-1.23", 2).expect("-1.23"), -123);
        assert_eq!(parse_decimal_minor_units("1.2", 2).expect("1.2"), 120);

        for value in [".10", "1.", "1..0", " 1.00", "1.00 ", "1e2", "NaN"] {
            let error = error_code(
                parse_decimal_minor_units(value, 2).expect_err("invalid syntax must fail"),
            );
            assert_eq!(error, "account_flow_amount_syntax_invalid");
        }
        assert_eq!(
            error_code(
                parse_decimal_minor_units("0.001", 2).expect_err("excess scale must fail closed")
            ),
            "account_flow_amount_scale_invalid"
        );
        assert_eq!(
            error_code(
                parse_decimal_minor_units("1701411834604692317316873037158841058.00", 2,)
                    .expect_err("minor-unit overflow must fail closed")
            ),
            "account_flow_amount_overflow"
        );
    }

    #[test]
    fn rfc3339_range_is_canonical_at_microsecond_precision_and_rejects_nanoseconds() {
        let mut utc = arguments();
        utc.start_inclusive = "2026-01-01T00:00:00.123456Z".to_string();
        utc.end_inclusive = "2026-01-01T00:00:00.123457Z".to_string();
        let utc_validated =
            ValidatedAccountFlowArguments::new(&utc).expect("validate UTC microseconds");
        assert_eq!(
            utc_validated.canonical_start_inclusive,
            "2026-01-01T00:00:00.123456Z"
        );
        assert_eq!(
            utc_validated.canonical_end_inclusive,
            "2026-01-01T00:00:00.123457Z"
        );

        let mut offset = utc.clone();
        offset.start_inclusive = "2026-01-01T08:00:00.123456+08:00".to_string();
        offset.end_inclusive = "2026-01-01T08:00:00.123457+08:00".to_string();
        let offset_validated =
            ValidatedAccountFlowArguments::new(&offset).expect("validate offset microseconds");
        assert_eq!(
            utc.canonical_scope(),
            offset.canonical_scope(),
            "equivalent instants must bind the same verified session scope"
        );
        let context_digest = "d".repeat(64);
        assert_eq!(
            build_query_hash(&context_digest, &utc_validated).expect("hash UTC microsecond range"),
            build_query_hash(&context_digest, &offset_validated)
                .expect("hash offset microsecond range")
        );

        let mut nanoseconds = utc;
        nanoseconds.start_inclusive = "2026-01-01T00:00:00.123456001Z".to_string();
        assert_eq!(
            error_code(
                validate_analyze_account_flows_host_arguments(&nanoseconds)
                    .expect_err("sub-microsecond precision must fail closed")
            ),
            "account_flow_request_contract_invalid"
        );
    }

    #[test]
    fn exact_aggregation_evidence_bound_and_scan_cap_are_independent() {
        let rows = [
            seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY"),
            seed_row(2, "2026-01-01 00:01:00", "进", "0.20", "CNY"),
            seed_row(3, "2026-01-01 00:02:00", "出", "0.40", "CNY"),
        ];
        let result = aggregate(&rows, |args| args.evidence_row_limit = 1)
            .expect("bounded evidence still has exact aggregate");
        assert_eq!(result.inflow_minor, "30");
        assert_eq!(result.outflow_minor, "40");
        assert_eq!(result.net_minor, "-10");
        assert_eq!(result.transaction_count, 3);
        assert!(result.aggregate_complete);
        assert!(!result.evidence_rows_complete);
        assert_eq!(result.coverage.state, AccountFlowCoverageState::Partial);
        assert_eq!(
            result.coverage.gaps,
            vec![AccountFlowCoverageGap::EvidenceRowLimit]
        );
        assert_eq!(result.evidence_rows.len(), 1);
        assert_eq!(result.evidence_rows[0].amount_minor, "10");

        let error = error_code(
            aggregate(&rows, |args| args.scan_cap = 2)
                .expect_err("scan cap must fail before partial aggregation"),
        );
        assert_eq!(error, "account_flow_scan_limit_exceeded");
    }

    #[test]
    fn rejected_or_untimed_coverage_is_partial_and_zero_remains_pending_host_binding() {
        let args = arguments();
        let validated =
            ValidatedAccountFlowArguments::new(&args).expect("validate coverage arguments");
        let dataset_snapshot_digest = "e".repeat(64);
        let query_hash =
            build_query_hash(&dataset_snapshot_digest, &validated).expect("hash coverage query");

        let rejected = aggregate_account_flow_rows(
            &validated,
            &query_hash,
            Vec::new(),
            AccountFlowSnapshotCoverage {
                normalized_row_count: 1,
                accepted_row_count: 0,
                rejected_row_count: 1,
                duplicate_row_count: 0,
                untimed_subject_rows: 0,
            },
            test_provenance(&dataset_snapshot_digest),
        )
        .expect("rejected coverage returns an explicit partial result");
        assert!(!rejected.aggregate_complete);
        assert_eq!(rejected.coverage.state, AccountFlowCoverageState::Partial);
        assert_eq!(
            rejected.coverage.gaps,
            vec![AccountFlowCoverageGap::RejectedSourceRows]
        );

        let untimed = aggregate_account_flow_rows(
            &validated,
            &query_hash,
            Vec::new(),
            AccountFlowSnapshotCoverage {
                normalized_row_count: 1,
                accepted_row_count: 1,
                rejected_row_count: 0,
                duplicate_row_count: 0,
                untimed_subject_rows: 1,
            },
            test_provenance(&dataset_snapshot_digest),
        )
        .expect("untimed coverage returns an explicit partial result");
        assert!(!untimed.aggregate_complete);
        assert_eq!(untimed.coverage.state, AccountFlowCoverageState::Partial);
        assert_eq!(
            untimed.coverage.gaps,
            vec![AccountFlowCoverageGap::UntimedSubjectRows]
        );

        let duplicate = aggregate_account_flow_rows(
            &validated,
            &query_hash,
            Vec::new(),
            AccountFlowSnapshotCoverage {
                normalized_row_count: 1,
                accepted_row_count: 0,
                rejected_row_count: 0,
                duplicate_row_count: 1,
                untimed_subject_rows: 0,
            },
            test_provenance(&dataset_snapshot_digest),
        )
        .expect("duplicate coverage returns an explicit partial result");
        assert!(!duplicate.aggregate_complete);
        assert_eq!(duplicate.coverage.state, AccountFlowCoverageState::Partial);
        assert_eq!(
            duplicate.coverage.gaps,
            vec![AccountFlowCoverageGap::DuplicateSourceRows]
        );

        let no_hit = aggregate_account_flow_rows(
            &validated,
            &query_hash,
            Vec::new(),
            AccountFlowSnapshotCoverage {
                normalized_row_count: 0,
                accepted_row_count: 0,
                rejected_row_count: 0,
                duplicate_row_count: 0,
                untimed_subject_rows: 0,
            },
            test_provenance(&dataset_snapshot_digest),
        )
        .expect("complete empty scope remains pending host resolution/currentness binding");
        assert!(no_hit.aggregate_complete);
        assert!(no_hit.evidence_rows_complete);
        assert_eq!(
            no_hit.coverage.state,
            AccountFlowCoverageState::ObservedNoHitPendingHostBinding
        );
        assert!(no_hit.coverage.gaps.is_empty());
    }

    #[test]
    fn row_contract_failures_are_closed_and_private_value_free() {
        let base = seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY");
        let cases = [
            (
                SeedRow {
                    source_row_number: 0,
                    ..base.clone()
                },
                "account_flow_source_locator_invalid",
            ),
            (
                SeedRow {
                    source_row_number: MAX_SOURCE_ROW_NUMBER + 1,
                    ..base.clone()
                },
                "account_flow_source_locator_invalid",
            ),
            (
                SeedRow {
                    direction: "unknown",
                    ..base.clone()
                },
                "account_flow_direction_invalid",
            ),
            (
                SeedRow {
                    currency: None,
                    ..base.clone()
                },
                "account_flow_currency_missing",
            ),
            (
                SeedRow {
                    amount: None,
                    amount_source_present: 0,
                    ..base.clone()
                },
                "account_flow_amount_missing",
            ),
            (
                SeedRow {
                    amount: Some("invalid"),
                    amount_parse_failed: 1,
                    ..base.clone()
                },
                "account_flow_amount_parse_failed",
            ),
            (
                SeedRow {
                    joined_row_count: 2,
                    ..base.clone()
                },
                "account_flow_duplicate_join",
            ),
        ];
        for (row, expected) in cases {
            let error =
                error_code(aggregate(&[row], |_| {}).expect_err("invalid row must fail closed"));
            assert_eq!(error, expected);
            assert!(!error.contains(PRIVATE_ACCOUNT_KEY));
        }

        let mixed = [
            base,
            seed_row(2, "2026-01-01 00:01:00", "进", "0.20", "USD"),
        ];
        assert_eq!(
            error_code(aggregate(&mixed, |_| {}).expect_err("mixed currency must fail closed")),
            "account_flow_currency_mismatch"
        );
    }

    #[test]
    fn query_hash_uses_resolution_digest_and_host_frame_redacts_debug_output() {
        let args_a = arguments();
        let mut args_b = arguments();
        args_b.resolved_account_key = "different-private-account-key".to_string();
        args_b.subject_resolution_digest = "8".repeat(64);
        assert!(!args_a
            .canonical_scope()
            .to_string()
            .contains(PRIVATE_ACCOUNT_KEY));
        let validated_a = ValidatedAccountFlowArguments::new(&args_a).expect("validate account A");
        let validated_b = ValidatedAccountFlowArguments::new(&args_b).expect("validate account B");
        let context_digest = "c".repeat(64);
        let query_hash_a =
            build_query_hash(&context_digest, &validated_a).expect("hash account A query");
        let query_hash_b =
            build_query_hash(&context_digest, &validated_b).expect("hash account B query");
        assert_ne!(query_hash_a, query_hash_b);
        let mut args_offset = arguments();
        args_offset.dataset_utc_offset_minutes = 480;
        let validated_offset =
            ValidatedAccountFlowArguments::new(&args_offset).expect("validate offset scope");
        let query_hash_offset =
            build_query_hash(&context_digest, &validated_offset).expect("hash offset-bound query");
        assert_ne!(query_hash_a, query_hash_offset);

        let result = aggregate(
            &[
                seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY"),
                seed_row(2, "2026-01-01 00:01:00", "出", "0.20", "CNY"),
            ],
            |_| {},
        )
        .expect("aggregate safe rows");
        let serialized = serde_json::to_string(&result).expect("serialize safe result");
        assert!(!serialized.contains(PRIVATE_ACCOUNT_KEY));
        let debug = format!("{result:?}");
        assert!(!debug.contains("counterparty-private-key-a"));
        assert!(!debug.contains("counterparty-private-name-a"));
        assert!(!debug.contains("file-1"));
        assert_eq!(result.query_hash.len(), 64);
        assert_eq!(result.result_hash.len(), 64);
        assert_eq!(recompute_result_hash(&result), result.result_hash);
        assert_eq!(
            result.currentness,
            AccountFlowCurrentness::HostRevalidationRequired
        );
        assert_eq!(
            result.semantic_projection_state,
            AccountFlowSemanticProjectionState::HostCounterpartyResolutionRequired
        );
        assert_eq!(result.evidence_rows[0].source_file_id, "file-1");
        assert_eq!(result.evidence_rows[0].source_row_number, 1_001);

        let mut tampered_timezone = result.clone();
        tampered_timezone.timezone = "+00:00".to_string();
        assert_ne!(
            recompute_result_hash(&tampered_timezone),
            result.result_hash
        );
        let mut tampered_count = result.clone();
        tampered_count.transaction_count += 1;
        assert_ne!(recompute_result_hash(&tampered_count), result.result_hash);
        let mut tampered_amount = result.clone();
        tampered_amount.evidence_rows[0].amount_minor = "11".to_string();
        assert_ne!(recompute_result_hash(&tampered_amount), result.result_hash);
        let mut reordered_rows = result.clone();
        reordered_rows.evidence_rows.reverse();
        assert_ne!(recompute_result_hash(&reordered_rows), result.result_hash);
    }

    #[test]
    fn private_source_locator_is_query_scope_independent_and_never_fakes_a_ledger_id() {
        let row = seed_row(7, "2026-01-01 00:00:00", "进", "1.00", "CNY");
        let first = aggregate(std::slice::from_ref(&row), |args| {
            args.evidence_row_limit = 1;
        })
        .expect("aggregate first query scope");
        let second = aggregate(std::slice::from_ref(&row), |args| {
            args.end_inclusive = "2026-01-02T00:00:00Z".to_string();
            args.evidence_row_limit = 10;
        })
        .expect("aggregate second query scope");

        assert_ne!(first.query_hash, second.query_hash);
        assert_eq!(
            first.evidence_rows[0].source_file_id,
            second.evidence_rows[0].source_file_id
        );
        assert_eq!(
            first.evidence_rows[0].source_row_number,
            second.evidence_rows[0].source_row_number
        );
        let private = serde_json::to_string(&first).expect("serialize private host result");
        assert!(!private.contains("fer1_"));
        assert!(!private.contains("srow1_"));
    }

    #[test]
    fn read_only_query_includes_microsecond_endpoints_and_replays_deterministically() {
        let db = TempDb::materialized(
            "case-a",
            &[
                seed_row(1, "2026-01-01 00:00:00.123455", "进", "9.99", "CNY"),
                seed_row(2, "2026-01-01 00:00:00.123456", "进", "0.10", "CNY"),
                seed_row(3, "2026-01-01 00:00:00.123457", "进", "0.20", "CNY"),
                seed_row(4, "2026-01-01 00:00:00.123458", "出", "9.99", "CNY"),
            ],
        );
        let conn = db.readonly_connection();
        crate::txn_daily_store::verify_txn_daily_snapshot_binding(&conn, "case-a")
            .expect("verify restricted read-only DuckDB content binding");
        let mut args = db.arguments();
        args.start_inclusive = "2026-01-01T00:00:00.123456Z".to_string();
        args.end_inclusive = "2026-01-01T00:00:00.123457Z".to_string();

        let first = analyze_account_flows(&conn, &args).expect("first exact account-flow query");
        let replay = analyze_account_flows(&conn, &args).expect("deterministic replay");

        assert_eq!(first, replay);
        let external_access = conn
            .query_row(
                "SELECT value FROM duckdb_settings() \
                  WHERE name='enable_external_access'",
                [],
                |row| row.get::<_, String>(0),
            )
            .expect("read external-access setting");
        assert_eq!(external_access.to_ascii_lowercase(), "false");
        assert_eq!(first.inflow_minor, "30");
        assert_eq!(first.outflow_minor, "0");
        assert_eq!(first.net_minor, "30");
        assert_eq!(first.transaction_count, 2);
        assert_eq!(first.timezone, "Z");
        assert_eq!(first.evidence_rows.len(), 2);
        assert_eq!(
            first
                .evidence_rows
                .iter()
                .map(|row| row.source_row_number)
                .collect::<Vec<_>>(),
            vec![1_002, 1_003]
        );
        assert_eq!(
            first.evidence_rows[0].occurred_at,
            "2026-01-01T00:00:00.123456Z"
        );
        assert_eq!(
            first.evidence_rows[1].occurred_at,
            "2026-01-01T00:00:00.123457Z"
        );
    }

    #[test]
    fn duckdb_oracle_matches_every_receipt_eligible_account_flow_field_and_exposes_double_divergence(
    ) {
        let db = TempDb::materialized(
            "case-a",
            &[
                seed_row(1, "2026-01-01 00:00:00.000000", "进", "999.99", "CNY"),
                seed_row(
                    2,
                    "2026-01-01 00:00:00.000001",
                    "进",
                    "90071992547409.91",
                    "CNY",
                ),
                seed_row(3, "2026-01-01 00:00:00.000002", "进", "0.02", "CNY"),
                seed_row(4, "2026-01-01 00:00:00.000003", "出", "0.01", "CNY"),
                seed_row(5, "2026-01-01 00:00:00.000004", "出", "999.99", "CNY"),
            ],
        );
        let conn = db.readonly_connection();
        let mut args = db.arguments();
        args.start_inclusive = "2026-01-01T00:00:00.000001Z".to_string();
        args.end_inclusive = "2026-01-01T00:00:00.000003Z".to_string();
        args.evidence_row_limit = 3;

        let result = analyze_account_flows(&conn, &args)
            .expect("production account-flow seam returns exact host result");
        let oracle = conn
            .query_row(
                "SELECT \
                   CAST(CAST(SUM(CASE WHEN dc_flag='进' THEN CAST(clean_amount AS DECIMAL(30,2)) ELSE CAST(0 AS DECIMAL(30,2)) END) * 100 AS HUGEINT) AS VARCHAR), \
                   CAST(CAST(SUM(CASE WHEN dc_flag='出' THEN CAST(clean_amount AS DECIMAL(30,2)) ELSE CAST(0 AS DECIMAL(30,2)) END) * 100 AS HUGEINT) AS VARCHAR), \
                   CAST(COUNT(*) AS UBIGINT), MIN(currency), COUNT(DISTINCT currency), \
                   strftime(MIN(txn_ts), '%Y-%m-%dT%H:%M:%S.%fZ'), \
                   strftime(MAX(txn_ts), '%Y-%m-%dT%H:%M:%S.%fZ'), \
                   CAST(COUNT(*) FILTER (WHERE clean_amount IS NULL) AS UBIGINT), \
                   CAST(COUNT(*) FILTER (WHERE clean_amount IS NOT NULL AND TRY_CAST(clean_amount AS DECIMAL(30,2)) IS NULL) AS UBIGINT) \
                 FROM fc_transaction_norm \
                WHERE case_id=? AND acct_no=? \
                  AND txn_ts >= CAST(? AS TIMESTAMP) AND txn_ts <= CAST(? AS TIMESTAMP)",
                params![
                    args.case_id.as_str(),
                    args.resolved_account_key.as_str(),
                    "2026-01-01 00:00:00.000001",
                    "2026-01-01 00:00:00.000003",
                ],
                |row| {
                    Ok((
                        row.get::<_, String>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, u64>(2)?,
                        row.get::<_, String>(3)?,
                        row.get::<_, i64>(4)?,
                        row.get::<_, String>(5)?,
                        row.get::<_, String>(6)?,
                        row.get::<_, u64>(7)?,
                        row.get::<_, u64>(8)?,
                    ))
                },
            )
            .expect("independent exact DuckDB oracle query");
        let oracle_net = oracle
            .0
            .parse::<i128>()
            .expect("oracle inflow minor")
            .checked_sub(oracle.1.parse::<i128>().expect("oracle outflow minor"))
            .expect("oracle net minor")
            .to_string();

        assert_eq!(result.inflow_minor, oracle.0);
        assert_eq!(result.outflow_minor, oracle.1);
        assert_eq!(result.net_minor, oracle_net);
        assert_eq!(result.transaction_count, oracle.2);
        assert_eq!(result.currency, oracle.3);
        assert_eq!(oracle.4, 1, "oracle fixture must have one exact currency");
        assert_eq!(result.minor_unit_scale, 2);
        assert_eq!(result.start_inclusive, args.start_inclusive);
        assert_eq!(result.end_inclusive, args.end_inclusive);
        assert_eq!(oracle.5, args.start_inclusive);
        assert_eq!(oracle.6, args.end_inclusive);
        assert_eq!(
            oracle.7, 0,
            "receipt-eligible scope cannot contain null amounts"
        );
        assert_eq!(
            oracle.8, 0,
            "receipt-eligible scope cannot contain invalid amounts"
        );
        assert!(result.aggregate_complete);
        assert!(result.evidence_rows_complete);
        assert_eq!(result.coverage.state, AccountFlowCoverageState::Complete);
        assert!(result.coverage.gaps.is_empty());
        assert_eq!(result.coverage.normalized_snapshot_rows, 5);
        assert_eq!(result.coverage.accepted_snapshot_rows, 5);
        assert_eq!(result.coverage.rejected_snapshot_rows, 0);
        assert_eq!(result.coverage.duplicate_snapshot_rows, 0);
        assert_eq!(result.coverage.untimed_subject_rows, 0);
        assert_eq!(result.coverage.observed_matching_rows, oracle.2);
        assert_eq!(
            result.currentness,
            AccountFlowCurrentness::HostRevalidationRequired
        );
        assert_eq!(result.query_hash.len(), 64);
        assert_eq!(result.result_hash, recompute_result_hash(&result));
        assert_eq!(
            result.provenance.dataset_snapshot_id,
            args.dataset_snapshot_id
        );
        assert_eq!(result.provenance.context_epoch, args.context_epoch);
        assert_eq!(result.provenance.context_digest, args.context_digest);
        assert_eq!(result.provenance.case_binding_hash, args.case_binding_hash);
        assert_eq!(
            result.provenance.expected_producer_content_id,
            args.expected_producer_content_id
        );
        assert_eq!(
            result.provenance.expected_producer_manifest_sha256,
            args.expected_producer_manifest_sha256
        );

        let mut oracle_rows_statement = conn
            .prepare(
                "SELECT CAST(row_no AS UBIGINT), \
                        strftime(txn_ts, '%Y-%m-%dT%H:%M:%S.%fZ'), dc_flag, \
                        CAST(CAST(CAST(clean_amount AS DECIMAL(30,2)) * 100 AS HUGEINT) AS VARCHAR), currency \
                   FROM fc_transaction_norm \
                  WHERE case_id=? AND acct_no=? \
                    AND txn_ts >= CAST(? AS TIMESTAMP) AND txn_ts <= CAST(? AS TIMESTAMP) \
                  ORDER BY txn_ts, id, file_id, row_no",
            )
            .expect("prepare independent oracle row query");
        let oracle_rows = oracle_rows_statement
            .query_map(
                params![
                    args.case_id.as_str(),
                    args.resolved_account_key.as_str(),
                    "2026-01-01 00:00:00.000001",
                    "2026-01-01 00:00:00.000003",
                ],
                |row| {
                    Ok((
                        row.get::<_, u64>(0)?,
                        row.get::<_, String>(1)?,
                        row.get::<_, String>(2)?,
                        row.get::<_, String>(3)?,
                        row.get::<_, String>(4)?,
                    ))
                },
            )
            .expect("run independent oracle row query")
            .collect::<std::result::Result<Vec<_>, _>>()
            .expect("collect independent oracle rows");
        assert_eq!(oracle_rows.len(), result.evidence_rows.len());
        for (oracle_row, exact_row) in oracle_rows.iter().zip(&result.evidence_rows) {
            assert_eq!(exact_row.source_row_number, oracle_row.0);
            assert_eq!(exact_row.occurred_at, oracle_row.1);
            assert_eq!(
                exact_row.direction,
                if oracle_row.2 == "进" {
                    AccountFlowDirection::Inflow
                } else {
                    AccountFlowDirection::Outflow
                }
            );
            assert_eq!(exact_row.amount_minor, oracle_row.3);
            assert_eq!(exact_row.currency, oracle_row.4);
            assert_eq!(exact_row.minor_unit_scale, 2);
            assert!(exact_row.counterparty_key.is_some());
        }

        let lossy_double_inflow = conn
            .query_row(
                "SELECT CAST(CAST(ROUND(SUM(CASE WHEN dc_flag='进' THEN CAST(clean_amount AS DOUBLE) ELSE 0.0 END) * 100.0) AS HUGEINT) AS VARCHAR) \
                   FROM fc_transaction_norm \
                  WHERE case_id=? AND acct_no=? \
                    AND txn_ts >= CAST(? AS TIMESTAMP) AND txn_ts <= CAST(? AS TIMESTAMP)",
                params![
                    args.case_id.as_str(),
                    args.resolved_account_key.as_str(),
                    "2026-01-01 00:00:00.000001",
                    "2026-01-01 00:00:00.000003",
                ],
                |row| row.get::<_, String>(0),
            )
            .expect("run deliberately lossy DOUBLE oracle");
        assert_eq!(result.inflow_minor, "9007199254740993");
        assert_ne!(
            lossy_double_inflow, result.inflow_minor,
            "DOUBLE divergence fixture must prove f64 is not receipt eligible"
        );

        let missing_db = TempDb::materialized(
            "case-a",
            &[SeedRow {
                amount: None,
                ..seed_row(1, "2026-01-01 00:00:00", "进", "0.01", "CNY")
            }],
        );
        let missing_conn = missing_db.readonly_connection();
        assert_eq!(
            error_code(
                analyze_account_flows(&missing_conn, &missing_db.arguments())
                    .expect_err("null amount must fail at the production seam")
            ),
            "account_flow_amount_missing"
        );

        let invalid_db = TempDb::materialized(
            "case-a",
            &[seed_row(1, "2026-01-01 00:00:00", "进", "invalid", "CNY")],
        );
        let invalid_conn = invalid_db.readonly_connection();
        assert_eq!(
            error_code(
                analyze_account_flows(&invalid_conn, &invalid_db.arguments())
                    .expect_err("invalid amount must fail at the production seam")
            ),
            "account_flow_amount_parse_failed"
        );
    }

    #[test]
    fn account_ingress_resolution_uses_verified_snapshot_and_never_returns_candidate() {
        use crate::stats_query_store::account_ingress::{
            resolve_account_ingress, AccountIngressResolutionDisposition,
            ResolveAccountIngressCandidate, ResolveAccountIngressHostArguments,
            ACCOUNT_INGRESS_QUERY_CONTRACT, ACCOUNT_INGRESS_RESULT_CONTRACT,
        };

        const ACCOUNT: &str = "6222021234567890123";
        let db = TempDb::materialized_with_account_key(
            "case-a",
            ACCOUNT,
            &[seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY")],
        );
        let conn = db.readonly_connection();
        let arguments = ResolveAccountIngressHostArguments {
            case_id: "case-a".to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 7,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: db.producer_content_id.clone(),
            expected_producer_manifest_sha256: db.producer_manifest_sha256.clone(),
            candidates: vec![
                ResolveAccountIngressCandidate {
                    ordinal: 3,
                    normalized_candidate: ACCOUNT.to_string(),
                },
                ResolveAccountIngressCandidate {
                    ordinal: 9,
                    normalized_candidate: "6217009876543210987".to_string(),
                },
            ],
        };

        let result = resolve_account_ingress(&conn, &arguments)
            .expect("resolve exact current-snapshot candidates");

        assert_eq!(result.schema_version, 1);
        assert_eq!(result.contract, ACCOUNT_INGRESS_RESULT_CONTRACT);
        assert_eq!(result.resolutions.len(), 2);
        assert_eq!(result.resolutions[0].ordinal, 3);
        assert_eq!(
            result.resolutions[0].disposition,
            AccountIngressResolutionDisposition::Resolved
        );
        assert_eq!(result.resolutions[0].entity_type, "bank_account_number");
        assert_eq!(result.resolutions[1].ordinal, 9);
        assert_eq!(
            result.resolutions[1].disposition,
            AccountIngressResolutionDisposition::NotFound
        );
        assert_eq!(
            result.provenance.query_contract,
            ACCOUNT_INGRESS_QUERY_CONTRACT
        );
        assert_eq!(
            result.provenance.producer_content_id,
            db.producer_content_id
        );
        let encoded = serde_json::to_string(&result).expect("serialize safe result");
        assert!(!encoded.contains(ACCOUNT));
        assert!(!encoded.contains("6217009876543210987"));

        const CARD_ONLY: &str = "6217009876543210987";
        let card_only_db = TempDb::materialized_with_source_identifiers(
            "case-a",
            None,
            Some(CARD_ONLY),
            &[seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY")],
        );
        let card_only_arguments = ResolveAccountIngressHostArguments {
            case_id: "case-a".to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            context_epoch: 7,
            context_digest: "2".repeat(64),
            case_binding_hash: "3".repeat(64),
            expected_producer_content_id: card_only_db.producer_content_id.clone(),
            expected_producer_manifest_sha256: card_only_db.producer_manifest_sha256.clone(),
            candidates: vec![ResolveAccountIngressCandidate {
                ordinal: 1,
                normalized_candidate: CARD_ONLY.to_string(),
            }],
        };
        let card_only_result =
            resolve_account_ingress(&card_only_db.readonly_connection(), &card_only_arguments)
                .expect("card-only fallback remains unresolved");
        assert_eq!(
            card_only_result.resolutions[0].disposition,
            AccountIngressResolutionDisposition::NotFound
        );
        assert!(!serde_json::to_string(&card_only_result)
            .expect("serialize card-only result")
            .contains(CARD_ONLY));
    }

    #[test]
    fn account_flow_sql_uses_source_locator_tie_breakers() {
        assert!(ACCOUNT_FLOW_QUERY_SQL.contains("d.file_id ASC"));
        assert!(ACCOUNT_FLOW_QUERY_SQL.contains("r.row_no ASC"));
        assert!(ACCOUNT_FLOW_QUERY_SQL.contains("PARTITION BY d.file_id, d.id"));
    }

    #[test]
    fn positive_fixed_offset_preserves_inclusive_instant_boundaries() {
        let db = TempDb::materialized(
            "case-a",
            &[
                seed_row(1, "2026-01-01 07:59:59.123455", "进", "9.99", "CNY"),
                seed_row(2, "2026-01-01 07:59:59.123456", "进", "0.10", "CNY"),
                seed_row(3, "2026-01-01 08:00:00.123457", "进", "0.20", "CNY"),
                seed_row(4, "2026-01-01 08:00:00.123458", "出", "9.99", "CNY"),
            ],
        );
        let conn = db.readonly_connection();
        let mut args = db.arguments();
        args.start_inclusive = "2025-12-31T23:59:59.123456Z".to_string();
        args.end_inclusive = "2026-01-01T00:00:00.123457Z".to_string();
        args.dataset_utc_offset_minutes = 480;

        let result = analyze_account_flows(&conn, &args)
            .expect("query snapshot-local timestamps at fixed +08:00");

        assert_eq!(result.timezone, "+08:00");
        assert_eq!(result.start_inclusive, "2025-12-31T23:59:59.123456Z");
        assert_eq!(result.end_inclusive, "2026-01-01T00:00:00.123457Z");
        assert_eq!(result.inflow_minor, "30");
        assert_eq!(result.outflow_minor, "0");
        assert_eq!(result.transaction_count, 2);
        assert_eq!(
            result
                .evidence_rows
                .iter()
                .map(|row| row.occurred_at.as_str())
                .collect::<Vec<_>>(),
            vec!["2025-12-31T23:59:59.123456Z", "2026-01-01T00:00:00.123457Z",]
        );
    }

    #[test]
    fn negative_fixed_offset_preserves_inclusive_instant_boundaries() {
        let db = TempDb::materialized(
            "case-a",
            &[
                seed_row(1, "2025-12-31 19:00:00.123455", "进", "9.99", "CNY"),
                seed_row(2, "2025-12-31 19:00:00.123456", "进", "0.10", "CNY"),
                seed_row(3, "2025-12-31 19:00:01.123457", "出", "0.20", "CNY"),
                seed_row(4, "2025-12-31 19:00:01.123458", "出", "9.99", "CNY"),
            ],
        );
        let conn = db.readonly_connection();
        let mut args = db.arguments();
        args.start_inclusive = "2026-01-01T00:00:00.123456Z".to_string();
        args.end_inclusive = "2026-01-01T00:00:01.123457Z".to_string();
        args.dataset_utc_offset_minutes = -300;

        let result = analyze_account_flows(&conn, &args)
            .expect("query snapshot-local timestamps at fixed -05:00");

        assert_eq!(result.timezone, "-05:00");
        assert_eq!(result.inflow_minor, "10");
        assert_eq!(result.outflow_minor, "20");
        assert_eq!(result.net_minor, "-10");
        assert_eq!(result.transaction_count, 2);
        assert_eq!(
            result
                .evidence_rows
                .iter()
                .map(|row| row.occurred_at.as_str())
                .collect::<Vec<_>>(),
            vec!["2026-01-01T00:00:00.123456Z", "2026-01-01T00:00:01.123457Z",]
        );
    }

    #[test]
    fn read_only_session_and_case_scope_fail_closed_independently() {
        let db = TempDb::materialized(
            "case-a",
            &[seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY")],
        );
        let readwrite = Connection::open(db.path()).expect("open read-write fixture");
        let error = error_code(
            analyze_account_flows(&readwrite, &db.arguments())
                .expect_err("read-write connection must be rejected"),
        );
        assert_eq!(error, "account_flow_snapshot_connection_invalid");
        drop(readwrite);

        let readonly = db.readonly_connection();
        let mut wrong_case = db.arguments();
        wrong_case.case_id = "case-b".to_string();
        let error = error_code(
            analyze_account_flows(&readonly, &wrong_case)
                .expect_err("cross-case query must be rejected"),
        );
        assert_eq!(error, "account_flow_snapshot_verification_failed");

        let mut wrong_account = db.arguments();
        wrong_account.resolved_account_key = "private-account-key-b".to_string();
        wrong_account.subject_resolution_digest = "9".repeat(64);
        let no_hit = analyze_account_flows(&readonly, &wrong_account)
            .expect("empty account observation remains pending host binding");
        assert_eq!(
            no_hit.coverage.state,
            AccountFlowCoverageState::ObservedNoHitPendingHostBinding
        );
        assert_eq!(no_hit.transaction_count, 0);
        assert!(!serde_json::to_string(&no_hit)
            .expect("serialize no-hit result")
            .contains(&wrong_account.resolved_account_key));
    }

    #[test]
    fn host_selected_producer_content_must_match_the_verified_snapshot() {
        let db = TempDb::materialized(
            "case-a",
            &[seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY")],
        );
        let readonly = db.readonly_connection();
        let mut wrong_producer = db.arguments();
        wrong_producer.expected_producer_content_id = format!("fpc1_{}", "0".repeat(64));

        let error = error_code(
            analyze_account_flows(&readonly, &wrong_producer)
                .expect_err("host selection must match the verified producer content"),
        );
        assert_eq!(error, "account_flow_snapshot_authority_binding_mismatch");
    }

    #[test]
    fn exact_source_requires_amount_and_ledger_locator_storage() {
        let conn = Connection::open_in_memory().expect("open exact-schema fixture");
        conn.execute_batch(
            "CREATE TABLE fc_transaction_norm( \
               clean_amount DOUBLE, file_id VARCHAR, row_no BIGINT \
             )",
        )
        .expect("create lossy amount fixture");

        let error = error_code(
            require_exact_account_flow_source_schema(&conn)
                .expect_err("lossy floating-point amount storage must fail closed"),
        );
        assert_eq!(error, "account_flow_exact_source_unavailable");

        let missing_locator =
            Connection::open_in_memory().expect("open missing-locator schema fixture");
        missing_locator
            .execute_batch(
                "CREATE TABLE fc_transaction_norm( \
                   clean_amount VARCHAR, file_id VARCHAR \
                 )",
            )
            .expect("create missing-locator fixture");
        let error = error_code(
            require_exact_account_flow_source_schema(&missing_locator)
                .expect_err("missing source row number must fail closed"),
        );
        assert_eq!(error, "account_flow_exact_source_unavailable");
    }

    #[test]
    fn persisted_view_cannot_replace_a_verified_snapshot_base_table() {
        let db = TempDb::materialized(
            "case-a",
            &[seed_row(1, "2026-01-01 00:00:00", "进", "0.10", "CNY")],
        );
        let mutator = Connection::open(db.path()).expect("open view-spoof fixture");
        mutator
            .execute_batch(
                "ALTER TABLE analysis_dataset_snapshot_manifest_v2 \
                   RENAME TO analysis_dataset_snapshot_manifest_v2_backing; \
                 CREATE VIEW analysis_dataset_snapshot_manifest_v2 AS \
                   SELECT * FROM analysis_dataset_snapshot_manifest_v2_backing;",
            )
            .expect("replace persisted manifest name with a view");
        drop(mutator);

        let readonly = db.readonly_connection();
        let error = error_code(
            analyze_account_flows(&readonly, &db.arguments())
                .expect_err("a view must not satisfy snapshot table identity"),
        );
        assert_eq!(error, "account_flow_snapshot_verification_failed");
    }
}
