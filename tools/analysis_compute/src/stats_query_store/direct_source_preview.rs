use std::collections::HashSet;
use std::fmt;

use anyhow::{anyhow, bail, Result};
use duckdb::types::ValueRef;
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

use super::session::{VerifiedStatsQueryRequest, VerifiedStatsQuerySession};

pub(crate) const DIRECT_SOURCE_PREVIEW_COMMAND: &str = "funds.direct_source_preview";

const DIRECT_SOURCE_PREVIEW_PURPOSE: &str = "analytix.direct-source-preview/v1";
const DIRECT_SOURCE_PREVIEW_QUERY_HASH_DOMAIN: &[u8] =
    b"analytix.direct-source-preview-query-hash/v1";
const DIRECT_SOURCE_PREVIEW_RESULT_HASH_DOMAIN: &[u8] =
    b"analytix.direct-source-preview-result-hash/v1";
const DIRECT_SOURCE_PREVIEW_QUERY_SQL_HASH_DOMAIN: &[u8] =
    b"analytix.direct-source-preview-query-sql/v1";
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_FIELD_VALUE_BYTES: usize = 4 * 1024;
const MAX_SCANNED_TEXT_BYTES: usize = 8 * 1024 * 1024;
const MAX_RESULT_BYTES: usize = 1024 * 1024;
const MAX_ROW_OFFSET: u32 = 100_000;
const MAX_ROW_LIMIT: u16 = 100;
const MIN_DATASET_UTC_OFFSET_MINUTES: i16 = -840;
const MAX_DATASET_UTC_OFFSET_MINUTES: i16 = 840;
const REQUIRED_MINOR_UNIT_SCALE: u8 = 2;

const DIRECT_SOURCE_PREVIEW_RAW_COLUMNS: &[&str] = &[
    "account_open_name_raw",
    "acct_no_raw",
    "amount_raw",
    "card_no_raw",
    "counterparty_acct_raw",
    "counterparty_bank_raw",
    "counterparty_id_no_raw",
    "counterparty_name_raw",
    "currency_raw",
    "dc_flag_raw",
    "merchant_name_raw",
    "opener_id_no_raw",
    "remark_raw",
    "summary_raw",
    "txn_time_raw",
];

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) enum DirectSourcePreviewField {
    TransactionTime,
    Account,
    Card,
    AccountName,
    IdentityNumber,
    AmountText,
    Direction,
    CounterpartyAccount,
    CounterpartyName,
    CounterpartyIdentityNumber,
    CounterpartyBank,
    Summary,
    Currency,
    MerchantName,
    Remark,
}

impl DirectSourcePreviewField {
    fn sql_expression(self) -> &'static str {
        match self {
            Self::TransactionTime => "s.txn_time_raw",
            Self::Account => "s.acct_no_raw",
            Self::Card => "s.card_no_raw",
            Self::AccountName => "s.account_open_name_raw",
            Self::IdentityNumber => "s.opener_id_no_raw",
            Self::AmountText => "s.amount_raw",
            Self::Direction => "s.dc_flag_raw",
            Self::CounterpartyAccount => "s.counterparty_acct_raw",
            Self::CounterpartyName => "s.counterparty_name_raw",
            Self::CounterpartyIdentityNumber => "s.counterparty_id_no_raw",
            Self::CounterpartyBank => "s.counterparty_bank_raw",
            Self::Summary => "s.summary_raw",
            Self::Currency => "s.currency_raw",
            Self::MerchantName => "s.merchant_name_raw",
            Self::Remark => "s.remark_raw",
        }
    }
}

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DirectSourcePreviewHostArguments {
    pub case_id: String,
    pub dataset_snapshot_id: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub fields: Vec<DirectSourcePreviewField>,
    pub row_offset: u32,
    pub row_limit: u16,
    pub dataset_utc_offset_minutes: i16,
    pub expected_currency: String,
    pub minor_unit_scale: u8,
}

impl fmt::Debug for DirectSourcePreviewHostArguments {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("DirectSourcePreviewHostArguments")
            .field("case_id", &self.case_id)
            .field("dataset_snapshot_id", &self.dataset_snapshot_id)
            .field("case_binding_hash", &self.case_binding_hash)
            .field(
                "expected_producer_content_id",
                &self.expected_producer_content_id,
            )
            .field(
                "expected_producer_manifest_sha256",
                &self.expected_producer_manifest_sha256,
            )
            .field("fields", &self.fields)
            .field("row_offset", &self.row_offset)
            .field("row_limit", &self.row_limit)
            .field(
                "dataset_utc_offset_minutes",
                &self.dataset_utc_offset_minutes,
            )
            .field("expected_currency", &self.expected_currency)
            .field("minor_unit_scale", &self.minor_unit_scale)
            .finish()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub(crate) enum DirectSourcePreviewCurrentness {
    HostRevalidationRequired,
}

#[derive(Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DirectSourcePreviewCell {
    pub field: DirectSourcePreviewField,
    pub value: String,
}

#[derive(Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DirectSourcePreviewRow {
    pub row_index: u32,
    pub cells: Vec<DirectSourcePreviewCell>,
}

#[derive(Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DirectSourcePreviewHostResult {
    pub schema_version: u8,
    pub purpose: String,
    pub dataset_snapshot_id: String,
    pub fields: Vec<DirectSourcePreviewField>,
    pub row_offset: u32,
    pub row_limit: u16,
    pub rows: Vec<DirectSourcePreviewRow>,
    pub has_more: bool,
    pub query_hash: String,
    pub result_hash: String,
    pub currentness: DirectSourcePreviewCurrentness,
}

impl fmt::Debug for DirectSourcePreviewHostResult {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("DirectSourcePreviewHostResult")
            .field("schema_version", &self.schema_version)
            .field("purpose", &self.purpose)
            .field("dataset_snapshot_id", &self.dataset_snapshot_id)
            .field("fields", &self.fields)
            .field("row_offset", &self.row_offset)
            .field("row_limit", &self.row_limit)
            .field("row_count", &self.rows.len())
            .field("has_more", &self.has_more)
            .field("query_hash", &self.query_hash)
            .field("result_hash", &self.result_hash)
            .field("currentness", &self.currentness)
            .finish()
    }
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct DirectSourcePreviewResultHashMaterial<'a> {
    schema_version: u8,
    purpose: &'a str,
    dataset_snapshot_id: &'a str,
    fields: &'a [DirectSourcePreviewField],
    row_offset: u32,
    row_limit: u16,
    rows: &'a [DirectSourcePreviewRow],
    has_more: bool,
    query_hash: &'a str,
    currentness: DirectSourcePreviewCurrentness,
}

struct ValidatedDirectSourcePreviewArguments<'a> {
    arguments: &'a DirectSourcePreviewHostArguments,
    fields: Vec<DirectSourcePreviewField>,
}

pub(crate) fn parse_direct_source_preview_host_arguments(
    value: &Value,
) -> Result<DirectSourcePreviewHostArguments> {
    let arguments = serde_json::from_value::<DirectSourcePreviewHostArguments>(value.clone())
        .map_err(|_| anyhow!("direct_source_preview_request_contract_invalid"))?;
    Ok(arguments)
}

pub(crate) fn validate_direct_source_preview_host_arguments(
    arguments: &DirectSourcePreviewHostArguments,
) -> Result<()> {
    ValidatedDirectSourcePreviewArguments::new(arguments).map(|_| ())
}

pub(crate) fn direct_source_preview(
    conn: &Connection,
    arguments: &DirectSourcePreviewHostArguments,
) -> Result<DirectSourcePreviewHostResult> {
    direct_source_preview_inner(conn, arguments).map_err(sanitize_direct_source_preview_error)
}

fn direct_source_preview_inner(
    conn: &Connection,
    arguments: &DirectSourcePreviewHostArguments,
) -> Result<DirectSourcePreviewHostResult> {
    let validated = ValidatedDirectSourcePreviewArguments::new(arguments)?;
    let mut session = VerifiedStatsQuerySession::begin(conn, arguments)?;
    require_direct_source_schema(session.conn())?;

    let snapshot_binding = session.snapshot_binding();
    if snapshot_binding.producer_content_id != arguments.expected_producer_content_id
        || snapshot_binding.producer_manifest_sha256 != arguments.expected_producer_manifest_sha256
    {
        bail!("direct_source_preview_snapshot_authority_binding_mismatch");
    }

    let query_sql = build_direct_source_preview_query(&validated.fields);
    let query_hash = build_query_hash(
        &snapshot_binding.duckdb_content_snapshot_digest,
        &validated,
        &query_sql,
    )?;
    let scan_limit = i64::from(arguments.row_limit) + 1;
    let mut statement = conn
        .prepare(&query_sql)
        .map_err(|_| anyhow!("direct_source_preview_query_failed"))?;
    let mut rows = statement
        .query(params![
            arguments.case_id.as_str(),
            arguments.case_id.as_str(),
            scan_limit,
            i64::from(arguments.row_offset),
        ])
        .map_err(|_| anyhow!("direct_source_preview_query_failed"))?;
    let mut scanned_text_bytes = 0usize;
    let mut raw_rows = Vec::with_capacity(usize::from(arguments.row_limit) + 1);
    while let Some(row) = rows
        .next()
        .map_err(|_| anyhow!("direct_source_preview_query_failed"))?
    {
        let mut values = Vec::with_capacity(validated.fields.len());
        for index in 0..validated.fields.len() {
            values.push(required_bounded_text(
                row.get_ref(index)
                    .map_err(|_| anyhow!("direct_source_preview_result_contract_invalid"))?,
                &mut scanned_text_bytes,
            )?);
        }
        raw_rows.push(values);
    }

    let observed_row_count = i64::try_from(raw_rows.len())
        .map_err(|_| anyhow!("direct_source_preview_result_contract_invalid"))?;
    session.record_observed_range(
        DIRECT_SOURCE_PREVIEW_COMMAND,
        &query_hash,
        observed_row_count,
    )?;

    let has_more = raw_rows.len() > usize::from(arguments.row_limit);
    if has_more {
        raw_rows.pop();
    }
    let result_rows = raw_rows
        .into_iter()
        .enumerate()
        .map(|(index, values)| {
            let row_index = arguments
                .row_offset
                .checked_add(u32::try_from(index).unwrap_or(u32::MAX))
                .ok_or_else(|| anyhow!("direct_source_preview_result_contract_invalid"))?;
            let cells = validated
                .fields
                .iter()
                .copied()
                .zip(values)
                .map(|(field, value)| DirectSourcePreviewCell { field, value })
                .collect();
            Ok(DirectSourcePreviewRow { row_index, cells })
        })
        .collect::<Result<Vec<_>>>()?;
    let currentness = DirectSourcePreviewCurrentness::HostRevalidationRequired;
    let result_hash_material = DirectSourcePreviewResultHashMaterial {
        schema_version: 1,
        purpose: DIRECT_SOURCE_PREVIEW_PURPOSE,
        dataset_snapshot_id: &arguments.dataset_snapshot_id,
        fields: &validated.fields,
        row_offset: arguments.row_offset,
        row_limit: arguments.row_limit,
        rows: &result_rows,
        has_more,
        query_hash: &query_hash,
        currentness,
    };
    let result_hash = hash_serialized(
        DIRECT_SOURCE_PREVIEW_RESULT_HASH_DOMAIN,
        &result_hash_material,
    )?;
    let result = DirectSourcePreviewHostResult {
        schema_version: 1,
        purpose: DIRECT_SOURCE_PREVIEW_PURPOSE.to_string(),
        dataset_snapshot_id: arguments.dataset_snapshot_id.clone(),
        fields: validated.fields,
        row_offset: arguments.row_offset,
        row_limit: arguments.row_limit,
        rows: result_rows,
        has_more,
        query_hash,
        result_hash,
        currentness,
    };
    let result_bytes = serde_json::to_vec(&result)
        .map_err(|_| anyhow!("direct_source_preview_result_contract_invalid"))?;
    if result_bytes.len() > MAX_RESULT_BYTES {
        bail!("direct_source_preview_result_limit_exceeded");
    }
    session.commit()?;
    Ok(result)
}

impl<'a> ValidatedDirectSourcePreviewArguments<'a> {
    fn new(arguments: &'a DirectSourcePreviewHostArguments) -> Result<Self> {
        if arguments.case_id.is_empty()
            || arguments.case_id != arguments.case_id.trim()
            || arguments.case_id.len() > MAX_CASE_ID_BYTES
            || arguments.case_id.chars().any(char::is_control)
            || !valid_prefixed_sha256(&arguments.dataset_snapshot_id, "dsv2_")
            || !crate::is_sha256_hex(&arguments.case_binding_hash)
            || !valid_prefixed_sha256(&arguments.expected_producer_content_id, "fpc1_")
            || !crate::is_sha256_hex(&arguments.expected_producer_manifest_sha256)
            || arguments.row_offset > MAX_ROW_OFFSET
            || arguments.row_limit == 0
            || arguments.row_limit > MAX_ROW_LIMIT
            || arguments.dataset_utc_offset_minutes < MIN_DATASET_UTC_OFFSET_MINUTES
            || arguments.dataset_utc_offset_minutes > MAX_DATASET_UTC_OFFSET_MINUTES
            || !valid_currency(&arguments.expected_currency)
            || arguments.minor_unit_scale != REQUIRED_MINOR_UNIT_SCALE
        {
            bail!("direct_source_preview_request_contract_invalid");
        }
        let fields = dedupe_fields(&arguments.fields);
        if fields.is_empty() || fields.len() != arguments.fields.len() {
            bail!("direct_source_preview_request_contract_invalid");
        }
        Ok(Self { arguments, fields })
    }
}

impl VerifiedStatsQueryRequest for DirectSourcePreviewHostArguments {
    fn case_id(&self) -> &str {
        &self.case_id
    }

    fn command(&self) -> &'static str {
        DIRECT_SOURCE_PREVIEW_COMMAND
    }

    fn canonical_scope(&self) -> Value {
        json!({
            "datasetSnapshotId": self.dataset_snapshot_id,
            "caseBindingHash": self.case_binding_hash,
            "expectedProducerContentId": self.expected_producer_content_id,
            "expectedProducerManifestSha256": self.expected_producer_manifest_sha256,
            "fields": self.fields,
            "rowOffset": self.row_offset,
            "rowLimit": self.row_limit,
            "datasetUtcOffsetMinutes": self.dataset_utc_offset_minutes,
            "expectedCurrency": self.expected_currency,
            "minorUnitScale": self.minor_unit_scale,
        })
    }

    fn has_required_scope(&self) -> bool {
        !self.fields.is_empty()
    }
}

fn require_direct_source_schema(conn: &Connection) -> Result<()> {
    for column in DIRECT_SOURCE_PREVIEW_RAW_COLUMNS {
        let present = conn
            .query_row(
                "SELECT COUNT(1) FROM information_schema.columns \
                  WHERE table_schema='main' AND table_name='fc_transaction_raw' \
                    AND column_name=? AND data_type='VARCHAR' AND COALESCE(collation_name, '')=''",
                [*column],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|_| anyhow!("direct_source_preview_exact_source_unavailable"))?;
        if present != 1 {
            bail!("direct_source_preview_exact_source_unavailable");
        }
    }
    for (relation, column, expected_type) in [
        ("fc_transaction_raw", "case_id", "VARCHAR"),
        ("fc_transaction_raw", "file_id", "VARCHAR"),
        ("fc_transaction_raw", "id", "BIGINT"),
        ("fc_transaction_raw", "row_no", "BIGINT"),
        ("fc_transaction_norm", "case_id", "VARCHAR"),
        ("fc_transaction_norm", "file_id", "VARCHAR"),
        ("fc_transaction_norm", "id", "BIGINT"),
        ("fc_transaction_norm", "row_no", "BIGINT"),
        ("fc_transaction_norm", "txn_ts", "TIMESTAMP"),
    ] {
        let current_catalog = conn
            .query_row("SELECT current_database()", [], |row| {
                row.get::<_, String>(0)
            })
            .map_err(|_| anyhow!("direct_source_preview_exact_source_unavailable"))?;
        let mut statement = conn
            .prepare(
                "SELECT table_catalog, table_schema, data_type, \
                        COALESCE(collation_name, '') \
                   FROM information_schema.columns \
                  WHERE table_name=? AND column_name=? \
                  ORDER BY table_catalog, table_schema, data_type, collation_name",
            )
            .map_err(|_| anyhow!("direct_source_preview_exact_source_unavailable"))?;
        let rows = statement
            .query_map([relation, column], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()
            .map_err(|_| anyhow!("direct_source_preview_exact_source_unavailable"))?;
        if rows.len() != 1
            || rows[0].0 != current_catalog
            || rows[0].1 != "main"
            || !rows[0].2.eq_ignore_ascii_case(expected_type)
            || !rows[0].3.is_empty()
        {
            bail!("direct_source_preview_exact_source_unavailable");
        }
    }
    Ok(())
}

fn build_direct_source_preview_query(fields: &[DirectSourcePreviewField]) -> String {
    let projection = fields
        .iter()
        .map(|field| field.sql_expression())
        .collect::<Vec<_>>()
        .join(", ");
    format!(
        "SELECT {projection} \
           FROM fc_transaction_raw s \
           JOIN fc_transaction_norm n \
             ON n.case_id=? \
            AND n.id=s.id \
            AND n.file_id=s.file_id \
            AND n.row_no=s.row_no \
          WHERE s.case_id=? \
          ORDER BY n.txn_ts ASC NULLS LAST, \
                   s.id ASC NULLS LAST, \
                   s.file_id ASC NULLS LAST, \
                   s.row_no ASC NULLS LAST \
          LIMIT ? OFFSET ?"
    )
}

fn build_query_hash(
    duckdb_content_snapshot_digest: &str,
    validated: &ValidatedDirectSourcePreviewArguments<'_>,
    query_sql: &str,
) -> Result<String> {
    let safe_scope = json!({
        "command": DIRECT_SOURCE_PREVIEW_COMMAND,
        "datasetSnapshotId": validated.arguments.dataset_snapshot_id,
        "caseId": validated.arguments.case_id,
        "caseBindingHash": validated.arguments.case_binding_hash,
        "expectedProducerContentId": validated.arguments.expected_producer_content_id,
        "expectedProducerManifestSha256": validated.arguments.expected_producer_manifest_sha256,
        "fields": validated.fields,
        "rowOffset": validated.arguments.row_offset,
        "rowLimit": validated.arguments.row_limit,
        "datasetUtcOffsetMinutes": validated.arguments.dataset_utc_offset_minutes,
        "expectedCurrency": validated.arguments.expected_currency,
        "minorUnitScale": validated.arguments.minor_unit_scale,
    });
    let safe_scope = serde_json::to_vec(&safe_scope)
        .map_err(|_| anyhow!("direct_source_preview_hash_failed"))?;
    let query_sql_hash = hash_framed(
        DIRECT_SOURCE_PREVIEW_QUERY_SQL_HASH_DOMAIN,
        &[query_sql.as_bytes()],
    );
    Ok(hash_framed(
        DIRECT_SOURCE_PREVIEW_QUERY_HASH_DOMAIN,
        &[
            duckdb_content_snapshot_digest.as_bytes(),
            query_sql_hash.as_bytes(),
            &safe_scope,
        ],
    ))
}

fn dedupe_fields(fields: &[DirectSourcePreviewField]) -> Vec<DirectSourcePreviewField> {
    let mut seen = HashSet::new();
    fields
        .iter()
        .copied()
        .filter(|field| seen.insert(*field))
        .collect()
}

fn required_bounded_text(value: ValueRef<'_>, scanned_text_bytes: &mut usize) -> Result<String> {
    let bytes = match value {
        ValueRef::Null => return Ok(String::new()),
        ValueRef::Text(value) => value,
        _ => bail!("direct_source_preview_result_contract_invalid"),
    };
    if bytes.len() > MAX_FIELD_VALUE_BYTES {
        bail!("direct_source_preview_result_limit_exceeded");
    }
    *scanned_text_bytes = scanned_text_bytes
        .checked_add(bytes.len())
        .filter(|value| *value <= MAX_SCANNED_TEXT_BYTES)
        .ok_or_else(|| anyhow!("direct_source_preview_scan_limit_exceeded"))?;
    std::str::from_utf8(bytes)
        .map(str::to_string)
        .map_err(|_| anyhow!("direct_source_preview_result_contract_invalid"))
}

fn valid_currency(value: &str) -> bool {
    value.len() == 3 && value.bytes().all(|byte| byte.is_ascii_uppercase())
}

fn valid_prefixed_sha256(value: &str, prefix: &str) -> bool {
    value.strip_prefix(prefix).is_some_and(crate::is_sha256_hex)
}

fn hash_serialized<T: Serialize>(domain: &[u8], value: &T) -> Result<String> {
    let bytes =
        serde_json::to_vec(value).map_err(|_| anyhow!("direct_source_preview_hash_failed"))?;
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

fn sanitize_direct_source_preview_error(error: anyhow::Error) -> anyhow::Error {
    let error_chain = format!("{error:#}");
    let code = [
        "direct_source_preview_request_contract_invalid",
        "direct_source_preview_snapshot_authority_binding_mismatch",
        "direct_source_preview_exact_source_unavailable",
        "direct_source_preview_query_failed",
        "direct_source_preview_result_contract_invalid",
        "direct_source_preview_scan_limit_exceeded",
        "direct_source_preview_result_limit_exceeded",
        "direct_source_preview_hash_failed",
    ]
    .into_iter()
    .find(|code| error_chain.contains(code))
    .unwrap_or_else(|| {
        if error_chain.contains("stats_query_connection_not_readonly") {
            "direct_source_preview_snapshot_connection_invalid"
        } else if error_chain.contains("stats_query_external_access_not_disabled") {
            "direct_source_preview_external_access_not_disabled"
        } else if error_chain.contains("stats_query_") {
            "direct_source_preview_snapshot_verification_failed"
        } else {
            "direct_source_preview_internal_failed"
        }
    });
    anyhow!(code)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn arguments() -> Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "caseBindingHash": "2".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "3".repeat(64)),
            "expectedProducerManifestSha256": "4".repeat(64),
            "fields": ["account", "amountText", "currency"],
            "rowOffset": 0,
            "rowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
        })
    }

    #[test]
    fn request_is_strict_and_rejects_duplicate_fields() {
        let parsed = parse_direct_source_preview_host_arguments(&arguments())
            .expect("exact direct preview request");
        validate_direct_source_preview_host_arguments(&parsed).expect("valid request");

        let mut duplicated_arguments = arguments();
        duplicated_arguments["fields"] = json!(["account", "amountText", "account", "currency"]);
        let duplicated = parse_direct_source_preview_host_arguments(&duplicated_arguments)
            .expect("structurally valid duplicated request");
        let error = validate_direct_source_preview_host_arguments(&duplicated)
            .expect_err("duplicate fields must fail closed");
        assert_eq!(
            format!("{error:#}"),
            "direct_source_preview_request_contract_invalid"
        );

        for unknown in ["sql", "dbPath", "entityReference"] {
            let mut invalid = arguments();
            invalid
                .as_object_mut()
                .expect("request object")
                .insert(unknown.to_string(), json!("private"));
            let error = parse_direct_source_preview_host_arguments(&invalid)
                .expect_err("unknown field must fail closed");
            assert_eq!(
                format!("{error:#}"),
                "direct_source_preview_request_contract_invalid"
            );
        }
    }

    #[test]
    fn fixed_query_uses_only_allowlisted_columns_and_parameter_bindings() {
        let parsed = parse_direct_source_preview_host_arguments(&arguments()).expect("request");
        let validated = ValidatedDirectSourcePreviewArguments::new(&parsed).expect("validated");
        let query = build_direct_source_preview_query(&validated.fields);
        assert!(query.contains("s.amount_raw"));
        assert!(query.contains("n.txn_ts ASC"));
        assert!(query.contains("s.id ASC"));
        assert!(query.contains("s.file_id ASC"));
        assert!(query.contains("s.row_no ASC"));
        assert_eq!(query.matches('?').count(), 4);
        assert!(!query.contains("db_path"));
    }

    #[test]
    fn debug_result_does_not_render_cells_or_source_locators() {
        let result = DirectSourcePreviewHostResult {
            schema_version: 1,
            purpose: DIRECT_SOURCE_PREVIEW_PURPOSE.to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            fields: vec![DirectSourcePreviewField::Account],
            row_offset: 0,
            row_limit: 1,
            rows: vec![DirectSourcePreviewRow {
                row_index: 0,
                cells: vec![DirectSourcePreviewCell {
                    field: DirectSourcePreviewField::Account,
                    value: "private-source-value".to_string(),
                }],
            }],
            has_more: false,
            query_hash: "a".repeat(64),
            result_hash: "b".repeat(64),
            currentness: DirectSourcePreviewCurrentness::HostRevalidationRequired,
        };
        let debug = format!("{result:?}");
        assert!(!debug.contains("private-source-value"));
        assert!(!debug.contains("file_id"));
        assert!(!debug.contains("SELECT"));
    }
}
