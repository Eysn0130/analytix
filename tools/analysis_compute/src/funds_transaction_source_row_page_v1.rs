//! Staging-only transaction source-row projection for the trusted native host.
//!
//! The DuckDB import schema does not retain byte encoding, column-mapping,
//! parser-rejection, or duplicate-occurrence evidence. This operation therefore
//! admits only a constrained direct-CSV staging candidate (no skipped, error,
//! duplicate, or cleaning-rejected rows) and requires the host to replay the raw artifact
//! before it can build authoritative parsed-generation or lineage material.
//! Private import IDs are host-only inputs to that replay. Source paths are not
//! selected or returned: the host must resolve an authoritative descriptor
//! lease from the import id plus the source artifact digest.

use anyhow::{anyhow, bail, Result};
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::HashSet;
use std::path::Path;

pub const FUNDS_TRANSACTION_SOURCE_ROW_PAGE_V1_COMMAND: &str =
    "funds.transaction_source_row_page_v1";

const OPERATION: &str = FUNDS_TRANSACTION_SOURCE_ROW_PAGE_V1_COMMAND;
const RELATION: &str = "fc_transaction_raw";
const PARSER_ID: &str = "analytix.funds.transaction-row-parser";
const PARSER_VERSION: &str = "analytix.funds.transaction-row-parser/v1";
const LOCATOR_ORDERING: &str = "source_file_id_digest_asc+source_row_number_asc/v1";
const SOURCE_PROOF_STATUS: &str = "duckdb_projection_requires_host_raw_replay";
const REQUIRED_HOST_RAW_REPLAY_PROFILE: &str = "canonical_direct_csv_v1";
const MAX_SAFE_JSON_INTEGER: i64 = 9_007_199_254_740_991;
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_ROWS: u16 = 100;
const MAX_PAGE_BYTES: usize = 1024 * 1024;
const MAX_ROW_TEXT_BYTES: usize = 64 * 1024;
const MAX_CELL_TEXT_BYTES: usize = 16 * 1024;
const MAX_PRIVATE_FILE_ID_BYTES: usize = 4 * 1024;
const MAX_INVENTORY_BYTES: usize = 512 * 1024;
const MAX_INVENTORY_ENTRIES: usize = 4096;
const MAX_SOURCE_ROWS: i64 = 10_000_000;
const MAX_SOURCE_TEXT_BYTES_PER_TABLE: i64 = 32 * 1024 * 1024 * 1024;
const MAX_CANDIDATE_INPUT_BYTES: i64 = MAX_ROW_TEXT_BYTES as i64;
const MAX_CANDIDATE_BATCH_BYTES: i64 = MAX_ROW_TEXT_BYTES as i64 * (MAX_ROWS as i64 + 1);
const INVENTORY_WIRE_FIXED_BYTES_PER_ENTRY: i64 = 384;

const SOURCE_FILE_ID_DOMAIN: &[u8] = b"analytix.funds-source-file-id/v1\0";
const SOURCE_FILE_ID_DIGEST_DOMAIN: &[u8] = b"analytix.source-row-source-file-id/digest/v1\0";
const CANONICAL_ROW_DOMAIN: &[u8] = b"analytix.parsed-canonical-row/digest/v1\0";
const INVENTORY_DIGEST_DOMAIN: &[u8] = b"analytix.funds-source-inventory/v1\0";
const SNAPSHOT_DIGEST_DOMAIN: &[u8] = b"analytix.funds-source-snapshot/v1\0";
const PAGE_DIGEST_DOMAIN: &[u8] = b"analytix.funds-transaction-source-page/v1\0";

const TRANSACTION_RAW_TEXT_COLUMNS: &[(&str, &str)] = &[
    ("raw.card_no", "cardNo"),
    ("raw.acct_no", "acctNo"),
    ("raw.account_open_name", "accountOpenName"),
    ("raw.opener_id_no", "openerIdNo"),
    ("raw.txn_time", "txnTime"),
    ("raw.amount", "amount"),
    ("raw.balance", "balance"),
    ("raw.dc_flag", "dcFlag"),
    ("raw.counterparty_acct", "counterpartyAcct"),
    ("raw.cash_flag", "cashFlag"),
    ("raw.counterparty_name", "counterpartyName"),
    ("raw.counterparty_id_no", "counterpartyIdNo"),
    ("raw.counterparty_bank", "counterpartyBank"),
    ("raw.summary", "summary"),
    ("raw.currency", "currency"),
    ("raw.branch_name", "branchName"),
    ("raw.branch_code", "branchCode"),
    ("raw.location", "location"),
    ("raw.is_success", "isSuccess"),
    ("raw.voucher_no", "voucherNo"),
    ("raw.terminal_no", "terminalNo"),
    ("raw.ip_addr", "ipAddr"),
    ("raw.mac_addr", "macAddr"),
    ("raw.counterparty_balance", "counterpartyBalance"),
    ("raw.txn_id", "txnId"),
    ("raw.log_id", "logId"),
    ("raw.voucher_type", "voucherType"),
    ("raw.voucher_id", "voucherId"),
    ("raw.teller_no", "tellerNo"),
    ("raw.merchant_name", "merchantName"),
    ("raw.merchant_no", "merchantNo"),
    ("raw.remark", "remark"),
    ("raw.txn_type", "txnType"),
    ("raw.query_feedback_reason", "queryFeedbackReason"),
];

const SOURCE_VALUES_JSON_SQL: &str = r#"to_json(struct_pack(
         cardNo := r.card_no_raw,
         acctNo := r.acct_no_raw,
         accountOpenName := r.account_open_name_raw,
         openerIdNo := r.opener_id_no_raw,
         txnTime := r.txn_time_raw,
         amount := r.amount_raw,
         balance := r.balance_raw,
         dcFlag := r.dc_flag_raw,
         counterpartyAcct := r.counterparty_acct_raw,
         cashFlag := r.cash_flag_raw,
         counterpartyName := r.counterparty_name_raw,
         counterpartyIdNo := r.counterparty_id_no_raw,
         counterpartyBank := r.counterparty_bank_raw,
         summary := r.summary_raw,
         currency := r.currency_raw,
         branchName := r.branch_name_raw,
         branchCode := r.branch_code_raw,
         location := r.location_raw,
         isSuccess := r.is_success_raw,
         voucherNo := r.voucher_no_raw,
         terminalNo := r.terminal_no_raw,
         ipAddr := r.ip_addr_raw,
         macAddr := r.mac_addr_raw,
         counterpartyBalance := r.counterparty_balance_raw,
         txnId := r.txn_id_raw,
         logId := r.log_id_raw,
         voucherType := r.voucher_type_raw,
         voucherId := r.voucher_id_raw,
         tellerNo := r.teller_no_raw,
         merchantName := r.merchant_name_raw,
         merchantNo := r.merchant_no_raw,
         remark := r.remark_raw,
         txnType := r.txn_type_raw,
         queryFeedbackReason := r.query_feedback_reason_raw,
         txnTs := CAST(n.txn_ts AS VARCHAR),
         cleanAmount := n.clean_amount,
         cleanBalance := n.clean_balance,
         cleanDcFlag := n.clean_dc_flag,
         cleanCardNo := n.clean_card_no,
         cleanAcctNo := n.clean_acct_no
       ))"#;

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct FundsTransactionSourceRowPageV1Arguments {
    pub case_id: String,
    pub binding_key_digest: String,
    pub parsed_generation_identity_sha256: String,
    pub relation: String,
    pub max_rows: u16,
    pub cursor: Option<FundsTransactionSourceRowCursorV1>,
}

#[derive(Clone, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct FundsTransactionSourceRowCursorV1 {
    pub source_file_id_digest: String,
    pub source_row_number: u64,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionSourceSnapshotV1 {
    pub source_revision: u64,
    pub source_row_count: u64,
    pub source_max_txn_ts: String,
    pub source_max_id: u64,
    pub accepted_row_count: u64,
    pub rejected_row_count: u64,
    pub duplicate_row_count: u64,
    pub inventory_digest: String,
    pub source_snapshot_digest: String,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionSourceInventoryEntryV1 {
    pub source_file_id: String,
    pub source_file_id_digest: String,
    pub private_import_file_id: String,
    pub source_artifact_sha256: String,
    pub file_type: String,
    pub row_count: u64,
    pub accepted_row_count: u64,
    pub rejected_row_count: u64,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionSourceRowV1 {
    pub source_file_id: String,
    pub source_file_id_digest: String,
    pub source_row_number: u64,
    pub source_artifact_sha256: String,
    pub canonical_row_sha256: String,
    pub canonical_typed_row: Vec<FundsTransactionTypedFieldV1>,
    pub disposition: FundsTransactionSourceRowDispositionV1,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionTypedFieldV1 {
    pub name: String,
    pub scalar: FundsTransactionTypedScalarV1,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionTypedScalarV1 {
    pub kind: FundsTransactionTypedScalarKindV1,
    pub value: String,
}

#[derive(Clone, Copy, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum FundsTransactionTypedScalarKindV1 {
    Null,
    Integer,
    Decimal,
    Text,
}

#[derive(Clone, Copy, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum FundsTransactionSourceRowDispositionV1 {
    Accepted,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsTransactionSourceRowPageV1Result {
    pub schema_version: u8,
    pub operation: String,
    pub case_id: String,
    pub binding_key_digest: String,
    pub parsed_generation_identity_sha256: String,
    pub relation: String,
    pub materialization_identity: String,
    pub producer_content_contract: String,
    pub producer_content_id: String,
    pub producer_content_manifest_sha256: String,
    pub raw_artifact_manifest_sha256: String,
    pub duckdb_content_snapshot_digest: String,
    pub duckdb_snapshot_manifest_sha256: String,
    pub analytical_schema_digest: String,
    pub parser_id: String,
    pub parser_version: String,
    pub locator_ordering: String,
    pub source_proof_status: String,
    pub required_host_raw_replay_profile: String,
    pub host_raw_replay_required: bool,
    pub source_snapshot: FundsTransactionSourceSnapshotV1,
    pub inventory: Vec<FundsTransactionSourceInventoryEntryV1>,
    pub rows: Vec<FundsTransactionSourceRowV1>,
    pub next_cursor: Option<FundsTransactionSourceRowCursorV1>,
    pub complete: bool,
    pub page_digest: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct SourceValues {
    card_no: Option<String>,
    acct_no: Option<String>,
    account_open_name: Option<String>,
    opener_id_no: Option<String>,
    txn_time: Option<String>,
    amount: Option<String>,
    balance: Option<String>,
    dc_flag: Option<String>,
    counterparty_acct: Option<String>,
    cash_flag: Option<String>,
    counterparty_name: Option<String>,
    counterparty_id_no: Option<String>,
    counterparty_bank: Option<String>,
    summary: Option<String>,
    currency: Option<String>,
    branch_name: Option<String>,
    branch_code: Option<String>,
    location: Option<String>,
    is_success: Option<String>,
    voucher_no: Option<String>,
    terminal_no: Option<String>,
    ip_addr: Option<String>,
    mac_addr: Option<String>,
    counterparty_balance: Option<String>,
    txn_id: Option<String>,
    log_id: Option<String>,
    voucher_type: Option<String>,
    voucher_id: Option<String>,
    teller_no: Option<String>,
    merchant_name: Option<String>,
    merchant_no: Option<String>,
    remark: Option<String>,
    txn_type: Option<String>,
    query_feedback_reason: Option<String>,
    txn_ts: Option<String>,
    clean_amount: Option<String>,
    clean_balance: Option<String>,
    clean_dc_flag: Option<String>,
    clean_card_no: Option<String>,
    clean_acct_no: Option<String>,
}

struct VerifiedProducerBinding {
    materialization_identity: String,
    producer_content_contract: String,
    producer_content_id: String,
    producer_content_manifest_sha256: String,
    raw_artifact_manifest_sha256: String,
    duckdb_content_snapshot_digest: String,
    duckdb_snapshot_manifest_sha256: String,
    analytical_schema_digest: String,
}

impl SourceValues {
    fn raw_values(&self) -> [&Option<String>; 34] {
        [
            &self.card_no,
            &self.acct_no,
            &self.account_open_name,
            &self.opener_id_no,
            &self.txn_time,
            &self.amount,
            &self.balance,
            &self.dc_flag,
            &self.counterparty_acct,
            &self.cash_flag,
            &self.counterparty_name,
            &self.counterparty_id_no,
            &self.counterparty_bank,
            &self.summary,
            &self.currency,
            &self.branch_name,
            &self.branch_code,
            &self.location,
            &self.is_success,
            &self.voucher_no,
            &self.terminal_no,
            &self.ip_addr,
            &self.mac_addr,
            &self.counterparty_balance,
            &self.txn_id,
            &self.log_id,
            &self.voucher_type,
            &self.voucher_id,
            &self.teller_no,
            &self.merchant_name,
            &self.merchant_no,
            &self.remark,
            &self.txn_type,
            &self.query_feedback_reason,
        ]
    }
}

pub fn validate_funds_transaction_source_row_page_v1_arguments(
    arguments: &FundsTransactionSourceRowPageV1Arguments,
    authoritative_case_id: &str,
) -> Result<()> {
    if !valid_case_id(authoritative_case_id) {
        bail!("transaction_source_row_request_contract_invalid");
    }
    if arguments.case_id != authoritative_case_id {
        bail!("data_engine_context_mismatch");
    }
    if !is_sha256_hex(&arguments.binding_key_digest)
        || !is_sha256_hex(&arguments.parsed_generation_identity_sha256)
        || arguments.relation != RELATION
        || arguments.max_rows == 0
        || arguments.max_rows > MAX_ROWS
    {
        bail!("transaction_source_row_request_contract_invalid");
    }
    if let Some(cursor) = &arguments.cursor {
        if !is_sha256_hex(&cursor.source_file_id_digest)
            || cursor.source_row_number == 0
            || cursor.source_row_number > MAX_SAFE_JSON_INTEGER as u64
        {
            bail!("transaction_source_row_request_contract_invalid");
        }
    }
    Ok(())
}

pub fn run_funds_transaction_source_row_page_v1(
    arguments: FundsTransactionSourceRowPageV1Arguments,
    authoritative_case_id: &str,
    db_path: &Path,
) -> Result<FundsTransactionSourceRowPageV1Result> {
    validate_funds_transaction_source_row_page_v1_arguments(&arguments, authoritative_case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let conn = crate::open_account_flow_readonly_connection(db_path)
        .map_err(|_| anyhow!("transaction_source_row_snapshot_connection_invalid"))?;
    conn.execute_batch("BEGIN TRANSACTION")
        .map_err(|_| anyhow!("transaction_source_row_snapshot_begin_failed"))?;
    let result = build_page(&conn, &arguments);
    match result {
        Ok(result) => {
            conn.execute_batch("COMMIT")
                .map_err(|_| anyhow!("transaction_source_row_snapshot_commit_failed"))?;
            Ok(result)
        }
        Err(error) => {
            let _ = conn.execute_batch("ROLLBACK");
            Err(error)
        }
    }
}

fn build_page(
    conn: &Connection,
    arguments: &FundsTransactionSourceRowPageV1Arguments,
) -> Result<FundsTransactionSourceRowPageV1Result> {
    validate_schema(conn)?;
    validate_case_scope(conn, &arguments.case_id)?;
    let inventory = load_inventory(conn, &arguments.case_id)?;
    let source_snapshot = load_source_snapshot(conn, &arguments.case_id, &inventory)?;
    validate_source_text_budget(conn, &arguments.case_id)?;
    validate_exact_row_projection(conn, &arguments.case_id, &inventory, &source_snapshot)?;
    let verified_producer =
        crate::txn_daily_store::verify_txn_daily_snapshot_binding(conn, &arguments.case_id)
            .map_err(|_| anyhow!("transaction_source_row_fpc1_snapshot_invalid"))?;
    if verified_producer.case_id != arguments.case_id
        || verified_producer.source_revision as u64 != source_snapshot.source_revision
        || verified_producer.source_row_count as u64 != source_snapshot.source_row_count
        || verified_producer.accepted_row_count as u64 != source_snapshot.accepted_row_count
        || verified_producer.rejected_row_count as u64 != source_snapshot.rejected_row_count
        || verified_producer.duplicate_row_count as u64 != source_snapshot.duplicate_row_count
    {
        bail!("transaction_source_row_fpc1_snapshot_mismatch");
    }
    let producer_binding = VerifiedProducerBinding {
        materialization_identity: verified_producer.materialization_identity,
        producer_content_contract: verified_producer.producer_content_contract,
        producer_content_id: verified_producer.producer_content_id,
        producer_content_manifest_sha256: verified_producer.producer_manifest_sha256,
        raw_artifact_manifest_sha256: verified_producer.raw_artifact_manifest_sha256,
        duckdb_content_snapshot_digest: verified_producer.duckdb_content_snapshot_digest,
        duckdb_snapshot_manifest_sha256: verified_producer.duckdb_snapshot_manifest_sha256,
        analytical_schema_digest: verified_producer.schema_digest,
    };
    validate_cursor(
        conn,
        &arguments.case_id,
        &inventory,
        arguments.cursor.as_ref(),
    )?;
    let candidates = load_candidate_rows(
        conn,
        &arguments.case_id,
        &inventory,
        arguments.cursor.as_ref(),
        arguments.max_rows as usize + 1,
    )?;
    if candidates.is_empty() {
        bail!("transaction_source_row_page_unavailable");
    }

    let maximum = usize::from(arguments.max_rows).min(candidates.len());
    let mut accepted = None;
    for count in 1..=maximum {
        let has_more = candidates.len() > count;
        let candidate = build_result(
            arguments,
            &producer_binding,
            source_snapshot.clone(),
            inventory.clone(),
            candidates[..count].to_vec(),
            has_more,
        )?;
        let encoded = serde_json::to_vec(&candidate)
            .map_err(|_| anyhow!("transaction_source_row_result_contract_invalid"))?;
        if encoded.len() > MAX_PAGE_BYTES {
            break;
        }
        accepted = Some(candidate);
    }
    accepted.ok_or_else(|| anyhow!("transaction_source_row_result_limit_exceeded"))
}

fn build_result(
    arguments: &FundsTransactionSourceRowPageV1Arguments,
    producer_binding: &VerifiedProducerBinding,
    source_snapshot: FundsTransactionSourceSnapshotV1,
    inventory: Vec<FundsTransactionSourceInventoryEntryV1>,
    rows: Vec<FundsTransactionSourceRowV1>,
    has_more: bool,
) -> Result<FundsTransactionSourceRowPageV1Result> {
    let next_cursor = if has_more {
        let last = rows
            .last()
            .ok_or_else(|| anyhow!("transaction_source_row_result_contract_invalid"))?;
        Some(FundsTransactionSourceRowCursorV1 {
            source_file_id_digest: last.source_file_id_digest.clone(),
            source_row_number: last.source_row_number,
        })
    } else {
        None
    };
    let mut result = FundsTransactionSourceRowPageV1Result {
        schema_version: 1,
        operation: OPERATION.to_string(),
        case_id: arguments.case_id.clone(),
        binding_key_digest: arguments.binding_key_digest.clone(),
        parsed_generation_identity_sha256: arguments.parsed_generation_identity_sha256.clone(),
        relation: RELATION.to_string(),
        materialization_identity: producer_binding.materialization_identity.clone(),
        producer_content_contract: producer_binding.producer_content_contract.clone(),
        producer_content_id: producer_binding.producer_content_id.clone(),
        producer_content_manifest_sha256: producer_binding.producer_content_manifest_sha256.clone(),
        raw_artifact_manifest_sha256: producer_binding.raw_artifact_manifest_sha256.clone(),
        duckdb_content_snapshot_digest: producer_binding.duckdb_content_snapshot_digest.clone(),
        duckdb_snapshot_manifest_sha256: producer_binding.duckdb_snapshot_manifest_sha256.clone(),
        analytical_schema_digest: producer_binding.analytical_schema_digest.clone(),
        parser_id: PARSER_ID.to_string(),
        parser_version: PARSER_VERSION.to_string(),
        locator_ordering: LOCATOR_ORDERING.to_string(),
        source_proof_status: SOURCE_PROOF_STATUS.to_string(),
        required_host_raw_replay_profile: REQUIRED_HOST_RAW_REPLAY_PROFILE.to_string(),
        host_raw_replay_required: true,
        source_snapshot,
        inventory,
        rows,
        next_cursor,
        complete: !has_more,
        page_digest: String::new(),
    };
    result.page_digest = page_digest(&result)?;
    Ok(result)
}

fn validate_schema(conn: &Connection) -> Result<()> {
    for table in [
        "analysis_revision_state",
        "import_file_log",
        "fc_transaction_raw",
        "fc_transaction_norm",
    ] {
        require_base_table(conn, table)?;
    }
    require_exact_columns(
        conn,
        "analysis_revision_state",
        &[("revision_key", "VARCHAR"), ("revision", "BIGINT")],
    )?;
    require_exact_columns(
        conn,
        "import_file_log",
        &[
            ("file_id", "VARCHAR"),
            ("case_id", "VARCHAR"),
            ("kind", "VARCHAR"),
            ("stored_path", "VARCHAR"),
            ("file_type", "VARCHAR"),
            ("size", "BIGINT"),
            ("sha256", "VARCHAR"),
            ("rows_total", "BIGINT"),
            ("rows_imported", "BIGINT"),
            ("rows_imported_raw", "BIGINT"),
            ("rows_imported_norm", "BIGINT"),
            ("rows_dedup", "BIGINT"),
            ("rows_error", "BIGINT"),
            ("rows_skipped_non_data", "BIGINT"),
            ("import_counts_version", "BIGINT"),
            ("status", "VARCHAR"),
            ("error", "VARCHAR"),
            ("cleaned_status", "VARCHAR"),
            ("cleaned_error", "VARCHAR"),
            ("cleaning_counts_version", "BIGINT"),
        ],
    )?;
    let mut raw_columns = vec![
        ("id".to_string(), "BIGINT"),
        ("case_id".to_string(), "VARCHAR"),
        ("file_id".to_string(), "VARCHAR"),
        ("row_no".to_string(), "BIGINT"),
        ("row_hash".to_string(), "VARCHAR"),
        ("extra_json".to_string(), "VARCHAR"),
    ];
    for (_, key) in TRANSACTION_RAW_TEXT_COLUMNS {
        let snake = camel_to_snake(key);
        raw_columns.push((format!("{snake}_raw"), "VARCHAR"));
    }
    require_exact_owned_columns(conn, "fc_transaction_raw", &raw_columns)?;
    require_exact_columns(
        conn,
        "fc_transaction_norm",
        &[
            ("id", "BIGINT"),
            ("case_id", "VARCHAR"),
            ("file_id", "VARCHAR"),
            ("row_no", "BIGINT"),
            ("row_hash", "VARCHAR"),
            ("txn_ts", "TIMESTAMP"),
            ("clean_amount", "VARCHAR"),
            ("clean_balance", "VARCHAR"),
            ("clean_dc_flag", "VARCHAR"),
            ("clean_card_no", "VARCHAR"),
            ("clean_acct_no", "VARCHAR"),
            ("clean_invalid", "INTEGER"),
            ("clean_failed", "INTEGER"),
            ("clean_reversal", "INTEGER"),
            ("clean_duplicate", "INTEGER"),
        ],
    )?;
    Ok(())
}

fn validate_case_scope(conn: &Connection, case_id: &str) -> Result<()> {
    for table in ["fc_transaction_raw", "fc_transaction_norm"] {
        let sql = format!(
            "SELECT COUNT(*), COUNT(*) FILTER (WHERE case_id=?), \
                    COUNT(*) FILTER (WHERE case_id IS NULL OR TRIM(case_id)='') \
               FROM {table}"
        );
        let scope = conn.query_row(&sql, [case_id], |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        })?;
        if scope.0 <= 0 || scope.0 != scope.1 || scope.2 != 0 {
            bail!("transaction_source_row_case_scope_invalid");
        }
    }
    let import_scope = conn.query_row(
        "SELECT COUNT(*), COUNT(*) FILTER (WHERE case_id=?) \
           FROM import_file_log WHERE kind='fc_transaction'",
        [case_id],
        |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)),
    )?;
    if import_scope.0 <= 0 || import_scope.0 != import_scope.1 {
        bail!("transaction_source_row_case_scope_invalid");
    }
    Ok(())
}

fn load_inventory(
    conn: &Connection,
    case_id: &str,
) -> Result<Vec<FundsTransactionSourceInventoryEntryV1>> {
    validate_inventory_budget(conn, case_id)?;
    let mut statement = conn.prepare(
        "SELECT file_id, sha256, rows_imported_norm \
           FROM import_file_log \
          WHERE case_id=? AND kind='fc_transaction' \
          ORDER BY file_id",
    )?;
    let mapped = statement.query_map([case_id], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?,
            row.get::<_, Option<String>>(1)?,
            row.get::<_, Option<i64>>(2)?,
        ))
    })?;
    let mut inventory = Vec::new();
    let mut identities = HashSet::new();
    for row in mapped {
        let (private_file_id, sha256, rows_imported_norm) = row?;
        let private_file_id = private_file_id.unwrap_or_default();
        let sha256 = sha256.unwrap_or_default();
        if !valid_private_text(&private_file_id, MAX_PRIVATE_FILE_ID_BYTES)
            || !is_sha256_hex(&sha256)
            || rows_imported_norm.is_none_or(|count| count <= 0 || count > MAX_SAFE_JSON_INTEGER)
        {
            bail!("transaction_source_row_import_profile_unverified");
        }
        let source_file_id = derive_source_file_id(case_id, &private_file_id);
        let source_file_id_digest = derive_source_file_id_digest(&source_file_id);
        if !identities.insert((source_file_id.clone(), source_file_id_digest.clone())) {
            bail!("transaction_source_row_file_identity_collision");
        }
        let (raw_count, norm_count, rejected_count) =
            file_row_counts(conn, case_id, &private_file_id)?;
        let expected = rows_imported_norm.unwrap_or_default();
        if raw_count != expected || norm_count != expected {
            bail!("transaction_source_row_file_coverage_invalid");
        }
        if rejected_count != 0 {
            bail!("transaction_source_row_rejected_occurrence_unverified");
        }
        inventory.push(FundsTransactionSourceInventoryEntryV1 {
            source_file_id,
            source_file_id_digest,
            private_import_file_id: private_file_id,
            source_artifact_sha256: sha256,
            file_type: "CSV".to_string(),
            row_count: expected as u64,
            accepted_row_count: expected as u64,
            rejected_row_count: 0,
        });
        if inventory.len() > MAX_INVENTORY_ENTRIES {
            bail!("transaction_source_row_inventory_limit_exceeded");
        }
    }
    inventory.sort_by(|left, right| left.source_file_id_digest.cmp(&right.source_file_id_digest));
    if inventory.is_empty()
        || serde_json::to_vec(&inventory)
            .map_err(|_| anyhow!("transaction_source_row_inventory_invalid"))?
            .len()
            > MAX_INVENTORY_BYTES
    {
        bail!("transaction_source_row_inventory_limit_exceeded");
    }
    Ok(inventory)
}

fn validate_inventory_budget(conn: &Connection, case_id: &str) -> Result<()> {
    let profile = conn.query_row(
        "SELECT COUNT(*), COUNT(*) FILTER (WHERE \
                file_id IS NULL OR octet_length(encode(file_id))=0 \
                OR octet_length(encode(file_id))>? \
                OR file_type IS NULL OR file_type<>'CSV' \
                OR sha256 IS NULL OR NOT regexp_full_match(sha256, '[a-f0-9]{64}') \
                OR rows_total IS NULL OR rows_total<=0 OR rows_total>9007199254740991 \
                OR rows_imported IS NULL OR rows_imported<>rows_total \
                OR rows_imported_raw IS NULL OR rows_imported_raw<>rows_total \
                OR rows_imported_norm IS NULL OR rows_imported_norm<>rows_total \
                OR rows_dedup IS NULL OR rows_dedup<>0 \
                OR rows_error IS NULL OR rows_error<>0 \
                OR rows_skipped_non_data IS NULL OR rows_skipped_non_data<>0 \
                OR import_counts_version IS NULL OR import_counts_version<>1 \
                OR status IS NULL OR status<>'已完成' \
                OR COALESCE(error, '')<>'' \
                OR cleaned_status IS NULL OR cleaned_status<>'done' \
                OR COALESCE(cleaned_error, '')<>'' \
                OR cleaning_counts_version IS NULL OR cleaning_counts_version<>1) \
           FROM import_file_log WHERE case_id=? AND kind='fc_transaction'",
        params![MAX_PRIVATE_FILE_ID_BYTES as i64, case_id],
        |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)),
    )?;
    if profile.0 <= 0 || profile.0 > MAX_INVENTORY_ENTRIES as i64 || profile.1 != 0 {
        bail!("transaction_source_row_import_profile_unverified");
    }
    let exceeds_wire_budget = conn.query_row(
        "SELECT COALESCE(SUM(octet_length(encode(file_id)) + ?), 0)>? \
           FROM import_file_log WHERE case_id=? AND kind='fc_transaction'",
        params![
            INVENTORY_WIRE_FIXED_BYTES_PER_ENTRY,
            MAX_INVENTORY_BYTES as i64,
            case_id
        ],
        |row| row.get::<_, bool>(0),
    )?;
    if exceeds_wire_budget {
        bail!("transaction_source_row_inventory_limit_exceeded");
    }
    Ok(())
}

fn load_source_snapshot(
    conn: &Connection,
    case_id: &str,
    inventory: &[FundsTransactionSourceInventoryEntryV1],
) -> Result<FundsTransactionSourceSnapshotV1> {
    let revision = conn.query_row(
        "SELECT COUNT(*), COALESCE(MIN(revision), 0), COALESCE(MAX(revision), 0) \
           FROM analysis_revision_state WHERE revision_key='stats_flow_source'",
        [],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, i64>(1)?,
                row.get::<_, i64>(2)?,
            ))
        },
    )?;
    if revision.0 != 1 || revision.1 <= 0 || revision.1 != revision.2 {
        bail!("transaction_source_row_revision_invalid");
    }
    let source = conn.query_row(
        "SELECT COUNT(*), COALESCE(CAST(MAX(txn_ts) AS VARCHAR), ''), \
                COALESCE(MAX(id), 0), \
                COUNT(*) FILTER (WHERE clean_invalid=1 OR clean_failed=1 OR clean_reversal=1) \
           FROM fc_transaction_norm WHERE case_id=?",
        [case_id],
        |row| {
            Ok((
                row.get::<_, i64>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, i64>(2)?,
                row.get::<_, i64>(3)?,
            ))
        },
    )?;
    if source.0 <= 0
        || source.0 > MAX_SOURCE_ROWS
        || source.2 <= 0
        || source.2 > MAX_SAFE_JSON_INTEGER
        || source.3 != 0
        || !valid_optional_timestamp(&source.1)
    {
        bail!("transaction_source_row_snapshot_invalid");
    }
    let inventory_rows = inventory.iter().try_fold(0_u64, |total, entry| {
        total
            .checked_add(entry.row_count)
            .ok_or_else(|| anyhow!("transaction_source_row_inventory_count_overflow"))
    })?;
    if inventory_rows != source.0 as u64 {
        bail!("transaction_source_row_inventory_coverage_invalid");
    }
    let inventory_digest = digest_json(INVENTORY_DIGEST_DOMAIN, inventory)?;
    let mut snapshot = FundsTransactionSourceSnapshotV1 {
        source_revision: revision.1 as u64,
        source_row_count: source.0 as u64,
        source_max_txn_ts: source.1,
        source_max_id: source.2 as u64,
        accepted_row_count: source.0 as u64,
        rejected_row_count: 0,
        duplicate_row_count: 0,
        inventory_digest,
        source_snapshot_digest: String::new(),
    };
    snapshot.source_snapshot_digest = digest_json(SNAPSHOT_DIGEST_DOMAIN, &snapshot)?;
    Ok(snapshot)
}

fn validate_source_text_budget(conn: &Connection, case_id: &str) -> Result<()> {
    let mut raw_columns = vec![
        ("file_id".to_string(), MAX_PRIVATE_FILE_ID_BYTES),
        ("row_hash".to_string(), 64),
        ("extra_json".to_string(), MAX_ROW_TEXT_BYTES),
    ];
    raw_columns.extend(
        TRANSACTION_RAW_TEXT_COLUMNS
            .iter()
            .map(|(_, key)| (format!("{}_raw", camel_to_snake(key)), MAX_CELL_TEXT_BYTES)),
    );
    validate_table_text_budget(conn, case_id, "fc_transaction_raw", &raw_columns)?;

    let normalized_columns = [
        ("file_id".to_string(), MAX_PRIVATE_FILE_ID_BYTES),
        ("row_hash".to_string(), 64),
        ("clean_amount".to_string(), MAX_CELL_TEXT_BYTES),
        ("clean_balance".to_string(), MAX_CELL_TEXT_BYTES),
        ("clean_dc_flag".to_string(), MAX_CELL_TEXT_BYTES),
        ("clean_card_no".to_string(), MAX_CELL_TEXT_BYTES),
        ("clean_acct_no".to_string(), MAX_CELL_TEXT_BYTES),
    ];
    validate_table_text_budget(conn, case_id, "fc_transaction_norm", &normalized_columns)
}

fn validate_table_text_budget(
    conn: &Connection,
    case_id: &str,
    table: &str,
    columns: &[(String, usize)],
) -> Result<()> {
    let invalid = columns
        .iter()
        .map(|(column, maximum)| {
            format!("({column} IS NOT NULL AND octet_length(encode({column}))>{maximum})")
        })
        .collect::<Vec<_>>()
        .join(" OR ");
    let byte_count = columns
        .iter()
        .map(|(column, _)| format!("COALESCE(octet_length(encode({column})), 0)"))
        .collect::<Vec<_>>()
        .join(" + ");
    let sql = format!(
        "SELECT COUNT(*) FILTER (WHERE {invalid}), \
                COALESCE(SUM({byte_count}), 0)>? \
           FROM {table} WHERE case_id=?"
    );
    let result = conn.query_row(
        &sql,
        params![MAX_SOURCE_TEXT_BYTES_PER_TABLE, case_id],
        |row| Ok((row.get::<_, i64>(0)?, row.get::<_, bool>(1)?)),
    )?;
    if result.0 != 0 || result.1 {
        bail!("transaction_source_row_source_text_limit_exceeded");
    }
    Ok(())
}

fn validate_exact_row_projection(
    conn: &Connection,
    case_id: &str,
    inventory: &[FundsTransactionSourceInventoryEntryV1],
    snapshot: &FundsTransactionSourceSnapshotV1,
) -> Result<()> {
    let invalid_state = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_norm \
          WHERE case_id=? AND (id IS NULL OR id<=0 OR id>9007199254740991 \
             OR file_id IS NULL OR TRIM(file_id)='' \
             OR row_no IS NULL OR row_no<=0 OR row_no>9007199254740991 \
             OR row_hash IS NULL OR NOT regexp_full_match(row_hash, '[a-f0-9]{64}') \
             OR clean_invalid IS NULL OR clean_invalid NOT IN (0,1) \
             OR clean_failed IS NULL OR clean_failed NOT IN (0,1) \
             OR clean_reversal IS NULL OR clean_reversal NOT IN (0,1) \
             OR clean_duplicate IS NULL OR clean_duplicate<>0)",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    if invalid_state != 0 {
        bail!("transaction_source_row_normalized_state_invalid");
    }
    let ambiguous_raw = conn.query_row(
        "SELECT COUNT(*) FROM (SELECT file_id, row_no FROM fc_transaction_raw \
          WHERE case_id=? GROUP BY file_id, row_no HAVING COUNT(*)<>1)",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    let ambiguous_norm = conn.query_row(
        "SELECT COUNT(*) FROM (SELECT file_id, row_no FROM fc_transaction_norm \
          WHERE case_id=? GROUP BY file_id, row_no HAVING COUNT(*)<>1)",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    let unmatched = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_raw r FULL OUTER JOIN fc_transaction_norm n \
            ON n.case_id=r.case_id AND n.file_id=r.file_id AND n.row_no=r.row_no \
           AND n.row_hash=r.row_hash \
          WHERE COALESCE(r.case_id,n.case_id)=? AND (r.id IS NULL OR n.id IS NULL)",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    let extra_json = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_raw WHERE case_id=? \
          AND COALESCE(TRIM(extra_json), '') NOT IN ('', '{}')",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    if ambiguous_raw != 0 || ambiguous_norm != 0 || unmatched != 0 || extra_json != 0 {
        bail!("transaction_source_row_projection_ambiguous");
    }
    let raw_count = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_raw WHERE case_id=?",
        [case_id],
        |row| row.get::<_, i64>(0),
    )?;
    if raw_count != snapshot.source_row_count as i64
        || inventory
            .iter()
            .map(|entry| entry.rejected_row_count)
            .sum::<u64>()
            != snapshot.rejected_row_count
    {
        bail!("transaction_source_row_projection_coverage_invalid");
    }
    Ok(())
}

fn validate_cursor(
    conn: &Connection,
    case_id: &str,
    inventory: &[FundsTransactionSourceInventoryEntryV1],
    cursor: Option<&FundsTransactionSourceRowCursorV1>,
) -> Result<()> {
    let Some(cursor) = cursor else {
        return Ok(());
    };
    let entry = inventory
        .iter()
        .find(|entry| entry.source_file_id_digest == cursor.source_file_id_digest)
        .ok_or_else(|| anyhow!("transaction_source_row_cursor_invalid"))?;
    let exact = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_raw r JOIN fc_transaction_norm n \
            ON n.case_id=r.case_id AND n.file_id=r.file_id AND n.row_no=r.row_no \
           AND n.row_hash=r.row_hash \
          WHERE r.case_id=? AND r.file_id=? AND r.row_no=?",
        params![
            case_id,
            entry.private_import_file_id,
            cursor.source_row_number as i64
        ],
        |row| row.get::<_, i64>(0),
    )?;
    if exact != 1 {
        bail!("transaction_source_row_cursor_invalid");
    }
    Ok(())
}

fn load_candidate_rows(
    conn: &Connection,
    case_id: &str,
    inventory: &[FundsTransactionSourceInventoryEntryV1],
    cursor: Option<&FundsTransactionSourceRowCursorV1>,
    limit: usize,
) -> Result<Vec<FundsTransactionSourceRowV1>> {
    let expected_count = preflight_candidate_rows(conn, case_id, inventory, cursor, limit)?;
    let mut rows = Vec::with_capacity(limit);
    let start = cursor
        .and_then(|cursor| {
            inventory
                .iter()
                .position(|entry| entry.source_file_id_digest == cursor.source_file_id_digest)
        })
        .unwrap_or(0);
    for (index, entry) in inventory.iter().enumerate().skip(start) {
        if rows.len() >= limit {
            break;
        }
        let after = if index == start {
            cursor.map(|cursor| cursor.source_row_number).unwrap_or(0)
        } else {
            0
        };
        let remaining = limit - rows.len();
        let source_row_sql = source_row_sql();
        let mut statement = conn.prepare(&source_row_sql)?;
        let mapped = statement.query_map(
            params![
                case_id,
                entry.private_import_file_id,
                after as i64,
                remaining as i64
            ],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, i64>(3)?,
                    row.get::<_, i32>(4)?,
                    row.get::<_, i32>(5)?,
                    row.get::<_, i32>(6)?,
                    row.get::<_, i32>(7)?,
                    row.get::<_, String>(8)?,
                ))
            },
        )?;
        for mapped_row in mapped {
            let (
                raw_id,
                row_number,
                row_hash,
                norm_id,
                clean_invalid,
                clean_failed,
                clean_reversal,
                clean_duplicate,
                source_json,
            ) = mapped_row?;
            if raw_id <= 0
                || raw_id > MAX_SAFE_JSON_INTEGER
                || norm_id <= 0
                || norm_id > MAX_SAFE_JSON_INTEGER
                || row_number <= 0
                || row_number > MAX_SAFE_JSON_INTEGER
                || !is_sha256_hex(&row_hash)
                || ![clean_invalid, clean_failed, clean_reversal, clean_duplicate]
                    .iter()
                    .all(|value| matches!(value, 0 | 1))
                || clean_duplicate != 0
                || source_json.len() > MAX_ROW_TEXT_BYTES
            {
                bail!("transaction_source_row_record_invalid");
            }
            let values: SourceValues = serde_json::from_str(&source_json)
                .map_err(|_| anyhow!("transaction_source_row_record_invalid"))?;
            let canonical_typed_row =
                canonical_typed_row(clean_invalid, clean_failed, clean_reversal, &values)?;
            let canonical_row_sha256 = digest_json(CANONICAL_ROW_DOMAIN, &canonical_typed_row)?;
            rows.push(FundsTransactionSourceRowV1 {
                source_file_id: entry.source_file_id.clone(),
                source_file_id_digest: entry.source_file_id_digest.clone(),
                source_row_number: row_number as u64,
                source_artifact_sha256: entry.source_artifact_sha256.clone(),
                canonical_row_sha256,
                canonical_typed_row,
                disposition: FundsTransactionSourceRowDispositionV1::Accepted,
            });
        }
    }
    if rows.len() != expected_count {
        bail!("transaction_source_row_candidate_snapshot_changed");
    }
    Ok(rows)
}

fn preflight_candidate_rows(
    conn: &Connection,
    case_id: &str,
    inventory: &[FundsTransactionSourceInventoryEntryV1],
    cursor: Option<&FundsTransactionSourceRowCursorV1>,
    limit: usize,
) -> Result<usize> {
    let mut count = 0_usize;
    let mut json_bytes = 0_i64;
    let start = cursor
        .and_then(|cursor| {
            inventory
                .iter()
                .position(|entry| entry.source_file_id_digest == cursor.source_file_id_digest)
        })
        .unwrap_or(0);
    for (index, entry) in inventory.iter().enumerate().skip(start) {
        if count >= limit {
            break;
        }
        let after = if index == start {
            cursor.map(|cursor| cursor.source_row_number).unwrap_or(0)
        } else {
            0
        };
        let remaining = limit - count;
        let input_sql = source_row_projection_sql(&format!(
            "{} AS candidate_input_bytes",
            candidate_input_bytes_sql()
        ));
        let input_preflight = conn.query_row(
            &format!(
                "SELECT COUNT(*), COUNT(*) FILTER \
                        (WHERE candidate_input_bytes>?) FROM ({input_sql})"
            ),
            params![
                MAX_CANDIDATE_INPUT_BYTES,
                case_id,
                entry.private_import_file_id,
                after as i64,
                remaining as i64
            ],
            |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)),
        )?;
        if input_preflight.0 < 0 || input_preflight.0 > remaining as i64 || input_preflight.1 != 0 {
            bail!("transaction_source_row_record_limit_exceeded");
        }

        let json_sql =
            source_row_projection_sql(&format!("{SOURCE_VALUES_JSON_SQL} AS candidate_json"));
        let json_preflight = conn.query_row(
            &format!(
                "SELECT COUNT(*), COALESCE(MAX(octet_length(encode(candidate_json))), 0), \
                        COALESCE(SUM(octet_length(encode(candidate_json))), 0) \
                   FROM ({json_sql})"
            ),
            params![
                case_id,
                entry.private_import_file_id,
                after as i64,
                remaining as i64
            ],
            |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, i64>(2)?,
                ))
            },
        )?;
        if json_preflight.0 != input_preflight.0
            || json_preflight.1 > MAX_ROW_TEXT_BYTES as i64
            || json_preflight.2 < 0
        {
            bail!("transaction_source_row_record_limit_exceeded");
        }
        json_bytes = json_bytes
            .checked_add(json_preflight.2)
            .ok_or_else(|| anyhow!("transaction_source_row_record_limit_exceeded"))?;
        if json_bytes > MAX_CANDIDATE_BATCH_BYTES {
            bail!("transaction_source_row_record_limit_exceeded");
        }
        count += input_preflight.0 as usize;
    }
    Ok(count)
}

fn source_row_sql() -> String {
    source_row_projection_sql(&format!(
        "r.id, r.row_no, r.row_hash, n.id, n.clean_invalid, n.clean_failed, \
         n.clean_reversal, n.clean_duplicate, {SOURCE_VALUES_JSON_SQL}"
    ))
}

fn source_row_projection_sql(projection: &str) -> String {
    format!(
        "SELECT {projection} \
           FROM fc_transaction_raw r \
           JOIN fc_transaction_norm n \
             ON n.case_id=r.case_id \
            AND n.file_id=r.file_id \
            AND n.row_no=r.row_no \
            AND n.row_hash=r.row_hash \
          WHERE r.case_id=? AND r.file_id=? AND r.row_no>? \
          ORDER BY r.row_no ASC LIMIT ?"
    )
}

fn candidate_input_bytes_sql() -> String {
    let mut expressions = TRANSACTION_RAW_TEXT_COLUMNS
        .iter()
        .map(|(_, key)| {
            format!(
                "COALESCE(octet_length(encode(r.{}_raw)), 0)",
                camel_to_snake(key)
            )
        })
        .collect::<Vec<_>>();
    expressions.extend(
        [
            "CAST(n.txn_ts AS VARCHAR)",
            "n.clean_amount",
            "n.clean_balance",
            "n.clean_dc_flag",
            "n.clean_card_no",
            "n.clean_acct_no",
        ]
        .into_iter()
        .map(|column| format!("COALESCE(octet_length(encode({column})), 0)")),
    );
    expressions.join(" + ")
}

fn canonical_typed_row(
    clean_invalid: i32,
    clean_failed: i32,
    clean_reversal: i32,
    values: &SourceValues,
) -> Result<Vec<FundsTransactionTypedFieldV1>> {
    let mut fields = Vec::with_capacity(42);
    for ((name, _), value) in TRANSACTION_RAW_TEXT_COLUMNS.iter().zip(values.raw_values()) {
        push_text(&mut fields, name, value.as_deref())?;
    }
    push_text(&mut fields, "norm.txn_ts", values.txn_ts.as_deref())?;
    push_decimal(
        &mut fields,
        "norm.clean_amount",
        values.clean_amount.as_deref(),
    )?;
    push_decimal(
        &mut fields,
        "norm.clean_balance",
        values.clean_balance.as_deref(),
    )?;
    push_text(
        &mut fields,
        "norm.clean_dc_flag",
        values.clean_dc_flag.as_deref(),
    )?;
    push_text(
        &mut fields,
        "norm.clean_card_no",
        values.clean_card_no.as_deref(),
    )?;
    push_text(
        &mut fields,
        "norm.clean_acct_no",
        values.clean_acct_no.as_deref(),
    )?;
    push_integer(&mut fields, "norm.clean_invalid", i64::from(clean_invalid));
    push_integer(&mut fields, "norm.clean_failed", i64::from(clean_failed));
    push_integer(
        &mut fields,
        "norm.clean_reversal",
        i64::from(clean_reversal),
    );
    fields.sort_by(|left, right| left.name.cmp(&right.name));
    if fields.windows(2).any(|pair| pair[0].name >= pair[1].name) {
        bail!("transaction_source_row_typed_fields_invalid");
    }
    Ok(fields)
}

fn push_text(
    fields: &mut Vec<FundsTransactionTypedFieldV1>,
    name: &str,
    value: Option<&str>,
) -> Result<()> {
    let scalar = match value {
        Some(value) => {
            if !valid_cell_text(value) {
                bail!("transaction_source_row_cell_invalid");
            }
            FundsTransactionTypedScalarV1 {
                kind: FundsTransactionTypedScalarKindV1::Text,
                value: value.to_string(),
            }
        }
        None => FundsTransactionTypedScalarV1 {
            kind: FundsTransactionTypedScalarKindV1::Null,
            value: String::new(),
        },
    };
    fields.push(FundsTransactionTypedFieldV1 {
        name: name.to_string(),
        scalar,
    });
    Ok(())
}

fn push_decimal(
    fields: &mut Vec<FundsTransactionTypedFieldV1>,
    name: &str,
    value: Option<&str>,
) -> Result<()> {
    let scalar = match value {
        Some(value) => {
            let value = canonical_decimal_text(value)
                .ok_or_else(|| anyhow!("transaction_source_row_decimal_invalid"))?;
            FundsTransactionTypedScalarV1 {
                kind: FundsTransactionTypedScalarKindV1::Decimal,
                value,
            }
        }
        None => FundsTransactionTypedScalarV1 {
            kind: FundsTransactionTypedScalarKindV1::Null,
            value: String::new(),
        },
    };
    fields.push(FundsTransactionTypedFieldV1 {
        name: name.to_string(),
        scalar,
    });
    Ok(())
}

fn push_integer(fields: &mut Vec<FundsTransactionTypedFieldV1>, name: &str, value: i64) {
    fields.push(FundsTransactionTypedFieldV1 {
        name: name.to_string(),
        scalar: FundsTransactionTypedScalarV1 {
            kind: FundsTransactionTypedScalarKindV1::Integer,
            value: value.to_string(),
        },
    });
}

fn file_row_counts(conn: &Connection, case_id: &str, file_id: &str) -> Result<(i64, i64, i64)> {
    let raw = conn.query_row(
        "SELECT COUNT(*) FROM fc_transaction_raw WHERE case_id=? AND file_id=?",
        params![case_id, file_id],
        |row| row.get::<_, i64>(0),
    )?;
    let norm = conn.query_row(
        "SELECT COUNT(*), COUNT(*) FILTER \
                (WHERE clean_invalid=1 OR clean_failed=1 OR clean_reversal=1) \
           FROM fc_transaction_norm WHERE case_id=? AND file_id=?",
        params![case_id, file_id],
        |row| Ok((row.get::<_, i64>(0)?, row.get::<_, i64>(1)?)),
    )?;
    Ok((raw, norm.0, norm.1))
}

fn require_base_table(conn: &Connection, table: &str) -> Result<()> {
    let current = conn.query_row("SELECT current_database()", [], |row| {
        row.get::<_, String>(0)
    })?;
    let mut statement = conn.prepare(
        "SELECT table_catalog, table_schema, table_type FROM information_schema.tables \
          WHERE table_name=? ORDER BY table_catalog, table_schema, table_type",
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
    if rows.len() != 1 || rows[0] != (current, "main".to_string(), "BASE TABLE".to_string()) {
        bail!("transaction_source_row_schema_invalid");
    }
    Ok(())
}

fn require_exact_columns(conn: &Connection, table: &str, required: &[(&str, &str)]) -> Result<()> {
    let owned = required
        .iter()
        .map(|(column, expected_type)| ((*column).to_string(), *expected_type))
        .collect::<Vec<_>>();
    require_exact_owned_columns(conn, table, &owned)
}

fn require_exact_owned_columns(
    conn: &Connection,
    table: &str,
    required: &[(String, &str)],
) -> Result<()> {
    let current = conn.query_row("SELECT current_database()", [], |row| {
        row.get::<_, String>(0)
    })?;
    for (column, expected_type) in required {
        let mut statement = conn.prepare(
            "SELECT table_catalog, table_schema, data_type, COALESCE(collation_name, '') \
               FROM information_schema.columns \
              WHERE table_schema='main' AND table_name=? AND column_name=? \
              ORDER BY table_catalog, data_type, collation_name",
        )?;
        let rows = statement
            .query_map(params![table, column], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                ))
            })?
            .collect::<std::result::Result<Vec<_>, _>>()?;
        if rows.len() != 1
            || rows[0].0 != current
            || rows[0].1 != "main"
            || !rows[0].2.eq_ignore_ascii_case(expected_type)
            || !rows[0].3.is_empty()
        {
            bail!("transaction_source_row_schema_invalid");
        }
    }
    Ok(())
}

fn page_digest(result: &FundsTransactionSourceRowPageV1Result) -> Result<String> {
    let mut value = serde_json::to_value(result)
        .map_err(|_| anyhow!("transaction_source_row_result_contract_invalid"))?;
    value
        .as_object_mut()
        .and_then(|object| object.remove("pageDigest"))
        .ok_or_else(|| anyhow!("transaction_source_row_result_contract_invalid"))?;
    digest_json(PAGE_DIGEST_DOMAIN, &value)
}

fn digest_json<T: Serialize + ?Sized>(domain: &[u8], value: &T) -> Result<String> {
    let body = go_compatible_json_bytes(value)?;
    let mut hasher = Sha256::new();
    hasher.update(domain);
    hasher.update(body);
    Ok(format!("{:x}", hasher.finalize()))
}

// Go's encoding/json escapes HTML delimiters and the two JavaScript line
// separators by default. The private native result is consumed by Go, so all
// cross-language digests use those exact bytes without round-tripping numeric
// values or text through another representation.
fn go_compatible_json_bytes<T: Serialize + ?Sized>(value: &T) -> Result<Vec<u8>> {
    let encoded = serde_json::to_vec(value)
        .map_err(|_| anyhow!("transaction_source_row_result_contract_invalid"))?;
    let mut canonical = Vec::with_capacity(encoded.len());
    let mut index = 0;
    while index < encoded.len() {
        match encoded[index] {
            b'<' => canonical.extend_from_slice(b"\\u003c"),
            b'>' => canonical.extend_from_slice(b"\\u003e"),
            b'&' => canonical.extend_from_slice(b"\\u0026"),
            0xe2 if encoded.get(index + 1) == Some(&0x80)
                && matches!(encoded.get(index + 2), Some(0xa8 | 0xa9)) =>
            {
                if encoded[index + 2] == 0xa8 {
                    canonical.extend_from_slice(b"\\u2028");
                } else {
                    canonical.extend_from_slice(b"\\u2029");
                }
                index += 2;
            }
            byte => canonical.push(byte),
        }
        index += 1;
    }
    Ok(canonical)
}

fn derive_source_file_id(case_id: &str, private_file_id: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(SOURCE_FILE_ID_DOMAIN);
    hash_framed(&mut hasher, case_id.as_bytes());
    hash_framed(&mut hasher, private_file_id.as_bytes());
    format!("{:x}", hasher.finalize())[..20].to_string()
}

fn derive_source_file_id_digest(source_file_id: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(SOURCE_FILE_ID_DIGEST_DOMAIN);
    hasher.update(source_file_id.as_bytes());
    format!("{:x}", hasher.finalize())
}

fn hash_framed(hasher: &mut Sha256, value: &[u8]) {
    hasher.update((value.len() as u64).to_be_bytes());
    hasher.update(value);
}

fn valid_case_id(value: &str) -> bool {
    valid_private_text(value, MAX_CASE_ID_BYTES)
}

fn valid_private_text(value: &str, maximum: usize) -> bool {
    !value.is_empty()
        && value == value.trim()
        && value.len() <= maximum
        && !value.chars().any(char::is_control)
}

fn valid_cell_text(value: &str) -> bool {
    value.len() <= MAX_CELL_TEXT_BYTES && !value.contains('\0')
}

fn is_sha256_hex(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn valid_optional_timestamp(value: &str) -> bool {
    value.len() <= 64
        && value == value.trim()
        && value.bytes().all(|byte| {
            byte.is_ascii_digit() || matches!(byte, b'-' | b':' | b'.' | b' ' | b'+' | b'T' | b'Z')
        })
}

fn canonical_decimal_text(value: &str) -> Option<String> {
    if value.is_empty() || value.len() > 128 || value != value.trim() {
        return None;
    }
    let (negative, unsigned) = if let Some(unsigned) = value.strip_prefix('-') {
        (true, unsigned)
    } else if let Some(unsigned) = value.strip_prefix('+') {
        (false, unsigned)
    } else {
        (false, value)
    };
    if unsigned.is_empty() {
        return None;
    }
    let mut parts = unsigned.split('.');
    let integer = parts.next().unwrap_or_default();
    let fraction = parts.next();
    if parts.next().is_some()
        || integer.is_empty()
        || !integer.bytes().all(|byte| byte.is_ascii_digit())
        || fraction.is_some_and(|digits| {
            digits.is_empty() || !digits.bytes().all(|byte| byte.is_ascii_digit())
        })
    {
        return None;
    }
    let integer = integer.trim_start_matches('0');
    let integer = if integer.is_empty() { "0" } else { integer };
    let fraction = fraction.unwrap_or_default().trim_end_matches('0');
    let zero = integer == "0" && fraction.is_empty();
    let mut canonical = String::with_capacity(value.len());
    if negative && !zero {
        canonical.push('-');
    }
    canonical.push_str(integer);
    if !fraction.is_empty() {
        canonical.push('.');
        canonical.push_str(fraction);
    }
    Some(canonical)
}

fn camel_to_snake(value: &str) -> String {
    let mut out = String::with_capacity(value.len() + 4);
    for byte in value.bytes() {
        if byte.is_ascii_uppercase() {
            out.push('_');
            out.push((byte + (b'a' - b'A')) as char);
        } else {
            out.push(byte as char);
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use duckdb::Connection;
    use serde_json::Value;
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
                "analytix-source-row-{label}-{}-{unique}.duckdb",
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

    fn arguments(max_rows: u16) -> FundsTransactionSourceRowPageV1Arguments {
        FundsTransactionSourceRowPageV1Arguments {
            case_id: "case-a".to_string(),
            binding_key_digest: "1".repeat(64),
            parsed_generation_identity_sha256: "2".repeat(64),
            relation: RELATION.to_string(),
            max_rows,
            cursor: None,
        }
    }

    #[test]
    fn arguments_are_closed_case_bound_and_pathless() {
        validate_funds_transaction_source_row_page_v1_arguments(&arguments(100), "case-a")
            .expect("valid source-row request");
        let exact = serde_json::json!({
            "caseId": "case-a",
            "bindingKeyDigest": "1".repeat(64),
            "parsedGenerationIdentitySha256": "2".repeat(64),
            "relation": RELATION,
            "maxRows": 100,
            "cursor": null
        });
        serde_json::from_value::<FundsTransactionSourceRowPageV1Arguments>(exact.clone())
            .expect("closed source-row input");
        for (key, value) in [
            ("sql", Value::String("SELECT * FROM secret".to_string())),
            ("dbPath", Value::String("/private/case.duckdb".to_string())),
            ("sourcePath", Value::String("/private/raw.csv".to_string())),
        ] {
            let mut hostile = exact.clone();
            hostile
                .as_object_mut()
                .expect("source-row object")
                .insert(key.to_string(), value);
            assert!(
                serde_json::from_value::<FundsTransactionSourceRowPageV1Arguments>(hostile)
                    .is_err()
            );
        }
        let mut mismatch = arguments(100);
        mismatch.case_id = "case-b".to_string();
        assert!(
            validate_funds_transaction_source_row_page_v1_arguments(&mismatch, "case-a").is_err()
        );
        for limit in [0, 101] {
            assert!(validate_funds_transaction_source_row_page_v1_arguments(
                &arguments(limit),
                "case-a"
            )
            .is_err());
        }
    }

    #[test]
    fn decimal_canonicalization_is_exact_and_never_uses_binary_float() {
        for (input, expected) in [
            ("12.50", "12.5"),
            ("00012.5000", "12.5"),
            ("-0.00", "0"),
            ("+100", "100"),
            (
                "-9007199254740991.000000000000000001",
                "-9007199254740991.000000000000000001",
            ),
        ] {
            assert_eq!(canonical_decimal_text(input).as_deref(), Some(expected));
        }
        for invalid in ["", ".5", "5.", "1e3", "NaN", " 1", "1 "] {
            assert_eq!(canonical_decimal_text(invalid), None);
        }
    }

    #[test]
    fn digest_json_bytes_match_go_default_string_escaping() {
        let value = serde_json::json!({"text": "<>&\u{2028}\u{2029}"});
        assert_eq!(
            go_compatible_json_bytes(&value).expect("Go-compatible JSON"),
            br#"{"text":"\u003c\u003e\u0026\u2028\u2029"}"#
        );
    }

    #[test]
    fn exact_projection_is_deterministic_keyset_paginated_and_raw_replay_limited() {
        let db = TempDb::new("page");
        seed_source(&db.0);

        let first = run_funds_transaction_source_row_page_v1(arguments(1), "case-a", &db.0)
            .expect("first source-row page");
        assert_eq!(first.rows.len(), 1);
        assert!(!first.complete);
        assert!(first.next_cursor.is_some());
        assert!(first.host_raw_replay_required);
        assert_eq!(first.source_proof_status, SOURCE_PROOF_STATUS);
        assert!(first.producer_content_id.starts_with("fpc1_"));
        assert_eq!(first.producer_content_manifest_sha256.len(), 64);
        assert_eq!(first.raw_artifact_manifest_sha256, "d".repeat(64));
        assert_eq!(first.duckdb_content_snapshot_digest.len(), 64);
        assert_eq!(first.duckdb_snapshot_manifest_sha256.len(), 64);
        assert_eq!(first.analytical_schema_digest.len(), 64);
        assert_eq!(first.source_snapshot.source_row_count, 2);
        assert_eq!(first.source_snapshot.accepted_row_count, 2);
        assert_eq!(first.source_snapshot.rejected_row_count, 0);
        assert_eq!(first.source_snapshot.duplicate_row_count, 0);
        assert_eq!(first.inventory.len(), 1);
        let wire = serde_json::to_value(&first).expect("source-row wire result");
        let inventory_entry = wire
            .pointer("/inventory/0")
            .and_then(Value::as_object)
            .expect("source-row inventory entry");
        assert!(!inventory_entry.contains_key("privateStoredPath"));
        assert!(!inventory_entry.contains_key("sourceArtifactByteLength"));
        assert_eq!(
            inventory_entry
                .get("privateImportFileId")
                .and_then(Value::as_str),
            Some("file-1")
        );
        assert_eq!(
            inventory_entry
                .get("sourceArtifactSha256")
                .and_then(Value::as_str),
            Some("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
        );
        assert!(!wire.to_string().contains("/etc/hosts/.."));
        assert_eq!(first.rows[0].canonical_row_sha256.len(), 64);
        let typed = serde_json::to_value(&first.rows[0].canonical_typed_row)
            .expect("canonical typed row JSON");
        assert!(first.rows[0]
            .canonical_typed_row
            .windows(2)
            .all(|pair| pair[0].name < pair[1].name));
        assert!(typed.as_array().expect("typed fields").iter().any(|field| {
            field.get("name").and_then(Value::as_str) == Some("raw.amount")
                && field.pointer("/scalar/kind").and_then(Value::as_str) == Some("text")
                && field.pointer("/scalar/value").and_then(Value::as_str) == Some("12.50")
        }));
        assert!(typed.as_array().expect("typed fields").iter().any(|field| {
            field.get("name").and_then(Value::as_str) == Some("norm.clean_amount")
                && field.pointer("/scalar/kind").and_then(Value::as_str) == Some("decimal")
                && field.pointer("/scalar/value").and_then(Value::as_str) == Some("12.5")
        }));
        assert_eq!(first.page_digest, page_digest(&first).expect("page digest"));
        assert!(serde_json::to_vec(&first).expect("page JSON").len() <= MAX_PAGE_BYTES);

        let mut second_args = arguments(100);
        second_args.cursor = first.next_cursor.clone();
        let second = run_funds_transaction_source_row_page_v1(second_args, "case-a", &db.0)
            .expect("second source-row page");
        assert_eq!(second.rows.len(), 1);
        assert!(second.complete);
        assert!(second.next_cursor.is_none());
        let dispositions =
            serde_json::to_value([&first.rows[0], &second.rows[0]]).expect("row dispositions");
        assert!(dispositions.to_string().contains("accepted"));
        assert!(!dispositions.to_string().contains("rejected"));

        let repeated = run_funds_transaction_source_row_page_v1(arguments(1), "case-a", &db.0)
            .expect("repeat first page");
        assert_eq!(first.page_digest, repeated.page_digest);
        assert_eq!(
            first.rows[0].canonical_row_sha256,
            repeated.rows[0].canonical_row_sha256
        );
    }

    #[test]
    fn incomplete_import_provenance_and_ambiguous_rows_fail_closed() {
        for (label, mutation) in [
            (
                "dedup",
                "UPDATE import_file_log SET rows_dedup=1, rows_total=3",
            ),
            (
                "mapping",
                "UPDATE fc_transaction_raw SET extra_json='{\"unproved\":\"mapping\"}' WHERE row_no=1",
            ),
            (
                "lineage",
                "UPDATE fc_transaction_norm SET row_hash='ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff' WHERE row_no=1",
            ),
            (
                "rejection",
                "UPDATE fc_transaction_norm SET clean_invalid=1 WHERE row_no=1",
            ),
        ] {
            let db = TempDb::new(label);
            seed_source(&db.0);
            Connection::open(&db.0)
                .expect("open mutation fixture")
                .execute_batch(mutation)
                .expect("mutate source fixture");
            assert!(run_funds_transaction_source_row_page_v1(
                arguments(100),
                "case-a",
                &db.0
            )
            .is_err());
        }
    }

    #[test]
    fn sql_preflight_rejects_oversized_cells_and_candidate_rows_before_projection() {
        let oversized_cell = TempDb::new("oversized-cell");
        seed_source(&oversized_cell.0);
        Connection::open(&oversized_cell.0)
            .expect("open oversized cell fixture")
            .execute(
                "UPDATE fc_transaction_raw SET summary_raw=? WHERE row_no=1",
                ["x".repeat(MAX_CELL_TEXT_BYTES + 1)],
            )
            .expect("stage oversized source cell");
        let error = match run_funds_transaction_source_row_page_v1(
            arguments(100),
            "case-a",
            &oversized_cell.0,
        ) {
            Ok(_) => panic!("oversized source cell must fail closed"),
            Err(error) => error,
        };
        assert!(format!("{error:#}").contains("transaction_source_row_source_text_limit_exceeded"));

        let oversized_row = TempDb::new("oversized-row");
        seed_source(&oversized_row.0);
        let conn = Connection::open(&oversized_row.0).expect("open oversized row fixture");
        let bounded_cell = "y".repeat(14 * 1024);
        conn.execute(
            "UPDATE fc_transaction_raw SET account_open_name_raw=?, opener_id_no_raw=?, \
                    counterparty_name_raw=?, counterparty_id_no_raw=?, summary_raw=? \
              WHERE row_no=1",
            params![
                bounded_cell,
                bounded_cell,
                bounded_cell,
                bounded_cell,
                bounded_cell
            ],
        )
        .expect("stage oversized candidate row");
        let inventory = load_inventory(&conn, "case-a").expect("bounded source inventory");
        let error = preflight_candidate_rows(&conn, "case-a", &inventory, None, 2)
            .expect_err("oversized candidate row must fail before JSON projection");
        assert!(format!("{error:#}").contains("transaction_source_row_record_limit_exceeded"));
    }

    fn seed_source(path: &Path) {
        let conn = Connection::open(path).expect("open source-row fixture");
        let raw_columns = TRANSACTION_RAW_TEXT_COLUMNS
            .iter()
            .map(|(_, key)| format!("{}_raw VARCHAR", camel_to_snake(key)))
            .collect::<Vec<_>>()
            .join(", ");
        conn.execute_batch(&format!(
            "CREATE TABLE analysis_revision_state(revision_key VARCHAR, revision BIGINT); \
             INSERT INTO analysis_revision_state VALUES ('stats_flow_source', 7); \
             CREATE TABLE import_file_log( \
               file_id VARCHAR, case_id VARCHAR, kind VARCHAR, stored_path VARCHAR, \
               file_type VARCHAR, size BIGINT, sha256 VARCHAR, rows_total BIGINT, \
               rows_imported BIGINT, rows_imported_raw BIGINT, rows_imported_norm BIGINT, \
               rows_dedup BIGINT, rows_error BIGINT, rows_skipped_non_data BIGINT, \
               import_counts_version BIGINT, status VARCHAR, error VARCHAR, \
               cleaned_status VARCHAR, cleaned_error VARCHAR, cleaning_counts_version BIGINT \
             ); \
             INSERT INTO import_file_log VALUES ( \
               'file-1','case-a','fc_transaction','/etc/hosts/..', \
               'CSV',1024,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', \
               2,2,2,2,0,0,0,1,'已完成','', 'done','',1 \
             ); \
             CREATE TABLE fc_transaction_raw( \
               id BIGINT, case_id VARCHAR, file_id VARCHAR, row_no BIGINT, \
               row_hash VARCHAR, extra_json VARCHAR, {raw_columns} \
             ); \
             INSERT INTO fc_transaction_raw( \
               id,case_id,file_id,row_no,row_hash,extra_json,acct_no_raw,txn_time_raw, \
               amount_raw,balance_raw,dc_flag_raw,counterparty_acct_raw,currency_raw,summary_raw \
             ) VALUES \
               (1,'case-a','file-1',1,'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',NULL, \
                'A-1','2026-01-01 01:02:03','12.50','100.00','进','CP-1','CNY','工资'), \
               (2,'case-a','file-1',2,'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','{{}}', \
                'A-1','2026-01-02 01:02:03','0.00','100.00','出','CP-2','CNY','转出'); \
             CREATE TABLE fc_transaction_norm( \
               id BIGINT, case_id VARCHAR, file_id VARCHAR, row_no BIGINT, row_hash VARCHAR, \
               txn_ts TIMESTAMP, clean_amount VARCHAR, clean_balance VARCHAR, \
               clean_dc_flag VARCHAR, clean_card_no VARCHAR, clean_acct_no VARCHAR, \
               clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER, \
               clean_duplicate INTEGER \
             ); \
             INSERT INTO fc_transaction_norm VALUES \
               (1,'case-a','file-1',1,'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', \
                TIMESTAMP '2026-01-01 01:02:03','12.50','100.00','进',NULL,'A-1',0,0,0,0), \
               (2,'case-a','file-1',2,'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', \
                TIMESTAMP '2026-01-02 01:02:03','0.00','100.00','出',NULL,'A-1',0,0,0,0);"
        ))
        .expect("seed source-row fixture");
        drop(conn);
        crate::run_funds_materialize_txn_daily_v1(
            crate::FundsMaterializeTxnDailyV1Arguments {
                case_id: "case-a".to_string(),
                source_revision: 7,
                source_row_count: 2,
                source_max_txn_ts: "2026-01-02 01:02:03".to_string(),
                source_max_id: 2,
                raw_artifact_manifest_sha256: "d".repeat(64),
            },
            path,
        )
        .expect("materialize FPC1 source-row fixture");
    }
}
