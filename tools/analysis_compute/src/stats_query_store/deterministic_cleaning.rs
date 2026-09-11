use std::fmt;
use std::io::Write;

use anyhow::{anyhow, bail, Result};
use chrono::NaiveDateTime;
use csv::{Terminator, WriterBuilder};
use duckdb::types::ValueRef;
use duckdb::{params, Connection};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

use super::session::{VerifiedStatsQueryRequest, VerifiedStatsQuerySession};

pub(crate) const DETERMINISTIC_CLEANING_COMMAND: &str = "funds.deterministic_cleaning_v1";
pub(crate) const DETERMINISTIC_CLEANING_RULE_CONTRACT: &str = r#"{"contract":"analytix.funds.deterministic-cleaning-rules/v1","accountIdentity":{"operation":"remove_ascii_space_hyphen_underscore","optional":["counterpartyAccount"],"requiredAny":["card","account"]},"currency":{"aliases":["CNY","RMB","人民币"],"output":"CNY"},"decimal":{"grammar":"^[+-]?[0-9]+(?:\\.[0-9]{0,2})?$","minorUnitScale":2,"numeric":"checked_i128","output":"canonical_plain_up_to_scale","required":["amount"]},"direction":{"in":["进","入","收入","贷","credit","in"],"out":["出","支出","借","debit","out"]},"forbiddenText":"control_or_bidi_format","null":"empty_to_canonical_null","order":["txn_ts","id","file_id","row_no"],"schemaVersion":1,"text":"trim_unicode","timestamp":{"formats":["%Y-%m-%d %H:%M:%S","%Y/%m/%d %H:%M:%S","%Y-%m-%dT%H:%M:%S"],"output":"%Y-%m-%d %H:%M:%S"}}"#;

const MAX_CASE_ID_BYTES: usize = 512;
const MAX_CELL_BYTES: usize = 4 * 1024;
const MAX_ROWS: usize = 100_000;
const MAX_SOURCE_BYTES: usize = 64 * 1024 * 1024;
const MAX_RESULT_BYTES: usize = 1024 * 1024;
const MAX_RESULT_TOKENS: usize = 32_768;
const RESULT_BASE_TOKENS: usize = 27;
const RESULT_ROW_BASE_TOKENS: usize = 9;
const RESULT_CELL_TOKENS: usize = 12;
const REQUIRED_MINOR_UNIT_SCALE: u8 = 2;
const RESULT_DIGEST_DOMAIN: &[u8] = b"analytix.funds.deterministic-cleaning-result/v1\0";
const RULE_GENERATION_DOMAIN: &[u8] = b"AnalytixFundsDeterministicCleaningGenerationV1\0";

const HEADERS: [&str; 34] = [
    "交易卡号",
    "交易账号",
    "账户开户名称",
    "开户人证件号码",
    "交易时间",
    "交易金额",
    "交易余额",
    "收付标志",
    "交易对手账卡号",
    "现金标志",
    "对手户名",
    "对手身份证号",
    "对手开户银行",
    "摘要说明",
    "交易币种",
    "交易网点名称",
    "交易网点代码",
    "交易发生地",
    "交易是否成功",
    "传票号",
    "终端号",
    "IP地址",
    "MAC地址",
    "对手交易余额",
    "交易流水号",
    "日志号",
    "凭证种类",
    "凭证号",
    "交易柜员号",
    "商户名称",
    "商户号",
    "备注",
    "交易类型",
    "查询反馈结果原因",
];

const RAW_COLUMNS: [&str; 34] = [
    "card_no_raw",
    "acct_no_raw",
    "account_open_name_raw",
    "opener_id_no_raw",
    "txn_time_raw",
    "amount_raw",
    "balance_raw",
    "dc_flag_raw",
    "counterparty_acct_raw",
    "cash_flag_raw",
    "counterparty_name_raw",
    "counterparty_id_no_raw",
    "counterparty_bank_raw",
    "summary_raw",
    "currency_raw",
    "branch_name_raw",
    "branch_code_raw",
    "location_raw",
    "is_success_raw",
    "voucher_no_raw",
    "terminal_no_raw",
    "ip_addr_raw",
    "mac_addr_raw",
    "counterparty_balance_raw",
    "txn_id_raw",
    "log_id_raw",
    "voucher_type_raw",
    "voucher_id_raw",
    "teller_no_raw",
    "merchant_name_raw",
    "merchant_no_raw",
    "remark_raw",
    "txn_type_raw",
    "query_feedback_reason_raw",
];

const DISPLAY_FIELDS: [(&str, usize); 15] = [
    ("transactionTime", 4),
    ("account", 1),
    ("card", 0),
    ("accountName", 2),
    ("identityNumber", 3),
    ("amountText", 5),
    ("direction", 7),
    ("counterpartyAccount", 8),
    ("counterpartyName", 10),
    ("counterpartyIdentityNumber", 11),
    ("counterpartyBank", 12),
    ("summary", 13),
    ("currency", 14),
    ("merchantName", 29),
    ("remark", 31),
];

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct DeterministicCleaningArguments {
    pub case_id: String,
    pub dataset_snapshot_id: String,
    pub case_binding_hash: String,
    pub expected_producer_content_id: String,
    pub expected_producer_manifest_sha256: String,
    pub rule_generation: String,
    pub rule_digest: String,
    pub dataset_utc_offset_minutes: i16,
    pub expected_currency: String,
    pub minor_unit_scale: u8,
}

impl fmt::Debug for DeterministicCleaningArguments {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter
            .debug_struct("DeterministicCleaningArguments")
            .field("dataset_snapshot_id", &self.dataset_snapshot_id)
            .field("rule_generation", &self.rule_generation)
            .field("rule_digest", &self.rule_digest)
            .finish_non_exhaustive()
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum CellState {
    Value,
    Missing,
    Null,
    Invalid,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct ExactCell {
    state: CellState,
    value: String,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct CleaningChangedCell {
    pub field: String,
    pub before_value: String,
    pub after_value: String,
    before_state: CellState,
    after_state: CellState,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct CleaningChangedRow {
    pub row_index: u32,
    pub status: String,
    pub cells: Vec<CleaningChangedCell>,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct DeterministicCleaningResult {
    pub schema_version: u8,
    pub operation: String,
    pub input_dataset_snapshot_id: String,
    pub rule_generation: String,
    pub rule_digest: String,
    pub output_artifact_sha256: String,
    pub output_artifact_byte_length: u64,
    pub row_count: u64,
    pub changed_row_count: u64,
    pub unchanged_row_count: u64,
    pub changed_rows: Vec<CleaningChangedRow>,
    pub result_digest: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct CleaningResultDigestMaterial<'a> {
    schema_version: u8,
    operation: &'a str,
    input_dataset_snapshot_id: &'a str,
    rule_generation: &'a str,
    rule_digest: &'a str,
    output_artifact_sha256: &'a str,
    output_artifact_byte_length: u64,
    row_count: u64,
    changed_row_count: u64,
    unchanged_row_count: u64,
    changed_rows: &'a [CleaningChangedRow],
}

struct CleaningResultBudget {
    canonical_bytes: usize,
    tokens: usize,
    row_count: usize,
}

impl CleaningResultBudget {
    fn new(arguments: &DeterministicCleaningArguments) -> Result<Self> {
        let metadata_ceiling = DeterministicCleaningResult {
            schema_version: 1,
            operation: DETERMINISTIC_CLEANING_COMMAND.to_string(),
            input_dataset_snapshot_id: arguments.dataset_snapshot_id.clone(),
            rule_generation: arguments.rule_generation.clone(),
            rule_digest: arguments.rule_digest.clone(),
            output_artifact_sha256: "f".repeat(64),
            output_artifact_byte_length: MAX_SOURCE_BYTES as u64,
            row_count: MAX_ROWS as u64,
            changed_row_count: MAX_ROWS as u64,
            unchanged_row_count: MAX_ROWS as u64,
            changed_rows: Vec::new(),
            result_digest: "f".repeat(64),
        };
        let canonical_bytes = go_compatible_json_bytes(&metadata_ceiling)?.len();
        if canonical_bytes >= MAX_RESULT_BYTES {
            bail!("deterministic_cleaning_result_limit_exceeded");
        }
        Ok(Self {
            canonical_bytes,
            tokens: RESULT_BASE_TOKENS,
            row_count: 0,
        })
    }

    fn include(&mut self, row: &CleaningChangedRow) -> Result<()> {
        if row.cells.is_empty() {
            bail!("deterministic_cleaning_unprojectable_change");
        }
        let row_bytes = go_compatible_json_bytes(row)?.len();
        let separator = usize::from(self.row_count > 0);
        let canonical_bytes = self
            .canonical_bytes
            .checked_add(row_bytes)
            .and_then(|value| value.checked_add(separator))
            .ok_or_else(|| anyhow!("deterministic_cleaning_result_limit_exceeded"))?;
        let row_tokens = RESULT_ROW_BASE_TOKENS
            .checked_add(
                RESULT_CELL_TOKENS
                    .checked_mul(row.cells.len())
                    .ok_or_else(|| anyhow!("deterministic_cleaning_result_limit_exceeded"))?,
            )
            .ok_or_else(|| anyhow!("deterministic_cleaning_result_limit_exceeded"))?;
        let tokens = self
            .tokens
            .checked_add(row_tokens)
            .ok_or_else(|| anyhow!("deterministic_cleaning_result_limit_exceeded"))?;
        if canonical_bytes > MAX_RESULT_BYTES || tokens > MAX_RESULT_TOKENS {
            bail!("deterministic_cleaning_result_limit_exceeded");
        }
        self.canonical_bytes = canonical_bytes;
        self.tokens = tokens;
        self.row_count += 1;
        Ok(())
    }
}

impl VerifiedStatsQueryRequest for DeterministicCleaningArguments {
    fn case_id(&self) -> &str {
        &self.case_id
    }

    fn command(&self) -> &'static str {
        DETERMINISTIC_CLEANING_COMMAND
    }

    fn canonical_scope(&self) -> Value {
        json!({
            "datasetSnapshotId": self.dataset_snapshot_id,
            "caseBindingHash": self.case_binding_hash,
            "expectedProducerContentId": self.expected_producer_content_id,
            "expectedProducerManifestSha256": self.expected_producer_manifest_sha256,
            "ruleGeneration": self.rule_generation,
            "ruleDigest": self.rule_digest,
            "datasetUtcOffsetMinutes": self.dataset_utc_offset_minutes,
            "expectedCurrency": self.expected_currency,
            "minorUnitScale": self.minor_unit_scale,
        })
    }

    fn has_required_scope(&self) -> bool {
        true
    }
}

pub(crate) fn parse_arguments(value: &Value) -> Result<DeterministicCleaningArguments> {
    serde_json::from_value(value.clone())
        .map_err(|_| anyhow!("deterministic_cleaning_request_contract_invalid"))
}

pub(crate) fn validate_arguments(arguments: &DeterministicCleaningArguments) -> Result<()> {
    let rule_digest = format!(
        "{:x}",
        Sha256::digest(DETERMINISTIC_CLEANING_RULE_CONTRACT.as_bytes())
    );
    let mut generation = Sha256::new();
    generation.update(RULE_GENERATION_DOMAIN);
    generation.update(rule_digest.as_bytes());
    let rule_generation = format!("tlgen1_{:x}", generation.finalize());
    if arguments.case_id.is_empty()
        || arguments.case_id != arguments.case_id.trim()
        || arguments.case_id.len() > MAX_CASE_ID_BYTES
        || arguments.case_id.chars().any(char::is_control)
        || !valid_prefixed_digest(&arguments.dataset_snapshot_id, "dsv2_")
        || !crate::is_sha256_hex(&arguments.case_binding_hash)
        || !valid_prefixed_digest(&arguments.expected_producer_content_id, "fpc1_")
        || !crate::is_sha256_hex(&arguments.expected_producer_manifest_sha256)
        || arguments.rule_generation != rule_generation
        || arguments.rule_digest != rule_digest
        || arguments.dataset_utc_offset_minutes < -840
        || arguments.dataset_utc_offset_minutes > 840
        || arguments.expected_currency != "CNY"
        || arguments.minor_unit_scale != REQUIRED_MINOR_UNIT_SCALE
    {
        bail!("deterministic_cleaning_request_contract_invalid");
    }
    Ok(())
}

pub(crate) fn run(
    conn: &Connection,
    arguments: &DeterministicCleaningArguments,
    output: &mut impl Write,
) -> Result<DeterministicCleaningResult> {
    validate_arguments(arguments)?;
    let mut session = VerifiedStatsQuerySession::begin(conn, arguments)?;
    let binding = session.snapshot_binding();
    if binding.producer_content_id != arguments.expected_producer_content_id
        || binding.producer_manifest_sha256 != arguments.expected_producer_manifest_sha256
        || binding.accepted_row_count <= 0
        || binding.accepted_row_count != binding.normalized_row_count
        || binding.rejected_row_count != 0
        || binding.duplicate_row_count != 0
        || binding.accepted_row_count as usize > MAX_ROWS
    {
        bail!("deterministic_cleaning_snapshot_binding_mismatch");
    }

    require_raw_schema(session.conn())?;
    let query = cleaning_query();
    let mut statement = session
        .conn()
        .prepare(&query)
        .map_err(|_| anyhow!("deterministic_cleaning_query_failed"))?;
    let mut rows = statement
        .query(params![
            arguments.case_id.as_str(),
            arguments.case_id.as_str()
        ])
        .map_err(|_| anyhow!("deterministic_cleaning_query_failed"))?;

    let mut csv = WriterBuilder::new()
        .has_headers(false)
        .terminator(Terminator::CRLF)
        .from_writer(Vec::new());
    csv.write_record(HEADERS)
        .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;
    let mut changed_rows = Vec::new();
    let mut result_budget = CleaningResultBudget::new(arguments)?;
    let mut row_count = 0usize;
    while let Some(row) = rows
        .next()
        .map_err(|_| anyhow!("deterministic_cleaning_query_failed"))?
    {
        if row_count == MAX_ROWS {
            bail!("deterministic_cleaning_row_limit_exceeded");
        }
        let mut before = Vec::with_capacity(RAW_COLUMNS.len());
        for index in 0..RAW_COLUMNS.len() {
            before
                .push(exact_cell(row.get_ref(index).map_err(|_| {
                    anyhow!("deterministic_cleaning_input_invalid")
                })?)?);
        }
        let after = clean_row(&before, &arguments.expected_currency)?;
        let changed = changed_row(
            u32::try_from(row_count).unwrap_or(u32::MAX),
            &before,
            &after,
        )?;
        if changed.status != "unchanged" {
            result_budget.include(&changed)?;
            changed_rows.push(changed);
        }
        csv.write_record(after.iter().map(|cell| cell.value.as_str()))
            .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;
        if csv.get_ref().len() > MAX_SOURCE_BYTES {
            bail!("deterministic_cleaning_output_limit_exceeded");
        }
        row_count += 1;
    }
    if row_count == 0 || row_count as i64 != binding.accepted_row_count {
        bail!("deterministic_cleaning_row_coverage_mismatch");
    }
    csv.flush()
        .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;
    let body = csv
        .into_inner()
        .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;
    if body.is_empty() || body.len() > MAX_SOURCE_BYTES {
        bail!("deterministic_cleaning_output_limit_exceeded");
    }
    let output_artifact_sha256 = format!("{:x}", Sha256::digest(&body));
    let changed_row_count = changed_rows.len() as u64;
    let unchanged_row_count = row_count as u64 - changed_row_count;
    let material = CleaningResultDigestMaterial {
        schema_version: 1,
        operation: DETERMINISTIC_CLEANING_COMMAND,
        input_dataset_snapshot_id: &arguments.dataset_snapshot_id,
        rule_generation: &arguments.rule_generation,
        rule_digest: &arguments.rule_digest,
        output_artifact_sha256: &output_artifact_sha256,
        output_artifact_byte_length: body.len() as u64,
        row_count: row_count as u64,
        changed_row_count,
        unchanged_row_count,
        changed_rows: &changed_rows,
    };
    let encoded = go_compatible_json_bytes(&material)?;
    let mut hasher = Sha256::new();
    hasher.update(RESULT_DIGEST_DOMAIN);
    hasher.update((encoded.len() as u64).to_be_bytes());
    hasher.update(&encoded);
    let result_digest = format!("{:x}", hasher.finalize());
    let result = DeterministicCleaningResult {
        schema_version: 1,
        operation: DETERMINISTIC_CLEANING_COMMAND.to_string(),
        input_dataset_snapshot_id: arguments.dataset_snapshot_id.clone(),
        rule_generation: arguments.rule_generation.clone(),
        rule_digest: arguments.rule_digest.clone(),
        output_artifact_sha256,
        output_artifact_byte_length: body.len() as u64,
        row_count: row_count as u64,
        changed_row_count,
        unchanged_row_count,
        changed_rows,
        result_digest,
    };
    if result_budget.row_count != result.changed_rows.len()
        || go_compatible_json_bytes(&result)?.len() > MAX_RESULT_BYTES
    {
        bail!("deterministic_cleaning_result_limit_exceeded");
    }
    output
        .write_all(&body)
        .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;
    output
        .flush()
        .map_err(|_| anyhow!("deterministic_cleaning_output_failed"))?;

    session.record_nonempty_range(
        DETERMINISTIC_CLEANING_COMMAND,
        &result.result_digest,
        row_count as i64,
    )?;
    session.commit()?;
    Ok(result)
}

fn cleaning_query() -> String {
    let projection = RAW_COLUMNS
        .iter()
        .map(|column| format!("s.{column}"))
        .collect::<Vec<_>>()
        .join(", ");
    format!(
        "SELECT {projection} FROM fc_transaction_raw s \
         JOIN fc_transaction_norm n ON n.case_id=? AND n.id=s.id AND n.file_id=s.file_id AND n.row_no=s.row_no \
         WHERE s.case_id=? ORDER BY n.txn_ts ASC NULLS LAST, s.id ASC NULLS LAST, \
         s.file_id ASC NULLS LAST, s.row_no ASC NULLS LAST"
    )
}

fn require_raw_schema(conn: &Connection) -> Result<()> {
    for column in RAW_COLUMNS {
        let count = conn
            .query_row(
                "SELECT COUNT(1) FROM information_schema.columns \
                 WHERE table_schema='main' AND table_name='fc_transaction_raw' \
                   AND column_name=? AND data_type='VARCHAR' AND COALESCE(collation_name, '')=''",
                [column],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|_| anyhow!("deterministic_cleaning_source_unavailable"))?;
        if count != 1 {
            bail!("deterministic_cleaning_source_unavailable");
        }
    }
    Ok(())
}

fn exact_cell(value: ValueRef<'_>) -> Result<ExactCell> {
    match value {
        ValueRef::Null => Ok(ExactCell {
            state: CellState::Null,
            value: String::new(),
        }),
        ValueRef::Text(bytes) => {
            let text = std::str::from_utf8(bytes)
                .map_err(|_| anyhow!("deterministic_cleaning_input_invalid"))?;
            if text.len() > MAX_CELL_BYTES || text.chars().any(forbidden_character) {
                bail!("deterministic_cleaning_input_invalid");
            }
            Ok(ExactCell {
                state: if text.is_empty() {
                    CellState::Missing
                } else {
                    CellState::Value
                },
                value: text.to_string(),
            })
        }
        _ => bail!("deterministic_cleaning_input_invalid"),
    }
}

fn clean_row(before: &[ExactCell], expected_currency: &str) -> Result<Vec<ExactCell>> {
    if before.len() != RAW_COLUMNS.len() {
        bail!("deterministic_cleaning_input_invalid");
    }
    let mut after = before
        .iter()
        .map(|cell| clean_text(cell))
        .collect::<Result<Vec<_>>>()?;
    after[4] = clean_timestamp(&before[4])?;
    after[5] = clean_decimal(&before[5], true)?;
    after[7] = clean_direction(&before[7])?;
    after[14] = clean_currency(&before[14], expected_currency)?;
    after[0] = clean_account_identity(&before[0])?;
    after[1] = clean_account_identity(&before[1])?;
    after[8] = clean_account_identity(&before[8])?;
    if after[0].state != CellState::Value && after[1].state != CellState::Value {
        bail!("deterministic_cleaning_required_identity_invalid");
    }
    Ok(after)
}

fn clean_text(cell: &ExactCell) -> Result<ExactCell> {
    if matches!(cell.state, CellState::Null | CellState::Missing) {
        return Ok(ExactCell {
            state: CellState::Null,
            value: String::new(),
        });
    }
    let value = cell.value.trim();
    if value.len() > MAX_CELL_BYTES || value.chars().any(forbidden_character) {
        bail!("deterministic_cleaning_text_invalid");
    }
    Ok(ExactCell {
        state: if value.is_empty() {
            CellState::Missing
        } else {
            CellState::Value
        },
        value: value.to_string(),
    })
}

fn clean_timestamp(cell: &ExactCell) -> Result<ExactCell> {
    let value = required_trimmed(cell)?;
    let formats = [
        "%Y-%m-%d %H:%M:%S",
        "%Y/%m/%d %H:%M:%S",
        "%Y-%m-%dT%H:%M:%S",
    ];
    let parsed = formats
        .iter()
        .find_map(|format| NaiveDateTime::parse_from_str(value, format).ok())
        .ok_or_else(|| anyhow!("deterministic_cleaning_timestamp_invalid"))?;
    Ok(value_cell(parsed.format("%Y-%m-%d %H:%M:%S").to_string()))
}

fn clean_decimal(cell: &ExactCell, required: bool) -> Result<ExactCell> {
    if matches!(cell.state, CellState::Null | CellState::Missing) {
        if required {
            bail!("deterministic_cleaning_decimal_invalid");
        }
        return Ok(ExactCell {
            state: CellState::Null,
            value: String::new(),
        });
    }
    let value = cell.value.trim();
    let (negative, unsigned) = if let Some(rest) = value.strip_prefix('-') {
        (true, rest)
    } else if let Some(rest) = value.strip_prefix('+') {
        (false, rest)
    } else {
        (false, value)
    };
    let mut parts = unsigned.split('.');
    let integer = parts.next().unwrap_or_default();
    let fraction = parts.next().unwrap_or_default();
    if parts.next().is_some()
        || integer.is_empty()
        || !integer.bytes().all(|byte| byte.is_ascii_digit())
        || fraction.len() > REQUIRED_MINOR_UNIT_SCALE as usize
        || !fraction.bytes().all(|byte| byte.is_ascii_digit())
    {
        bail!("deterministic_cleaning_decimal_invalid");
    }
    let integer_value = parse_digits(integer)?;
    let fraction_value = if fraction.is_empty() {
        0
    } else {
        parse_digits(fraction)?
    };
    let padded_fraction = match fraction.len() {
        0 => Some(0),
        1 => fraction_value.checked_mul(10),
        2 => Some(fraction_value),
        _ => None,
    }
    .ok_or_else(|| anyhow!("deterministic_cleaning_decimal_overflow"))?;
    let minor = integer_value
        .checked_mul(100)
        .and_then(|value| value.checked_add(padded_fraction))
        .ok_or_else(|| anyhow!("deterministic_cleaning_decimal_overflow"))?;
    let signed = if negative && minor != 0 {
        minor
            .checked_neg()
            .ok_or_else(|| anyhow!("deterministic_cleaning_decimal_overflow"))?
    } else {
        minor
    };
    Ok(value_cell(decimal_from_minor(signed)))
}

fn parse_digits(value: &str) -> Result<i128> {
    value.bytes().try_fold(0_i128, |current, byte| {
        current
            .checked_mul(10)
            .and_then(|value| value.checked_add(i128::from(byte - b'0')))
            .ok_or_else(|| anyhow!("deterministic_cleaning_decimal_overflow"))
    })
}

fn decimal_from_minor(value: i128) -> String {
    let negative = value < 0;
    let absolute = value.unsigned_abs();
    let integer = absolute / 100;
    let fraction = absolute % 100;
    let mut result = if fraction == 0 {
        integer.to_string()
    } else if fraction % 10 == 0 {
        format!("{integer}.{}", fraction / 10)
    } else {
        format!("{integer}.{fraction:02}")
    };
    if negative {
        result.insert(0, '-');
    }
    result
}

fn clean_direction(cell: &ExactCell) -> Result<ExactCell> {
    let value = required_trimmed(cell)?;
    let normalized = match value.to_ascii_lowercase().as_str() {
        "进" | "入" | "收入" | "贷" | "credit" | "in" => "进",
        "出" | "支出" | "借" | "debit" | "out" => "出",
        _ => bail!("deterministic_cleaning_direction_invalid"),
    };
    Ok(value_cell(normalized.to_string()))
}

fn clean_currency(cell: &ExactCell, expected: &str) -> Result<ExactCell> {
    let value = required_trimmed(cell)?;
    let normalized = match value.to_ascii_uppercase().as_str() {
        "CNY" | "RMB" | "人民币" => "CNY",
        _ => bail!("deterministic_cleaning_currency_invalid"),
    };
    if normalized != expected {
        bail!("deterministic_cleaning_currency_invalid");
    }
    Ok(value_cell(normalized.to_string()))
}

fn clean_account_identity(cell: &ExactCell) -> Result<ExactCell> {
    if matches!(cell.state, CellState::Null | CellState::Missing) {
        return Ok(ExactCell {
            state: CellState::Null,
            value: String::new(),
        });
    }
    let value = cell
        .value
        .trim()
        .chars()
        .filter(|character| !matches!(character, ' ' | '-' | '_'))
        .collect::<String>();
    if value.is_empty() || value.len() > MAX_CELL_BYTES || value.chars().any(forbidden_character) {
        bail!("deterministic_cleaning_account_invalid");
    }
    Ok(value_cell(value))
}

fn required_trimmed(cell: &ExactCell) -> Result<&str> {
    if cell.state != CellState::Value {
        bail!("deterministic_cleaning_required_value_missing");
    }
    let value = cell.value.trim();
    if value.is_empty() {
        bail!("deterministic_cleaning_required_value_missing");
    }
    Ok(value)
}

fn value_cell(value: String) -> ExactCell {
    ExactCell {
        state: CellState::Value,
        value,
    }
}

fn changed_row(
    row_index: u32,
    before: &[ExactCell],
    after: &[ExactCell],
) -> Result<CleaningChangedRow> {
    let cells = DISPLAY_FIELDS
        .iter()
        .filter_map(|(field, index)| {
            let left = &before[*index];
            let right = &after[*index];
            (left != right).then(|| CleaningChangedCell {
                field: (*field).to_string(),
                before_value: left.value.clone(),
                after_value: right.value.clone(),
                before_state: left.state,
                after_state: right.state,
            })
        })
        .collect::<Vec<_>>();
    let any_changed = before != after;
    let status = if !any_changed {
        "unchanged"
    } else if cells.iter().any(|cell| {
        cell.before_state == CellState::Invalid || cell.after_state == CellState::Invalid
    }) {
        "invalid"
    } else if !cells.is_empty()
        && cells.iter().all(|cell| {
            matches!(cell.before_state, CellState::Null | CellState::Missing)
                && cell.after_state == CellState::Value
        })
    {
        "added"
    } else if !cells.is_empty()
        && cells.iter().all(|cell| {
            cell.before_state == CellState::Value
                && matches!(cell.after_state, CellState::Null | CellState::Missing)
        })
    {
        "removed"
    } else {
        "changed"
    };
    if any_changed && cells.is_empty() {
        bail!("deterministic_cleaning_unprojectable_change");
    }
    Ok(CleaningChangedRow {
        row_index,
        status: status.to_string(),
        cells,
    })
}

fn forbidden_character(character: char) -> bool {
    character.is_control()
        || matches!(
            character as u32,
            0x00ad
                | 0x061c
                | 0x06dd
                | 0x070f
                | 0x08e2
                | 0x180e
                | 0xfeff
                | 0x110bd
                | 0x110cd
                | 0xe0001
        )
        || matches!(character as u32,
            0x0600..=0x0605 | 0x0890..=0x0891 | 0x200b..=0x200f | 0x202a..=0x202e |
            0x2060..=0x2064 | 0x2066..=0x206f | 0xfff9..=0xfffb | 0x13430..=0x1343f
        )
}

// Go is the host-side authority for the native result and recomputes this
// digest with encoding/json. Match its canonical string escaping exactly:
// EscapeHTML covers <, >, and &, while U+2028/U+2029 are always escaped.
fn go_compatible_json_bytes(value: &impl Serialize) -> Result<Vec<u8>> {
    let encoded = serde_json::to_string(value)
        .map_err(|_| anyhow!("deterministic_cleaning_result_contract_invalid"))?;
    let mut canonical = Vec::with_capacity(encoded.len());
    for character in encoded.chars() {
        match character {
            '<' => canonical.extend_from_slice(b"\\u003c"),
            '>' => canonical.extend_from_slice(b"\\u003e"),
            '&' => canonical.extend_from_slice(b"\\u0026"),
            '\u{2028}' => canonical.extend_from_slice(b"\\u2028"),
            '\u{2029}' => canonical.extend_from_slice(b"\\u2029"),
            _ => {
                let mut buffer = [0_u8; 4];
                canonical.extend_from_slice(character.encode_utf8(&mut buffer).as_bytes());
            }
        }
    }
    Ok(canonical)
}

fn valid_prefixed_digest(value: &str, prefix: &str) -> bool {
    value.strip_prefix(prefix).is_some_and(crate::is_sha256_hex)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn value(text: &str) -> ExactCell {
        value_cell(text.to_string())
    }

    #[test]
    fn decimal_rules_are_exact_and_reject_overflow_or_excess_scale() {
        for (input, expected) in [
            ("+001.20", "1.2"),
            ("0.10", "0.1"),
            ("-0.00", "0"),
            (
                "1701411834604692317316873037158841.05",
                "1701411834604692317316873037158841.05",
            ),
        ] {
            assert_eq!(clean_decimal(&value(input), true).unwrap().value, expected);
        }
        for invalid in [
            "1.001",
            "NaN",
            "1e3",
            "1701411834604692317316873037158841058.00",
        ] {
            assert!(clean_decimal(&value(invalid), true).is_err(), "{invalid}");
        }
    }

    #[test]
    fn cell_states_are_distinct_and_invalid_rules_fail_closed() {
        let null = ExactCell {
            state: CellState::Null,
            value: String::new(),
        };
        let missing = ExactCell {
            state: CellState::Missing,
            value: String::new(),
        };
        let invalid = ExactCell {
            state: CellState::Invalid,
            value: "bad".to_string(),
        };
        assert_ne!(null, missing);
        assert_ne!(missing, invalid);

        let mut before = vec![missing.clone(); RAW_COLUMNS.len()];
        let mut after = before.clone();
        before[5] = value("+01.20");
        after[5] = value("1.20");
        let row = changed_row(7, &before, &after).unwrap();
        assert_eq!(row.status, "changed");
        assert_eq!(row.cells[0].field, "amountText");
        assert_eq!(row.cells[0].before_value, "+01.20");
        assert_eq!(row.cells[0].after_value, "1.20");
    }

    #[test]
    fn rule_output_and_change_order_are_stable() {
        let mut before = vec![
            ExactCell {
                state: CellState::Missing,
                value: String::new()
            };
            RAW_COLUMNS.len()
        ];
        before[0] = value(" 6222-0001 ");
        before[4] = value("2026/08/22 01:02:03");
        before[5] = value("+001.20");
        before[7] = value("credit");
        before[8] = value("CP-001_ 23");
        before[14] = value("rmb");
        let first = clean_row(&before, "CNY").unwrap();
        let second = clean_row(&before, "CNY").unwrap();
        assert_eq!(first, second);
        let first_diff = serde_json::to_vec(&changed_row(0, &before, &first).unwrap()).unwrap();
        let second_diff = serde_json::to_vec(&changed_row(0, &before, &second).unwrap()).unwrap();
        assert_eq!(first_diff, second_diff);
        let encode = |row: &[ExactCell]| {
            let mut writer = WriterBuilder::new()
                .has_headers(false)
                .terminator(Terminator::CRLF)
                .from_writer(Vec::new());
            writer.write_record(HEADERS).unwrap();
            writer
                .write_record(row.iter().map(|cell| cell.value.as_str()))
                .unwrap();
            writer.into_inner().unwrap()
        };
        let first_csv = encode(&first);
        let second_csv = encode(&second);
        assert_eq!(first_csv, second_csv);
        assert_eq!(Sha256::digest(&first_csv), Sha256::digest(&second_csv));
        assert!(cleaning_query().ends_with(
            "ORDER BY n.txn_ts ASC NULLS LAST, s.id ASC NULLS LAST, s.file_id ASC NULLS LAST, s.row_no ASC NULLS LAST"
        ));
        assert_eq!(first[0].value, "62220001");
        assert_eq!(first[5].value, "1.2");
        assert_eq!(first[7].value, "进");
        assert_eq!(first[8].value, "CP00123");
        assert_eq!(first[14].value, "CNY");
    }

    #[test]
    fn changed_rows_without_allowlisted_cells_fail_closed() {
        let before = vec![value("same"); RAW_COLUMNS.len()];
        let mut after = before.clone();
        after[23] = value("different");
        assert!(changed_row(0, &before, &after).is_err());
    }

    #[test]
    fn empty_values_are_normalized_to_canonical_null_before_csv_serialization() {
        let null = ExactCell {
            state: CellState::Null,
            value: String::new(),
        };
        let expected = ExactCell {
            state: CellState::Null,
            value: String::new(),
        };
        assert_eq!(clean_text(&null).unwrap(), expected);
        assert_eq!(clean_decimal(&null, false).unwrap(), expected);
        assert_eq!(clean_account_identity(&null).unwrap(), expected);
        let missing = ExactCell {
            state: CellState::Missing,
            value: String::new(),
        };
        assert_eq!(clean_text(&missing).unwrap(), expected);
    }

    #[test]
    fn result_digest_matches_go_json_string_escaping_fixture() {
        let changed_rows = vec![CleaningChangedRow {
            row_index: 0,
            status: "changed".to_string(),
            cells: vec![CleaningChangedCell {
                field: "remark".to_string(),
                before_value: "<&>\u{2028}\u{2029}".to_string(),
                after_value: "&<>\u{2029}\u{2028}".to_string(),
                before_state: CellState::Value,
                after_state: CellState::Value,
            }],
        }];
        let input_snapshot = format!("dsv2_{}", "1".repeat(64));
        let rule_generation = format!("tlgen1_{}", "2".repeat(64));
        let rule_digest = "3".repeat(64);
        let output_digest = "4".repeat(64);
        let material = CleaningResultDigestMaterial {
            schema_version: 1,
            operation: DETERMINISTIC_CLEANING_COMMAND,
            input_dataset_snapshot_id: &input_snapshot,
            rule_generation: &rule_generation,
            rule_digest: &rule_digest,
            output_artifact_sha256: &output_digest,
            output_artifact_byte_length: 512,
            row_count: 1,
            changed_row_count: 1,
            unchanged_row_count: 0,
            changed_rows: &changed_rows,
        };
        let canonical = go_compatible_json_bytes(&material).unwrap();
        let encoded = String::from_utf8(canonical.clone()).unwrap();
        assert!(encoded.contains("\\u003c\\u0026\\u003e\\u2028\\u2029"));
        let mut hasher = Sha256::new();
        hasher.update(RESULT_DIGEST_DOMAIN);
        hasher.update((canonical.len() as u64).to_be_bytes());
        hasher.update(canonical);
        assert_eq!(
            format!("{:x}", hasher.finalize()),
            "4dffc9bc7135c00defa6aafc6439607244890f8e20ca1b200289dc1bd9f0e91b"
        );
    }

    #[test]
    fn result_budget_accepts_token_boundary_and_rejects_token_or_byte_overflow() {
        let arguments = DeterministicCleaningArguments {
            case_id: "case-budget".to_string(),
            dataset_snapshot_id: format!("dsv2_{}", "1".repeat(64)),
            case_binding_hash: "2".repeat(64),
            expected_producer_content_id: format!("fpc1_{}", "3".repeat(64)),
            expected_producer_manifest_sha256: "4".repeat(64),
            rule_generation: format!("tlgen1_{}", "5".repeat(64)),
            rule_digest: "6".repeat(64),
            dataset_utc_offset_minutes: 0,
            expected_currency: "CNY".to_string(),
            minor_unit_scale: 2,
        };
        let row = |row_index| CleaningChangedRow {
            row_index,
            status: "changed".to_string(),
            cells: vec![CleaningChangedCell {
                field: "remark".to_string(),
                before_value: "a".to_string(),
                after_value: "b".to_string(),
                before_state: CellState::Value,
                after_state: CellState::Value,
            }],
        };
        let maximum_one_cell_rows = (MAX_RESULT_TOKENS - RESULT_BASE_TOKENS)
            / (RESULT_ROW_BASE_TOKENS + RESULT_CELL_TOKENS);
        let mut token_budget = CleaningResultBudget::new(&arguments).unwrap();
        for index in 0..maximum_one_cell_rows {
            token_budget.include(&row(index as u32)).unwrap();
        }
        assert!(token_budget.tokens <= MAX_RESULT_TOKENS);
        assert!(token_budget
            .include(&row(maximum_one_cell_rows as u32))
            .is_err());

        let large_cells = DISPLAY_FIELDS
            .iter()
            .map(|(field, _)| CleaningChangedCell {
                field: (*field).to_string(),
                before_value: "<".repeat(MAX_CELL_BYTES),
                after_value: ">".repeat(MAX_CELL_BYTES),
                before_state: CellState::Value,
                after_state: CellState::Value,
            })
            .collect::<Vec<_>>();
        let mut byte_budget = CleaningResultBudget::new(&arguments).unwrap();
        byte_budget
            .include(&CleaningChangedRow {
                row_index: 0,
                status: "changed".to_string(),
                cells: large_cells.clone(),
            })
            .unwrap();
        assert!(byte_budget.canonical_bytes <= MAX_RESULT_BYTES);
        assert!(byte_budget
            .include(&CleaningChangedRow {
                row_index: 1,
                status: "changed".to_string(),
                cells: large_cells,
            })
            .is_err());
    }
}
