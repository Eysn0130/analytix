//! Host-private staging for the first closed, replayable funds CSV profile.
//!
//! Source bytes are supplied by the native host from an inherited descriptor;
//! this module never accepts or returns a source or output path. The output is
//! an already-unlinked private inode supplied as an inherited descriptor.

use anyhow::{anyhow, bail, Context, Result};
use chrono::NaiveDateTime;
use csv::{ReaderBuilder, StringRecord};
use duckdb::{params_from_iter, types::Value as DuckValue, AccessMode, Config, Connection};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::fs::{self, DirBuilder, File, OpenOptions};
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};

#[cfg(unix)]
use std::os::fd::AsRawFd;
#[cfg(unix)]
use std::os::unix::fs::{DirBuilderExt, MetadataExt, OpenOptionsExt, PermissionsExt};

use crate::funds_materialize_txn_daily_v1::run_funds_materialize_txn_daily_v1_on_connection;
use crate::FundsMaterializeTxnDailyV1Arguments;

pub const FUNDS_BUILD_CANONICAL_CSV_SNAPSHOT_V1_COMMAND: &str =
    "funds.build_canonical_csv_snapshot_v1";
pub const FUNDS_CANONICAL_DIRECT_CSV_PROFILE_V1: &str = "canonical_direct_csv_v1";
pub const FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1: usize = 64 * 1024 * 1024;
pub const FUNDS_CANONICAL_CSV_MAX_ROWS_V1: u64 = 100_000;

const SCHEMA_VERSION: u8 = 1;
const MAX_CASE_ID_BYTES: usize = 512;
const MAX_CELL_BYTES: usize = 16 * 1024;
const MAX_HEADER_BYTES: u64 = 16 * 1024;
const MAX_DECIMAL_BYTES: usize = 128;
const MAX_SAFE_JSON_INTEGER: u64 = 9_007_199_254_740_991;
const ROW_HASH_CONTRACT: &str =
    "sha256(analytix.funds-canonical-typed-raw-occurrence/digest/v1\\0+go-json-v1)";
const ROW_HASH_DOMAIN: &[u8] = b"analytix.funds-canonical-typed-raw-occurrence/digest/v1\0";
const MAX_CANONICAL_DATABASE_BYTES: u64 = 8 * 1024 * 1024 * 1024;

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct FundsBuildCanonicalCSVSnapshotV1Arguments {
    pub case_id: String,
    pub profile: String,
    pub private_import_file_id: String,
    pub source_revision: u64,
    pub raw_artifact_manifest_sha256: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FundsBuildCanonicalCSVSnapshotV1Result {
    pub schema_version: u8,
    pub operation: String,
    pub profile: String,
    pub private_import_file_id: String,
    pub source_artifact_sha256: String,
    pub source_artifact_byte_length: u64,
    pub source_row_count: u64,
    pub source_max_txn_ts: String,
    pub source_max_id: u64,
    pub row_hash_contract: String,
    #[serde(flatten)]
    pub materialization: crate::FundsMaterializeTxnDailyV1Result,
}

#[derive(Clone, Copy)]
struct CanonicalCSVColumn {
    header: &'static str,
    field: &'static str,
    raw_column: &'static str,
}

const COLUMNS: [CanonicalCSVColumn; 34] = [
    CanonicalCSVColumn {
        header: "交易卡号",
        field: "raw.card_no",
        raw_column: "card_no_raw",
    },
    CanonicalCSVColumn {
        header: "交易账号",
        field: "raw.acct_no",
        raw_column: "acct_no_raw",
    },
    CanonicalCSVColumn {
        header: "账户开户名称",
        field: "raw.account_open_name",
        raw_column: "account_open_name_raw",
    },
    CanonicalCSVColumn {
        header: "开户人证件号码",
        field: "raw.opener_id_no",
        raw_column: "opener_id_no_raw",
    },
    CanonicalCSVColumn {
        header: "交易时间",
        field: "raw.txn_time",
        raw_column: "txn_time_raw",
    },
    CanonicalCSVColumn {
        header: "交易金额",
        field: "raw.amount",
        raw_column: "amount_raw",
    },
    CanonicalCSVColumn {
        header: "交易余额",
        field: "raw.balance",
        raw_column: "balance_raw",
    },
    CanonicalCSVColumn {
        header: "收付标志",
        field: "raw.dc_flag",
        raw_column: "dc_flag_raw",
    },
    CanonicalCSVColumn {
        header: "交易对手账卡号",
        field: "raw.counterparty_acct",
        raw_column: "counterparty_acct_raw",
    },
    CanonicalCSVColumn {
        header: "现金标志",
        field: "raw.cash_flag",
        raw_column: "cash_flag_raw",
    },
    CanonicalCSVColumn {
        header: "对手户名",
        field: "raw.counterparty_name",
        raw_column: "counterparty_name_raw",
    },
    CanonicalCSVColumn {
        header: "对手身份证号",
        field: "raw.counterparty_id_no",
        raw_column: "counterparty_id_no_raw",
    },
    CanonicalCSVColumn {
        header: "对手开户银行",
        field: "raw.counterparty_bank",
        raw_column: "counterparty_bank_raw",
    },
    CanonicalCSVColumn {
        header: "摘要说明",
        field: "raw.summary",
        raw_column: "summary_raw",
    },
    CanonicalCSVColumn {
        header: "交易币种",
        field: "raw.currency",
        raw_column: "currency_raw",
    },
    CanonicalCSVColumn {
        header: "交易网点名称",
        field: "raw.branch_name",
        raw_column: "branch_name_raw",
    },
    CanonicalCSVColumn {
        header: "交易网点代码",
        field: "raw.branch_code",
        raw_column: "branch_code_raw",
    },
    CanonicalCSVColumn {
        header: "交易发生地",
        field: "raw.location",
        raw_column: "location_raw",
    },
    CanonicalCSVColumn {
        header: "交易是否成功",
        field: "raw.is_success",
        raw_column: "is_success_raw",
    },
    CanonicalCSVColumn {
        header: "传票号",
        field: "raw.voucher_no",
        raw_column: "voucher_no_raw",
    },
    CanonicalCSVColumn {
        header: "终端号",
        field: "raw.terminal_no",
        raw_column: "terminal_no_raw",
    },
    CanonicalCSVColumn {
        header: "IP地址",
        field: "raw.ip_addr",
        raw_column: "ip_addr_raw",
    },
    CanonicalCSVColumn {
        header: "MAC地址",
        field: "raw.mac_addr",
        raw_column: "mac_addr_raw",
    },
    CanonicalCSVColumn {
        header: "对手交易余额",
        field: "raw.counterparty_balance",
        raw_column: "counterparty_balance_raw",
    },
    CanonicalCSVColumn {
        header: "交易流水号",
        field: "raw.txn_id",
        raw_column: "txn_id_raw",
    },
    CanonicalCSVColumn {
        header: "日志号",
        field: "raw.log_id",
        raw_column: "log_id_raw",
    },
    CanonicalCSVColumn {
        header: "凭证种类",
        field: "raw.voucher_type",
        raw_column: "voucher_type_raw",
    },
    CanonicalCSVColumn {
        header: "凭证号",
        field: "raw.voucher_id",
        raw_column: "voucher_id_raw",
    },
    CanonicalCSVColumn {
        header: "交易柜员号",
        field: "raw.teller_no",
        raw_column: "teller_no_raw",
    },
    CanonicalCSVColumn {
        header: "商户名称",
        field: "raw.merchant_name",
        raw_column: "merchant_name_raw",
    },
    CanonicalCSVColumn {
        header: "商户号",
        field: "raw.merchant_no",
        raw_column: "merchant_no_raw",
    },
    CanonicalCSVColumn {
        header: "备注",
        field: "raw.remark",
        raw_column: "remark_raw",
    },
    CanonicalCSVColumn {
        header: "交易类型",
        field: "raw.txn_type",
        raw_column: "txn_type_raw",
    },
    CanonicalCSVColumn {
        header: "查询反馈结果原因",
        field: "raw.query_feedback_reason",
        raw_column: "query_feedback_reason_raw",
    },
];

const CARD_INDEX: usize = 0;
const ACCOUNT_INDEX: usize = 1;
const TXN_TIME_INDEX: usize = 4;
const AMOUNT_INDEX: usize = 5;
const BALANCE_INDEX: usize = 6;
const DIRECTION_INDEX: usize = 7;
const CURRENCY_INDEX: usize = 14;

#[derive(Clone, Debug, Eq, PartialEq)]
struct ValidatedRecord {
    raw: Vec<Option<String>>,
    txn_time: String,
    amount: String,
    balance: Option<String>,
    direction: String,
    card: Option<String>,
    account: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct SourceStats {
    row_count: u64,
    max_txn_ts: String,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct RawOccurrence<'a> {
    occurrence_ordinal: u64,
    fields: &'a [RawTypedField<'a>],
}

#[derive(Serialize)]
struct RawTypedField<'a> {
    name: &'static str,
    scalar: RawTypedScalar<'a>,
}

#[derive(Serialize)]
struct RawTypedScalar<'a> {
    kind: &'static str,
    value: &'a str,
}

pub fn validate_funds_build_canonical_csv_snapshot_v1_arguments(
    arguments: &FundsBuildCanonicalCSVSnapshotV1Arguments,
    authoritative_case_id: &str,
) -> Result<()> {
    if !valid_private_text(authoritative_case_id, MAX_CASE_ID_BYTES)
        || arguments.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    if arguments.profile != FUNDS_CANONICAL_DIRECT_CSV_PROFILE_V1
        || !valid_lower_hex(&arguments.private_import_file_id, 20)
        || arguments.source_revision == 0
        || arguments.source_revision > MAX_SAFE_JSON_INTEGER
        || !valid_lower_hex(&arguments.raw_artifact_manifest_sha256, 64)
    {
        bail!("data_engine_request_contract_invalid");
    }
    Ok(())
}

pub fn run_funds_build_canonical_csv_snapshot_v1(
    arguments: FundsBuildCanonicalCSVSnapshotV1Arguments,
    source: &[u8],
    staging_file: &mut File,
) -> Result<FundsBuildCanonicalCSVSnapshotV1Result> {
    let bootstrap_parent = canonical_bootstrap_parent()?;
    run_funds_build_canonical_csv_snapshot_v1_in(arguments, source, staging_file, &bootstrap_parent)
}

fn run_funds_build_canonical_csv_snapshot_v1_in(
    arguments: FundsBuildCanonicalCSVSnapshotV1Arguments,
    source: &[u8],
    staging_file: &mut File,
    bootstrap_parent: &Path,
) -> Result<FundsBuildCanonicalCSVSnapshotV1Result> {
    validate_funds_build_canonical_csv_snapshot_v1_arguments(&arguments, &arguments.case_id)?;
    validate_source_envelope(source)?;
    let source_sha256 = format!("{:x}", Sha256::digest(source));
    let first = walk_canonical_source(source, |_ordinal, _record, _row_hash| Ok(()))?;

    canonical_output_descriptor_path(staging_file)?;
    let mut bootstrap = BootstrapDatabase::create_namespace(bootstrap_parent)
        .context("funds_canonical_csv_snapshot_database_failed")?;
    let mut connection = open_staging_connection(&bootstrap.path)
        .context("funds_canonical_csv_snapshot_database_failed")?;
    bootstrap
        .acquire_live_database()
        .context("funds_canonical_csv_snapshot_database_failed")?;
    create_source_schema(&connection).context("funds_canonical_csv_snapshot_database_failed")?;
    let transaction = connection
        .transaction()
        .context("funds_canonical_csv_snapshot_database_failed")?;
    insert_import_record(
        &transaction,
        &arguments,
        &source_sha256,
        source.len() as u64,
        first.row_count,
    )
    .context("funds_canonical_csv_snapshot_database_failed")?;
    let second = insert_source_rows(&transaction, source, &arguments)
        .context("funds_canonical_csv_snapshot_database_failed")?;
    if first != second {
        bail!("funds_canonical_csv_snapshot_source_replay_failed");
    }
    transaction
        .commit()
        .context("funds_canonical_csv_snapshot_database_failed")?;
    if !bootstrap
        .verify_live_inventory()
        .context("funds_canonical_csv_snapshot_database_failed")?
    {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    let materialization = run_funds_materialize_txn_daily_v1_on_connection(
        FundsMaterializeTxnDailyV1Arguments {
            case_id: arguments.case_id.clone(),
            source_revision: arguments.source_revision as i64,
            source_row_count: first.row_count as i64,
            source_max_txn_ts: first.max_txn_ts.clone(),
            source_max_id: first.row_count as i64,
            raw_artifact_manifest_sha256: arguments.raw_artifact_manifest_sha256,
        },
        &bootstrap.path,
        &connection,
    )
    .context("funds_canonical_csv_snapshot_materialization_failed")?;
    bootstrap
        .verify_live_inventory()
        .context("funds_canonical_csv_snapshot_materialization_failed")?;

    connection
        .execute_batch("CHECKPOINT")
        .context("funds_canonical_csv_snapshot_checkpoint_failed")?;
    bootstrap
        .verify_live_inventory()
        .context("funds_canonical_csv_snapshot_checkpoint_failed")?;
    drop(connection);
    bootstrap
        .verify_quiescent_inventory()
        .context("funds_canonical_csv_snapshot_checkpoint_failed")?;
    bootstrap
        .install_completed_output(staging_file)
        .context("funds_canonical_csv_snapshot_checkpoint_failed")?;

    Ok(FundsBuildCanonicalCSVSnapshotV1Result {
        schema_version: SCHEMA_VERSION,
        operation: FUNDS_BUILD_CANONICAL_CSV_SNAPSHOT_V1_COMMAND.to_string(),
        profile: arguments.profile,
        private_import_file_id: arguments.private_import_file_id,
        source_artifact_sha256: source_sha256,
        source_artifact_byte_length: source.len() as u64,
        source_row_count: first.row_count,
        source_max_txn_ts: first.max_txn_ts,
        source_max_id: first.row_count,
        row_hash_contract: ROW_HASH_CONTRACT.to_string(),
        materialization,
    })
}

fn validate_source_envelope(source: &[u8]) -> Result<()> {
    if source.is_empty()
        || source.len() > FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1
        || !std::str::from_utf8(source).is_ok()
        || source.contains(&0)
        || source.starts_with(&[0xef, 0xbb, 0xbf])
    {
        bail!("funds_canonical_csv_snapshot_source_invalid");
    }
    Ok(())
}

fn walk_canonical_source<F>(source: &[u8], mut visit: F) -> Result<SourceStats>
where
    F: FnMut(u64, &ValidatedRecord, &str) -> Result<()>,
{
    validate_source_envelope(source)?;
    let mut reader = ReaderBuilder::new()
        .has_headers(false)
        .flexible(false)
        .double_quote(true)
        .quoting(true)
        .trim(csv::Trim::None)
        .from_reader(source);
    let header = reader
        .records()
        .next()
        .transpose()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_header_invalid"))?
        .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_header_invalid"))?;
    if reader.position().byte() > MAX_HEADER_BYTES || !exact_header(&header) {
        bail!("funds_canonical_csv_snapshot_header_invalid");
    }

    let mut row_count = 0_u64;
    let mut max_txn_ts = String::new();
    for record in reader.records() {
        let record = record.map_err(|_| anyhow!("funds_canonical_csv_snapshot_record_invalid"))?;
        if row_count == FUNDS_CANONICAL_CSV_MAX_ROWS_V1 {
            bail!("funds_canonical_csv_snapshot_row_limit_exceeded");
        }
        row_count += 1;
        let validated = validate_record(record)?;
        let row_hash = raw_occurrence_digest(row_count, &validated.raw)?;
        if validated.txn_time > max_txn_ts {
            max_txn_ts = validated.txn_time.clone();
        }
        visit(row_count, &validated, &row_hash)?;
    }
    if row_count == 0 {
        bail!("funds_canonical_csv_snapshot_record_invalid");
    }
    Ok(SourceStats {
        row_count,
        max_txn_ts,
    })
}

fn exact_header(header: &StringRecord) -> bool {
    header.len() == COLUMNS.len()
        && header
            .iter()
            .zip(COLUMNS.iter())
            .all(|(actual, expected)| actual == expected.header)
}

fn validate_record(record: StringRecord) -> Result<ValidatedRecord> {
    if record.len() != COLUMNS.len() {
        bail!("funds_canonical_csv_snapshot_record_invalid");
    }
    let mut raw = Vec::with_capacity(COLUMNS.len());
    for value in record.iter() {
        if value.len() > MAX_CELL_BYTES
            || value != value.trim()
            || value.chars().any(forbidden_cell_character)
        {
            bail!("funds_canonical_csv_snapshot_cell_invalid");
        }
        raw.push((!value.is_empty()).then(|| value.to_string()));
    }
    let txn_time = required_value(&raw, TXN_TIME_INDEX)?;
    if !canonical_timestamp(txn_time) {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    let amount = required_value(&raw, AMOUNT_INDEX)?;
    if !canonical_decimal(amount) {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    if raw[BALANCE_INDEX]
        .as_deref()
        .is_some_and(|value| !canonical_decimal(value))
    {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    let direction = required_value(&raw, DIRECTION_INDEX)?;
    if direction != "进" && direction != "出" {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    if required_value(&raw, CURRENCY_INDEX)? != "CNY" {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    let card = raw[CARD_INDEX].as_deref();
    let account = raw[ACCOUNT_INDEX].as_deref();
    if card.is_none() && account.is_none()
        || card.is_some_and(|value| !canonical_account_identity(value))
        || account.is_some_and(|value| !canonical_account_identity(value))
    {
        bail!("funds_canonical_csv_snapshot_normalization_invalid");
    }
    Ok(ValidatedRecord {
        txn_time: txn_time.to_string(),
        amount: amount.to_string(),
        balance: raw[BALANCE_INDEX].clone(),
        direction: direction.to_string(),
        card: raw[CARD_INDEX].clone(),
        account: raw[ACCOUNT_INDEX].clone(),
        raw,
    })
}

fn required_value(values: &[Option<String>], index: usize) -> Result<&str> {
    values
        .get(index)
        .and_then(Option::as_deref)
        .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_normalization_invalid"))
}

fn canonical_timestamp(value: &str) -> bool {
    value.len() == 19
        && NaiveDateTime::parse_from_str(value, "%Y-%m-%d %H:%M:%S")
            .ok()
            .is_some_and(|parsed| parsed.format("%Y-%m-%d %H:%M:%S").to_string() == value)
}

fn canonical_decimal(value: &str) -> bool {
    if value.is_empty() || value.len() > MAX_DECIMAL_BYTES || value.starts_with('+') {
        return false;
    }
    let unsigned = value.strip_prefix('-').unwrap_or(value);
    if unsigned.is_empty() || value == "-0" {
        return false;
    }
    let mut parts = unsigned.split('.');
    let integer = parts.next().unwrap_or_default();
    let fraction = parts.next();
    if parts.next().is_some()
        || integer.is_empty()
        || !integer.bytes().all(|byte| byte.is_ascii_digit())
        || integer.len() > 1 && integer.starts_with('0')
    {
        return false;
    }
    match fraction {
        None => true,
        Some(fraction) => {
            !fraction.is_empty()
                && fraction.len() <= 2
                && fraction.bytes().all(|byte| byte.is_ascii_digit())
                && !fraction.ends_with('0')
        }
    }
}

fn canonical_account_identity(value: &str) -> bool {
    !value.is_empty()
        && !value.contains(['-', '_'])
        && !value.chars().any(char::is_whitespace)
        && !value.chars().any(forbidden_cell_character)
}

fn forbidden_cell_character(character: char) -> bool {
    character.is_control() || is_unicode_format_character(character)
}

// Unicode general category Cf. Keeping the closed table here avoids a parser
// dependency whose Unicode-data version could drift independently of Go's
// authoritative raw replay.
fn is_unicode_format_character(character: char) -> bool {
    matches!(
        character as u32,
        0x00ad | 0x061c | 0x06dd | 0x070f | 0x08e2 | 0x180e | 0xfeff | 0x110bd | 0x110cd | 0xe0001
    ) || matches!(
        character as u32,
        0x0600..=0x0605
            | 0x0890..=0x0891
            | 0x200b..=0x200f
            | 0x202a..=0x202e
            | 0x2060..=0x2064
            | 0x2066..=0x206f
            | 0xfff9..=0xfffb
            | 0x13430..=0x1343f
            | 0x1bca0..=0x1bca3
            | 0x1d173..=0x1d17a
            | 0xe0020..=0xe007f
    )
}

fn raw_occurrence_digest(ordinal: u64, raw: &[Option<String>]) -> Result<String> {
    if raw.len() != COLUMNS.len() || ordinal == 0 || ordinal > FUNDS_CANONICAL_CSV_MAX_ROWS_V1 {
        bail!("funds_canonical_csv_snapshot_row_hash_failed");
    }
    let mut fields = COLUMNS
        .iter()
        .zip(raw.iter())
        .map(|(column, value)| RawTypedField {
            name: column.field,
            scalar: RawTypedScalar {
                kind: if value.is_some() { "text" } else { "null" },
                value: value.as_deref().unwrap_or_default(),
            },
        })
        .collect::<Vec<_>>();
    fields.sort_by(|left, right| left.name.cmp(right.name));
    let serde_json = serde_json::to_vec(&RawOccurrence {
        occurrence_ordinal: ordinal,
        fields: &fields,
    })
    .map_err(|_| anyhow!("funds_canonical_csv_snapshot_row_hash_failed"))?;
    let canonical_json = go_compatible_json(serde_json);
    let mut hasher = Sha256::new();
    hasher.update(ROW_HASH_DOMAIN);
    hasher.update(canonical_json);
    Ok(format!("{:x}", hasher.finalize()))
}

fn go_compatible_json(value: Vec<u8>) -> Vec<u8> {
    let mut output = Vec::with_capacity(value.len());
    let mut index = 0;
    while index < value.len() {
        match value[index] {
            b'<' => output.extend_from_slice(b"\\u003c"),
            b'>' => output.extend_from_slice(b"\\u003e"),
            b'&' => output.extend_from_slice(b"\\u0026"),
            0xe2 if value.get(index..index + 3) == Some(&[0xe2, 0x80, 0xa8]) => {
                output.extend_from_slice(b"\\u2028");
                index += 3;
                continue;
            }
            0xe2 if value.get(index..index + 3) == Some(&[0xe2, 0x80, 0xa9]) => {
                output.extend_from_slice(b"\\u2029");
                index += 3;
                continue;
            }
            byte => output.push(byte),
        }
        index += 1;
    }
    output
}

fn open_staging_connection(path: &Path) -> Result<Connection> {
    Connection::open_with_flags(
        path,
        Config::default()
            .with("temp_directory", "")?
            .with("max_temp_directory_size", "0B")?
            .max_memory("512MB")?
            .threads(2)?
            .enable_autoload_extension(false)?
            .with("preserve_insertion_order", "false")?
            .access_mode(AccessMode::ReadWrite)?
            .enable_external_access(false)?,
    )
    .map_err(Into::into)
}

#[cfg(unix)]
fn canonical_output_descriptor_path(staging_file: &File) -> Result<PathBuf> {
    let fd = staging_file.as_raw_fd();
    let status_flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    let metadata = staging_file
        .metadata()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
    if fd <= 2
        || status_flags < 0
        || status_flags & libc::O_ACCMODE != libc::O_RDWR
        || !metadata.file_type().is_file()
        || metadata.nlink() != 0
        || metadata.uid() != unsafe { libc::geteuid() }
        || metadata.gid() != unsafe { libc::getegid() }
        || metadata.mode() & 0o7777 != 0o600
        || metadata.len() != 0
    {
        bail!("funds_canonical_csv_snapshot_output_invalid");
    }
    require_macos_descriptor_basename(fd)?;
    Ok(PathBuf::from(format!("/dev/fd/{fd}")))
}

#[cfg(not(unix))]
fn canonical_output_descriptor_path(_staging_file: &File) -> Result<PathBuf> {
    bail!("funds_canonical_csv_snapshot_output_unavailable")
}

#[cfg(target_os = "macos")]
fn require_macos_descriptor_basename(fd: libc::c_int) -> Result<()> {
    let mut path = [0 as libc::c_char; libc::PATH_MAX as usize];
    if unsafe { libc::fcntl(fd, libc::F_GETPATH, path.as_mut_ptr()) } < 0 {
        bail!("funds_canonical_csv_snapshot_output_invalid");
    }
    let length = path
        .iter()
        .position(|value| *value == 0)
        .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
    let bytes = path[..length]
        .iter()
        .map(|value| *value as u8)
        .collect::<Vec<_>>();
    let former = Path::new(
        std::str::from_utf8(&bytes)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?,
    );
    let expected_basename = fd.to_string();
    if former.file_name().and_then(|value| value.to_str()) != Some(expected_basename.as_str()) {
        bail!("funds_canonical_csv_snapshot_output_invalid");
    }
    Ok(())
}

#[cfg(all(unix, not(target_os = "macos")))]
fn require_macos_descriptor_basename(_fd: libc::c_int) -> Result<()> {
    Ok(())
}

#[cfg(unix)]
struct BootstrapDatabase {
    parent: PathBuf,
    directory: PathBuf,
    path: PathBuf,
    wal_path: PathBuf,
    file: Option<File>,
    parent_device: u64,
    parent_inode: u64,
    directory_device: u64,
    directory_inode: u64,
    device: u64,
    inode: u64,
    directory_linked: bool,
}

#[cfg(unix)]
impl BootstrapDatabase {
    fn create_namespace(parent: &Path) -> Result<Self> {
        if !parent.is_absolute() {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        let parent = parent.to_path_buf();
        let parent_metadata = fs::symlink_metadata(&parent)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_directory_metadata(&parent_metadata) {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        for _ in 0..16 {
            let name = random_bootstrap_directory_name()?;
            let directory = parent.join(name);
            let created = DirBuilder::new().mode(0o700).create(&directory);
            match created {
                Ok(()) => {}
                Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => continue,
                Err(_) => bail!("funds_canonical_csv_snapshot_database_failed"),
            }
            let metadata = fs::symlink_metadata(&directory)
                .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
            let current_parent = fs::symlink_metadata(&parent)
                .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
            if !canonical_bootstrap_directory_metadata(&metadata)
                || !same_bootstrap_directory_object(&parent_metadata, &current_parent)
                || fs::read_dir(&directory)
                    .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
                    .next()
                    .is_some()
            {
                bail!("funds_canonical_csv_snapshot_database_failed");
            }
            sync_bootstrap_directory(&parent, current_parent.dev(), current_parent.ino())?;
            sync_bootstrap_directory(&directory, metadata.dev(), metadata.ino())?;
            let path = directory.join("bootstrap.duckdb");
            let wal_path = directory.join("bootstrap.duckdb.wal");
            return Ok(Self {
                parent,
                path,
                wal_path,
                directory,
                file: None,
                parent_device: current_parent.dev(),
                parent_inode: current_parent.ino(),
                directory_device: metadata.dev(),
                directory_inode: metadata.ino(),
                device: 0,
                inode: 0,
                directory_linked: true,
            });
        }
        bail!("funds_canonical_csv_snapshot_database_failed")
    }

    fn acquire_live_database(&mut self) -> Result<()> {
        self.verify_parent()?;
        self.verify_directory()?;
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .custom_flags(libc::O_NOFOLLOW | libc::O_CLOEXEC)
            .open(&self.path)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        file.set_permissions(fs::Permissions::from_mode(0o600))
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        file.sync_all()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let metadata = file
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if metadata.len() == 0
            || metadata.len() > MAX_CANONICAL_DATABASE_BYTES
            || !canonical_bootstrap_metadata(&metadata, metadata.len(), 1)
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.device = metadata.dev();
        self.inode = metadata.ino();
        self.file = Some(file);
        self.verify_database_binding(metadata.len())?;
        self.verify_live_inventory()?;
        Ok(())
    }

    fn verify_parent(&self) -> Result<()> {
        let metadata = fs::symlink_metadata(&self.parent)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_directory_metadata(&metadata)
            || metadata.dev() != self.parent_device
            || metadata.ino() != self.parent_inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        Ok(())
    }

    fn verify_directory(&self) -> Result<()> {
        let metadata = fs::symlink_metadata(&self.directory)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_directory_metadata(&metadata)
            || metadata.dev() != self.directory_device
            || metadata.ino() != self.directory_inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.verify_parent()?;
        Ok(())
    }

    fn verify_database_binding(&self, expected_size: u64) -> Result<()> {
        let opened = self
            .file
            .as_ref()
            .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let linked = fs::symlink_metadata(&self.path)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_metadata(&opened, expected_size, 1)
            || !canonical_bootstrap_metadata(&linked, expected_size, 1)
            || opened.dev() != self.device
            || opened.ino() != self.inode
            || linked.dev() != self.device
            || linked.ino() != self.inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.verify_directory()?;
        Ok(())
    }

    fn verify_live_inventory(&self) -> Result<bool> {
        self.verify_directory()?;
        let entries = fs::read_dir(&self.directory)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .collect::<std::result::Result<Vec<_>, _>>()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let database_name = self.path.file_name().unwrap_or_default();
        let wal_name = self.wal_path.file_name().unwrap_or_default();
        let mut database_seen = false;
        let mut wal_seen = false;
        for entry in entries {
            let name = entry.file_name();
            if name == database_name && !database_seen {
                database_seen = true;
            } else if name == wal_name && !wal_seen {
                wal_seen = true;
                secure_live_wal(&self.wal_path)?;
            } else {
                bail!("funds_canonical_csv_snapshot_database_failed");
            }
        }
        let size = self
            .file
            .as_ref()
            .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .len();
        if !database_seen || size == 0 || size > MAX_CANONICAL_DATABASE_BYTES {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.verify_database_binding(size)?;
        let mut final_names = fs::read_dir(&self.directory)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .map(|entry| entry.map(|value| value.file_name()))
            .collect::<std::result::Result<Vec<_>, _>>()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        final_names.sort();
        let mut expected_names = vec![database_name.to_os_string()];
        if wal_seen {
            expected_names.push(wal_name.to_os_string());
        }
        expected_names.sort();
        if final_names != expected_names {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.verify_directory()?;
        Ok(wal_seen)
    }

    fn verify_quiescent_inventory(&self) -> Result<()> {
        if self.verify_live_inventory()? {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        Ok(())
    }

    fn prepare_completed_unlink(&mut self) -> Result<(u64, [u8; 32])> {
        self.verify_quiescent_inventory()?;
        let file = self
            .file
            .as_mut()
            .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        file.sync_all()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let metadata = file
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let expected_size = metadata.len();
        if expected_size == 0 || expected_size > MAX_CANONICAL_DATABASE_BYTES {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        let expected_sha256 = hash_bootstrap_file(file, expected_size)?;
        self.verify_database_binding(expected_size)?;
        fs::remove_file(&self.path)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let metadata = self
            .file
            .as_ref()
            .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_metadata(&metadata, expected_size, 0)
            || metadata.dev() != self.device
            || metadata.ino() != self.inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        self.verify_directory()?;
        if fs::read_dir(&self.directory)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?
            .next()
            .is_some()
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        sync_bootstrap_directory(&self.directory, self.directory_device, self.directory_inode)?;
        fs::remove_dir(&self.directory)
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        self.directory_linked = false;
        self.verify_parent()?;
        sync_bootstrap_directory(&self.parent, self.parent_device, self.parent_inode)?;
        Ok((expected_size, expected_sha256))
    }

    fn copy_unlinked_to(
        &mut self,
        staging_file: &mut File,
        expected_size: u64,
        expected_sha256: [u8; 32],
    ) -> Result<()> {
        let source = self
            .file
            .as_mut()
            .ok_or_else(|| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let metadata = source
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_metadata(&metadata, expected_size, 0)
            || metadata.dev() != self.device
            || metadata.ino() != self.inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        if hash_bootstrap_file(source, expected_size)? != expected_sha256 {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        source.seek(SeekFrom::Start(0))?;
        staging_file.seek(SeekFrom::Start(0))?;
        staging_file.set_len(0)?;
        let mut copied = 0_u64;
        let mut hasher = Sha256::new();
        let mut buffer = [0_u8; 256 * 1024];
        while copied < expected_size {
            let remaining = expected_size - copied;
            let limit = usize::try_from(remaining.min(buffer.len() as u64))
                .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
            let read = source.read(&mut buffer[..limit])?;
            if read == 0 {
                bail!("funds_canonical_csv_snapshot_database_failed");
            }
            hasher.update(&buffer[..read]);
            staging_file.write_all(&buffer[..read])?;
            copied += read as u64;
        }
        let mut extra = [0_u8; 1];
        if source.read(&mut extra)? != 0 || copied != expected_size {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        let copied_sha256: [u8; 32] = hasher.finalize().into();
        if copied_sha256 != expected_sha256
            || hash_bootstrap_file(source, expected_size)? != expected_sha256
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        let source_metadata = source
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        if !canonical_bootstrap_metadata(&source_metadata, expected_size, 0)
            || source_metadata.dev() != self.device
            || source_metadata.ino() != self.inode
        {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        staging_file.flush()?;
        staging_file.sync_all()?;
        let output_metadata = staging_file
            .metadata()
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_output_invalid"))?;
        if !canonical_bootstrap_metadata(&output_metadata, expected_size, 0) {
            bail!("funds_canonical_csv_snapshot_output_invalid");
        }
        Ok(())
    }

    fn install_completed_output(&mut self, staging_file: &mut File) -> Result<()> {
        let (expected_size, expected_sha256) = self.prepare_completed_unlink()?;
        self.copy_unlinked_to(staging_file, expected_size, expected_sha256)
    }
}

#[cfg(unix)]
impl Drop for BootstrapDatabase {
    fn drop(&mut self) {
        if self.directory_linked && self.verify_directory().is_ok() {
            cleanup_bootstrap_entry(&self.wal_path, None);
            let expected = self
                .file
                .as_ref()
                .and_then(|file| file.metadata().ok())
                .map(|metadata| (metadata.dev(), metadata.ino()));
            cleanup_bootstrap_entry(&self.path, expected);
        }
        if self.directory_linked
            && self.verify_directory().is_ok()
            && fs::read_dir(&self.directory)
                .ok()
                .is_some_and(|mut entries| entries.next().is_none())
        {
            let _ = fs::remove_dir(&self.directory);
        }
    }
}

#[cfg(unix)]
fn canonical_bootstrap_parent() -> Result<PathBuf> {
    // The native process authority pins and sets this directory from the
    // host-owned WorkingDirectoryAuthority before exec. Never consult TMPDIR:
    // a linked DuckDB and its live WAL are permitted only inside that existing
    // protected working authority, while the canonical FD 5 output stays
    // unlinked for its entire lifetime.
    let parent = std::env::current_dir()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    let metadata = fs::symlink_metadata(&parent)
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    if !parent.is_absolute() || !canonical_bootstrap_directory_metadata(&metadata) {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    Ok(parent)
}

#[cfg(not(unix))]
fn canonical_bootstrap_parent() -> Result<PathBuf> {
    bail!("funds_canonical_csv_snapshot_output_unavailable")
}

#[cfg(unix)]
fn secure_live_wal(path: &Path) -> Result<()> {
    let file = OpenOptions::new()
        .read(true)
        .write(true)
        .custom_flags(libc::O_NOFOLLOW | libc::O_CLOEXEC)
        .open(path)
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    file.set_permissions(fs::Permissions::from_mode(0o600))
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    let metadata = file
        .metadata()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    let linked = fs::symlink_metadata(path)
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    if !canonical_bootstrap_metadata(&metadata, metadata.len(), 1)
        || !canonical_bootstrap_metadata(&linked, metadata.len(), 1)
        || linked.dev() != metadata.dev()
        || linked.ino() != metadata.ino()
        || metadata.len() > MAX_CANONICAL_DATABASE_BYTES
    {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    Ok(())
}

#[cfg(unix)]
fn random_bootstrap_directory_name() -> Result<String> {
    let mut random = [0_u8; 32];
    if unsafe { libc::getentropy(random.as_mut_ptr().cast(), random.len()) } != 0 {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    Ok(format!(
        ".analytix-duckdb-bootstrap-{}",
        &format!("{:x}", Sha256::digest(random))[..32]
    ))
}

#[cfg(unix)]
fn sync_bootstrap_directory(path: &Path, expected_device: u64, expected_inode: u64) -> Result<()> {
    let directory = OpenOptions::new()
        .read(true)
        .custom_flags(libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC)
        .open(path)
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    let before = directory
        .metadata()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    if !canonical_bootstrap_directory_metadata(&before)
        || before.dev() != expected_device
        || before.ino() != expected_inode
    {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    directory
        .sync_all()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    let after = directory
        .metadata()
        .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
    if !same_bootstrap_directory_object(&before, &after) {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    Ok(())
}

#[cfg(unix)]
fn hash_bootstrap_file(file: &mut File, expected_size: u64) -> Result<[u8; 32]> {
    file.seek(SeekFrom::Start(0))?;
    let mut hasher = Sha256::new();
    let mut read_total = 0_u64;
    let mut buffer = [0_u8; 256 * 1024];
    while read_total < expected_size {
        let remaining = expected_size - read_total;
        let limit = usize::try_from(remaining.min(buffer.len() as u64))
            .map_err(|_| anyhow!("funds_canonical_csv_snapshot_database_failed"))?;
        let read = file.read(&mut buffer[..limit])?;
        if read == 0 {
            bail!("funds_canonical_csv_snapshot_database_failed");
        }
        hasher.update(&buffer[..read]);
        read_total += read as u64;
    }
    let mut extra = [0_u8; 1];
    if file.read(&mut extra)? != 0 {
        bail!("funds_canonical_csv_snapshot_database_failed");
    }
    Ok(hasher.finalize().into())
}

#[cfg(unix)]
fn cleanup_bootstrap_entry(path: &Path, expected: Option<(u64, u64)>) {
    let Ok(metadata) = fs::symlink_metadata(path) else {
        return;
    };
    let exact = metadata.file_type().is_file()
        && metadata.uid() == unsafe { libc::geteuid() }
        && metadata.gid() == unsafe { libc::getegid() }
        && metadata.nlink() == 1
        && expected
            .is_none_or(|(device, inode)| metadata.dev() == device && metadata.ino() == inode);
    if exact {
        let _ = fs::remove_file(path);
    }
}

#[cfg(unix)]
fn canonical_bootstrap_metadata(metadata: &fs::Metadata, size: u64, links: u64) -> bool {
    metadata.file_type().is_file()
        && metadata.uid() == unsafe { libc::geteuid() }
        && metadata.gid() == unsafe { libc::getegid() }
        && metadata.nlink() == links
        && metadata.mode() & 0o7777 == 0o600
        && metadata.len() == size
}

#[cfg(unix)]
fn canonical_bootstrap_directory_metadata(metadata: &fs::Metadata) -> bool {
    metadata.file_type().is_dir()
        && metadata.uid() == unsafe { libc::geteuid() }
        && metadata.gid() == unsafe { libc::getegid() }
        && metadata.mode() & 0o7777 == 0o700
}

#[cfg(unix)]
fn same_bootstrap_directory_object(left: &fs::Metadata, right: &fs::Metadata) -> bool {
    canonical_bootstrap_directory_metadata(left)
        && canonical_bootstrap_directory_metadata(right)
        && left.dev() == right.dev()
        && left.ino() == right.ino()
}

#[cfg(not(unix))]
struct BootstrapDatabase {
    path: PathBuf,
}

#[cfg(not(unix))]
impl BootstrapDatabase {
    fn create_namespace(_parent: &Path) -> Result<Self> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }

    fn acquire_live_database(&mut self) -> Result<()> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }

    fn verify_live_inventory(&self) -> Result<bool> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }

    fn verify_quiescent_inventory(&self) -> Result<()> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }

    fn install_completed_output(&mut self, _staging_file: &mut File) -> Result<()> {
        bail!("funds_canonical_csv_snapshot_output_unavailable")
    }
}

fn create_source_schema(connection: &Connection) -> Result<()> {
    let raw_columns = COLUMNS
        .iter()
        .map(|column| format!("{} VARCHAR", column.raw_column))
        .collect::<Vec<_>>()
        .join(", ");
    connection.execute_batch(&format!(
        "CREATE TABLE analysis_revision_state(revision_key VARCHAR, revision BIGINT); \
         CREATE TABLE import_file_log( \
           file_id VARCHAR, case_id VARCHAR, kind VARCHAR, stored_path VARCHAR, \
           file_type VARCHAR, size BIGINT, sha256 VARCHAR, rows_total BIGINT, \
           rows_imported BIGINT, rows_imported_raw BIGINT, rows_imported_norm BIGINT, \
           rows_dedup BIGINT, rows_error BIGINT, rows_skipped_non_data BIGINT, \
           import_counts_version BIGINT, status VARCHAR, error VARCHAR, \
           cleaned_status VARCHAR, cleaned_error VARCHAR, cleaning_counts_version BIGINT \
         ); \
         CREATE TABLE fc_transaction_raw( \
           id BIGINT, case_id VARCHAR, file_id VARCHAR, row_no BIGINT, \
           row_hash VARCHAR, extra_json VARCHAR, {raw_columns} \
         ); \
         CREATE TABLE fc_transaction_norm( \
           id BIGINT, case_id VARCHAR, file_id VARCHAR, row_no BIGINT, row_hash VARCHAR, \
           txn_ts TIMESTAMP, clean_amount VARCHAR, clean_balance VARCHAR, \
           clean_dc_flag VARCHAR, clean_card_no VARCHAR, clean_acct_no VARCHAR, \
           clean_invalid INTEGER, clean_failed INTEGER, clean_reversal INTEGER, \
           clean_duplicate INTEGER \
         );"
    ))?;
    Ok(())
}

fn insert_import_record(
    connection: &Connection,
    arguments: &FundsBuildCanonicalCSVSnapshotV1Arguments,
    source_sha256: &str,
    source_bytes: u64,
    source_rows: u64,
) -> Result<()> {
    connection.execute(
        "INSERT INTO analysis_revision_state VALUES ('stats_flow_source', ?)",
        [arguments.source_revision as i64],
    )?;
    connection.execute(
        "INSERT INTO import_file_log VALUES ( \
           ?, ?, 'fc_transaction', '', 'CSV', ?, ?, ?, ?, ?, ?, \
           0, 0, 0, 1, '已完成', '', 'done', '', 1 \
         )",
        duckdb::params![
            arguments.private_import_file_id,
            arguments.case_id,
            source_bytes as i64,
            source_sha256,
            source_rows as i64,
            source_rows as i64,
            source_rows as i64,
            source_rows as i64,
        ],
    )?;
    Ok(())
}

fn insert_source_rows(
    connection: &Connection,
    source: &[u8],
    arguments: &FundsBuildCanonicalCSVSnapshotV1Arguments,
) -> Result<SourceStats> {
    let raw_columns = COLUMNS
        .iter()
        .map(|column| column.raw_column)
        .collect::<Vec<_>>()
        .join(", ");
    let raw_placeholders = (0..(6 + COLUMNS.len()))
        .map(|_| "?")
        .collect::<Vec<_>>()
        .join(", ");
    let raw_sql = format!(
        "INSERT INTO fc_transaction_raw( \
           id, case_id, file_id, row_no, row_hash, extra_json, {raw_columns} \
         ) VALUES ({raw_placeholders})"
    );
    let norm_sql = "INSERT INTO fc_transaction_norm VALUES ( \
        ?, ?, ?, ?, ?, CAST(? AS TIMESTAMP), ?, ?, ?, ?, ?, 0, 0, 0, 0 \
    )";
    let mut raw_statement = connection.prepare(&raw_sql)?;
    let mut norm_statement = connection.prepare(norm_sql)?;
    walk_canonical_source(source, |ordinal, record, row_hash| {
        let mut raw_values = Vec::with_capacity(6 + COLUMNS.len());
        raw_values.push(DuckValue::BigInt(ordinal as i64));
        raw_values.push(DuckValue::Text(arguments.case_id.clone()));
        raw_values.push(DuckValue::Text(arguments.private_import_file_id.clone()));
        raw_values.push(DuckValue::BigInt(ordinal as i64));
        raw_values.push(DuckValue::Text(row_hash.to_string()));
        raw_values.push(DuckValue::Null);
        raw_values.extend(record.raw.iter().map(|value| match value {
            Some(value) => DuckValue::Text(value.clone()),
            None => DuckValue::Null,
        }));
        raw_statement.execute(params_from_iter(raw_values.iter()))?;

        let norm_values = vec![
            DuckValue::BigInt(ordinal as i64),
            DuckValue::Text(arguments.case_id.clone()),
            DuckValue::Text(arguments.private_import_file_id.clone()),
            DuckValue::BigInt(ordinal as i64),
            DuckValue::Text(row_hash.to_string()),
            DuckValue::Text(record.txn_time.clone()),
            DuckValue::Text(record.amount.clone()),
            record
                .balance
                .as_ref()
                .map_or(DuckValue::Null, |value| DuckValue::Text(value.clone())),
            DuckValue::Text(record.direction.clone()),
            record
                .card
                .as_ref()
                .map_or(DuckValue::Null, |value| DuckValue::Text(value.clone())),
            record
                .account
                .as_ref()
                .map_or(DuckValue::Null, |value| DuckValue::Text(value.clone())),
        ];
        norm_statement.execute(params_from_iter(norm_values.iter()))?;
        Ok(())
    })
}

fn valid_private_text(value: &str, maximum: usize) -> bool {
    !value.is_empty()
        && value.len() <= maximum
        && value == value.trim()
        && !value.chars().any(forbidden_cell_character)
}

fn valid_lower_hex(value: &str, length: usize) -> bool {
    value.len() == length
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

#[cfg(test)]
mod tests {
    include!("funds_delivery_vectors_test.rs");
    use super::*;
    use duckdb::Connection;
    use std::fs::{self, OpenOptions};
    #[cfg(unix)]
    use std::os::fd::AsRawFd;
    #[cfg(unix)]
    use std::os::unix::fs::{MetadataExt, OpenOptionsExt};
    use std::path::PathBuf;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn header() -> String {
        COLUMNS
            .iter()
            .map(|column| column.header)
            .collect::<Vec<_>>()
            .join(",")
    }

    fn row() -> Vec<String> {
        let mut values = vec![String::new(); COLUMNS.len()];
        values[CARD_INDEX] = "6222021234567890123".to_string();
        values[ACCOUNT_INDEX] = "1000000000000000001".to_string();
        values[TXN_TIME_INDEX] = "2026-01-02 03:04:05".to_string();
        values[AMOUNT_INDEX] = "-12.5".to_string();
        values[BALANCE_INDEX] = "100.01".to_string();
        values[DIRECTION_INDEX] = "出".to_string();
        values[CURRENCY_INDEX] = "CNY".to_string();
        values[10] = "完整敏感姓名".to_string();
        values
    }

    fn go_cross_language_row() -> Vec<String> {
        [
            "6222021234567890123",
            "9558800012345678901",
            "张三",
            "110101199001010011",
            "2026-07-25 10:11:12",
            "100.25",
            "2000.5",
            "进",
            "6217009876543210987",
            "否",
            "李四",
            "110101198801010022",
            "中国银行",
            "往来款",
            "CNY",
            "北京分行",
            "1100",
            "北京",
            "成功",
            "voucher-1",
            "terminal-1",
            "192.0.2.1",
            "00:11:22:33:44:55",
            "3000",
            "txn-0001",
            "log-0001",
            "电子凭证",
            "credential-1",
            "teller-1",
            "商户甲",
            "merchant-1",
            "备注甲",
            "转账",
            "查询成功",
        ]
        .into_iter()
        .map(str::to_string)
        .collect()
    }

    fn csv(values: &[String]) -> Vec<u8> {
        format!("{}\r\n{}\r\n", header(), values.join(",")).into_bytes()
    }

    fn arguments() -> FundsBuildCanonicalCSVSnapshotV1Arguments {
        FundsBuildCanonicalCSVSnapshotV1Arguments {
            case_id: "case-a".to_string(),
            profile: FUNDS_CANONICAL_DIRECT_CSV_PROFILE_V1.to_string(),
            private_import_file_id: "0123456789abcdefabcd".to_string(),
            source_revision: 7,
            raw_artifact_manifest_sha256: "1".repeat(64),
        }
    }

    #[cfg(unix)]
    fn temp_unlinked_output(label: &str) -> (PathBuf, PathBuf, File) {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock")
            .as_nanos();
        let directory = std::env::temp_dir().join(format!(
            "analytix-canonical-stage-{label}-{}-{unique}",
            std::process::id()
        ));
        fs::create_dir(&directory).expect("create private test directory");
        fs::set_permissions(&directory, fs::Permissions::from_mode(0o700))
            .expect("secure private test directory");
        let provisional = directory.join("output");
        let mut options = OpenOptions::new();
        options.create_new(true).read(true).write(true);
        options.mode(0o600);
        let file = options.open(&provisional).expect("precreate output file");
        let former_path = directory.join(file.as_raw_fd().to_string());
        fs::rename(&provisional, &former_path).expect("bind descriptor basename");
        fs::remove_file(&former_path).expect("unlink exact output before staging");
        assert_eq!(file.metadata().expect("output metadata").nlink(), 0);
        (directory, former_path, file)
    }

    #[cfg(unix)]
    fn output_descriptor_path(file: &File) -> PathBuf {
        PathBuf::from(format!("/dev/fd/{}", file.as_raw_fd()))
    }

    #[cfg(unix)]
    #[test]
    fn bootstrap_rejects_the_process_wide_temporary_directory() {
        let ordinary_tmp = fs::canonicalize("/tmp").expect("resolve ordinary temporary root");
        assert!(BootstrapDatabase::create_namespace(&ordinary_tmp).is_err());
    }

    #[cfg(unix)]
    #[test]
    fn live_wal_is_the_only_admitted_bootstrap_sidecar() {
        let (directory, _former_path, output) = temp_unlinked_output("live-wal");
        let mut bootstrap = BootstrapDatabase::create_namespace(&directory)
            .expect("create protected bootstrap namespace");
        let connection = open_staging_connection(&bootstrap.path).expect("open bootstrap database");
        bootstrap
            .acquire_live_database()
            .expect("pin live bootstrap database");
        connection
            .execute_batch(
                "CREATE TABLE live_wal_inventory(value BIGINT); \
                 BEGIN TRANSACTION; \
                 INSERT INTO live_wal_inventory VALUES (1); \
                 COMMIT;",
            )
            .expect("commit WAL-backed bootstrap transaction");
        assert!(
            bootstrap
                .verify_live_inventory()
                .expect("inventory live bootstrap namespace"),
            "the pinned DuckDB build must expose its exact live WAL to inventory"
        );
        drop(connection);
        bootstrap
            .verify_quiescent_inventory()
            .expect("WAL retired after connection close");
        drop(bootstrap);
        drop(output);
        fs::remove_dir(directory).expect("remove empty test authority");
    }

    #[cfg(unix)]
    #[test]
    fn retained_writer_tamper_cannot_reach_canonical_output() {
        let (directory, _former_path, mut output) = temp_unlinked_output("retained-writer");
        let mut bootstrap = BootstrapDatabase::create_namespace(&directory)
            .expect("create protected bootstrap namespace");
        let connection = open_staging_connection(&bootstrap.path).expect("open bootstrap database");
        bootstrap
            .acquire_live_database()
            .expect("pin live bootstrap database");
        connection
            .execute_batch(
                "CREATE TABLE retained_writer(value BIGINT); \
                 INSERT INTO retained_writer VALUES (7); \
                 CHECKPOINT;",
            )
            .expect("build retained-writer fixture");
        drop(connection);
        bootstrap
            .verify_quiescent_inventory()
            .expect("fixture is quiescent");

        let mut retained = OpenOptions::new()
            .read(true)
            .write(true)
            .custom_flags(libc::O_NOFOLLOW | libc::O_CLOEXEC)
            .open(&bootstrap.path)
            .expect("simulate same-uid writer retained before unlink");
        let (expected_size, expected_sha256) = bootstrap
            .prepare_completed_unlink()
            .expect("seal and unlink exact bootstrap inode");
        retained
            .seek(SeekFrom::Start(0))
            .expect("seek retained writer");
        retained
            .write_all(b"ANALYTIX-ADVERSARIAL-TAMPER")
            .expect("tamper retained unlinked inode");
        retained.sync_all().expect("persist adversarial tamper");

        assert!(bootstrap
            .copy_unlinked_to(&mut output, expected_size, expected_sha256)
            .is_err());
        assert_eq!(
            output.metadata().expect("inspect rejected output").len(),
            0,
            "pre-copy tamper must not write any byte to the canonical output descriptor"
        );
        drop(retained);
        drop(bootstrap);
        drop(output);
        fs::remove_dir(directory).expect("remove empty test authority");
    }

    #[cfg(unix)]
    #[test]
    fn materialization_never_reopens_a_substituted_database_path() {
        let (directory, _former_path, output) = temp_unlinked_output("path-substitution");
        let build_arguments = arguments();
        let source = csv(&row());
        let source_sha256 = format!("{:x}", Sha256::digest(&source));
        let first = walk_canonical_source(&source, |_ordinal, _record, _row_hash| Ok(()))
            .expect("validate canonical source");
        let mut bootstrap = BootstrapDatabase::create_namespace(&directory)
            .expect("create protected bootstrap namespace");
        let mut connection =
            open_staging_connection(&bootstrap.path).expect("open bootstrap database");
        bootstrap
            .acquire_live_database()
            .expect("pin live bootstrap database");
        create_source_schema(&connection).expect("create source schema");
        let transaction = connection.transaction().expect("begin source transaction");
        insert_import_record(
            &transaction,
            &build_arguments,
            &source_sha256,
            source.len() as u64,
            first.row_count,
        )
        .expect("insert import record");
        let second = insert_source_rows(&transaction, &source, &build_arguments)
            .expect("insert source rows");
        assert_eq!(first, second);
        transaction.commit().expect("commit source transaction");

        let retained_path = bootstrap.directory.join("retained.duckdb");
        fs::rename(&bootstrap.path, &retained_path).expect("retain exact database inode");
        let mut replacement_options = OpenOptions::new();
        replacement_options.create_new(true).write(true).mode(0o600);
        let mut replacement = replacement_options
            .open(&bootstrap.path)
            .expect("install hostile path replacement");
        replacement
            .write_all(b"ANALYTIX-HOSTILE-DATABASE-REPLACEMENT")
            .expect("write hostile path replacement");
        replacement.sync_all().expect("sync hostile replacement");
        drop(replacement);

        let materialization = run_funds_materialize_txn_daily_v1_on_connection(
            FundsMaterializeTxnDailyV1Arguments {
                case_id: build_arguments.case_id.clone(),
                source_revision: build_arguments.source_revision as i64,
                source_row_count: first.row_count as i64,
                source_max_txn_ts: first.max_txn_ts.clone(),
                source_max_id: first.row_count as i64,
                raw_artifact_manifest_sha256: build_arguments.raw_artifact_manifest_sha256.clone(),
            },
            &bootstrap.path,
            &connection,
        )
        .expect("materialize only through the pinned connection");
        assert_eq!(materialization.row_count, 1);
        assert_eq!(
            fs::read(&bootstrap.path).expect("read hostile replacement"),
            b"ANALYTIX-HOSTILE-DATABASE-REPLACEMENT"
        );

        fs::remove_file(&bootstrap.path).expect("remove hostile replacement");
        fs::rename(&retained_path, &bootstrap.path).expect("restore exact database binding");
        bootstrap
            .verify_live_inventory()
            .expect("verify restored live inventory");
        connection
            .execute_batch("CHECKPOINT")
            .expect("checkpoint pinned connection");
        drop(connection);
        bootstrap
            .verify_quiescent_inventory()
            .expect("verify quiescent pinned database");
        drop(bootstrap);
        drop(output);
        fs::remove_dir(directory).expect("remove empty test authority");
    }

    #[cfg(unix)]
    #[test]
    fn canonical_csv_builds_exact_source_tables_and_path_free_result() {
        let (directory, former_path, mut output) = temp_unlinked_output("positive");
        let path = output_descriptor_path(&output);
        let source = csv(&row());
        let result = run_funds_build_canonical_csv_snapshot_v1_in(
            arguments(),
            &source,
            &mut output,
            &directory,
        )
        .expect("build canonical snapshot");
        assert_eq!(result.source_row_count, 1);
        assert_eq!(result.source_max_id, 1);
        assert_eq!(result.source_max_txn_ts, "2026-01-02 03:04:05");
        assert_eq!(
            result.operation,
            FUNDS_BUILD_CANONICAL_CSV_SNAPSHOT_V1_COMMAND
        );
        assert_eq!(result.materialization.row_count, 1);
        assert_eq!(
            result.materialization.schema_digest,
            "81a474553b16799c12d8d85c28d99f25ba6c9b40ae2073701443d2898f91e11e"
        );
        let encoded = serde_json::to_string(&result).expect("serialize result");
        assert!(!encoded.contains("6222021234567890123"));
        assert!(!encoded.contains("1000000000000000001"));
        assert!(!encoded.contains("完整敏感姓名"));
        assert!(!encoded.contains(former_path.to_string_lossy().as_ref()));
        let object = serde_json::to_value(&result)
            .expect("serialize typed result")
            .as_object()
            .expect("typed result object")
            .keys()
            .cloned()
            .collect::<std::collections::BTreeSet<_>>();
        let expected = [
            "aggName",
            "aggVersion",
            "caseId",
            "duckdbContentSnapshotDigest",
            "duckdbSnapshotManifestSha256",
            "materializationIdentity",
            "materializationIdentitySchemaVersion",
            "operation",
            "privateImportFileId",
            "producerContentId",
            "producerContentManifestBase64",
            "producerContentManifestByteLength",
            "producerContentManifestSha256",
            "profile",
            "rawArtifactManifestSha256",
            "rawSourceManifestBase64",
            "rawSourceManifestByteLength",
            "rawSourceManifestSha256",
            "rowCount",
            "rowHashContract",
            "schemaDigest",
            "schemaVersion",
            "sourceArtifactByteLength",
            "sourceArtifactSha256",
            "sourceMaxId",
            "sourceMaxTxnTs",
            "sourceRowCount",
        ]
        .into_iter()
        .map(str::to_string)
        .collect::<std::collections::BTreeSet<_>>();
        assert_eq!(object, expected, "typed result field set drifted");

        let connection = Connection::open(&path).expect("open completed snapshot");
        let row_state = connection
            .query_row(
                "SELECT r.id,r.row_no,r.row_hash,n.row_hash,n.clean_amount,n.clean_dc_flag, \
                        n.clean_invalid,n.clean_failed,n.clean_reversal,n.clean_duplicate \
                   FROM fc_transaction_raw r JOIN fc_transaction_norm n USING(case_id,file_id,row_no)",
                [],
                |row| {
                    Ok((
                        row.get::<_, i64>(0)?,
                        row.get::<_, i64>(1)?,
                        row.get::<_, String>(2)?,
                        row.get::<_, String>(3)?,
                        row.get::<_, String>(4)?,
                        row.get::<_, String>(5)?,
                        row.get::<_, i32>(6)?,
                        row.get::<_, i32>(7)?,
                        row.get::<_, i32>(8)?,
                        row.get::<_, i32>(9)?,
                    ))
                },
            )
            .expect("read canonical rows");
        assert_eq!(row_state.0, 1);
        assert_eq!(row_state.1, 1);
        assert_eq!(row_state.2, row_state.3);
        assert_eq!(row_state.2.len(), 64);
        assert_eq!(row_state.4, "-12.5");
        assert_eq!(row_state.5, "出");
        assert_eq!(
            (row_state.6, row_state.7, row_state.8, row_state.9),
            (0, 0, 0, 0)
        );
        let currency_state = connection
            .query_row(
                "SELECT r.currency_raw, d.currency \
                   FROM fc_transaction_raw r \
                   JOIN analysis_txn_detail_idx d ON d.id=r.id AND d.file_id=r.file_id",
                [],
                |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?)),
            )
            .expect("read canonical currency projection");
        assert_eq!(currency_state, ("CNY".to_string(), "CNY".to_string()));
        drop(connection);
        assert_eq!(
            fs::read_dir(&directory)
                .expect("read output directory")
                .count(),
            0
        );
        drop(output);
        fs::remove_dir(directory).expect("remove empty output directory");
    }

    #[cfg(unix)]
    #[test]
    fn sparse_staged_canonical_csv_build() {
        const PRIVATE_SENTINEL: &str = "AB_R2_PRIVATE_SOURCE_SENTINEL_9f07f00d_GENERATION_ONE";

        let (directory, _former_path, mut output) = temp_unlinked_output("ab-r2-sparse");
        assert_eq!(COLUMNS.len(), 34);
        let mut values = vec![String::new(); COLUMNS.len()];
        values[0] = "6222021234567890001".to_string();
        values[1] = "1000001".to_string();
        values[2] = "合成账户甲".to_string();
        values[3] = "SYNTHID0001".to_string();
        values[4] = "2026-08-27 10:00:00".to_string();
        values[5] = "12.5".to_string();
        values[6] = "1000".to_string();
        values[7] = "进".to_string();
        values[8] = "CP001".to_string();
        values[9] = "否".to_string();
        values[10] = "合成对手甲".to_string();
        values[11] = "SYNTHCPID001".to_string();
        values[12] = "合成银行".to_string();
        values[13] = "合成交易".to_string();
        values[14] = "CNY".to_string();
        values[31] = PRIVATE_SENTINEL.to_string();
        let mut source = csv(&values);

        let outcome = run_funds_build_canonical_csv_snapshot_v1_in(
            arguments(),
            &source,
            &mut output,
            &directory,
        );
        for value in &mut values {
            value.clear();
        }
        source.fill(0);

        let result = match outcome {
            Ok(result) => result,
            Err(_) => {
                drop(output);
                fs::remove_dir(directory).expect("remove empty sparse output directory");
                panic!("sparse staged canonical CSV build failed closed");
            }
        };

        assert_eq!(result.source_row_count, 1);
        assert_eq!(result.source_max_id, 1);
        assert_eq!(result.source_max_txn_ts, "2026-08-27 10:00:00");
        assert_eq!(result.materialization.row_count, 1);
        assert!(output.metadata().expect("sparse output metadata").len() > 0);
        let mut encoded = serde_json::to_vec(&result).expect("serialize sparse public result");
        assert!(!encoded
            .windows(PRIVATE_SENTINEL.len())
            .any(|window| window == PRIVATE_SENTINEL.as_bytes()));
        encoded.fill(0);
        drop(result);
        drop(output);
        fs::remove_dir(directory).expect("remove empty sparse output directory");
    }

    #[cfg(unix)]
    #[test]
    fn canonical_snapshot_runs_deterministic_cleaning_with_stable_exact_output() {
        let (directory, _former_path, mut snapshot) =
            temp_unlinked_output("deterministic-cleaning");
        let mut dirty_row = row();
        dirty_row[8] = "CP-001_ 23".to_string();
        let source = csv(&dirty_row);
        let built = run_funds_build_canonical_csv_snapshot_v1_in(
            arguments(),
            &source,
            &mut snapshot,
            &directory,
        )
        .expect("build canonical cleaning input");
        let rule_digest = format!(
            "{:x}",
            Sha256::digest(
                crate::stats_query_store::deterministic_cleaning::DETERMINISTIC_CLEANING_RULE_CONTRACT
                    .as_bytes()
            )
        );
        let mut generation = Sha256::new();
        generation.update(b"AnalytixFundsDeterministicCleaningGenerationV1\0");
        generation.update(rule_digest.as_bytes());
        let cleaning_arguments =
            crate::stats_query_store::deterministic_cleaning::DeterministicCleaningArguments {
                case_id: built.materialization.case_id.clone(),
                dataset_snapshot_id: format!("dsv2_{}", "2".repeat(64)),
                case_binding_hash: "3".repeat(64),
                expected_producer_content_id: built.materialization.producer_content_id.clone(),
                expected_producer_manifest_sha256: built
                    .materialization
                    .producer_content_manifest_sha256
                    .clone(),
                rule_generation: format!("tlgen1_{:x}", generation.finalize()),
                rule_digest,
                dataset_utc_offset_minutes: 0,
                expected_currency: "CNY".to_string(),
                minor_unit_scale: 2,
            };
        let path = output_descriptor_path(&snapshot);
        let connection = crate::open_account_flow_readonly_connection(&path)
            .expect("open immutable cleaning input read-only");
        let mut cleaned = Vec::new();
        let result = crate::stats_query_store::deterministic_cleaning::run(
            &connection,
            &cleaning_arguments,
            &mut cleaned,
        )
        .expect("run deterministic cleaning");
        drop(connection);
        assert_eq!(result.row_count, 1);
        assert_eq!(result.changed_row_count, 1);
        assert_eq!(result.changed_rows.len(), 1);
        assert_eq!(result.changed_rows[0].status, "changed");
        assert_eq!(result.changed_rows[0].cells.len(), 1);
        assert_eq!(result.changed_rows[0].cells[0].field, "counterpartyAccount");
        assert_eq!(result.changed_rows[0].cells[0].before_value, "CP-001_ 23");
        assert_eq!(result.changed_rows[0].cells[0].after_value, "CP00123");
        assert_eq!(
            result.output_artifact_sha256,
            format!("{:x}", Sha256::digest(&cleaned))
        );
        assert!(String::from_utf8_lossy(&cleaned).contains("CP00123"));

        let (cleaned_directory, _cleaned_former_path, mut cleaned_snapshot) =
            temp_unlinked_output("deterministic-cleaning-output");
        let mut cleaned_build_arguments = arguments();
        cleaned_build_arguments.private_import_file_id = "abcdef0123456789abcd".to_string();
        cleaned_build_arguments.source_revision = 8;
        cleaned_build_arguments.raw_artifact_manifest_sha256 = result.result_digest.clone();
        let rebuilt = run_funds_build_canonical_csv_snapshot_v1_in(
            cleaned_build_arguments,
            &cleaned,
            &mut cleaned_snapshot,
            &cleaned_directory,
        )
        .expect("rebuild deterministic cleaning output");
        assert_eq!(rebuilt.source_row_count, result.row_count);
        let cleaned_path = output_descriptor_path(&cleaned_snapshot);
        let cleaned_connection = crate::open_account_flow_readonly_connection(&cleaned_path)
            .expect("open rebuilt cleaning output read-only");
        let mut replay_arguments = cleaning_arguments.clone();
        replay_arguments.dataset_snapshot_id = format!("dsv2_{}", "7".repeat(64));
        replay_arguments.expected_producer_content_id =
            rebuilt.materialization.producer_content_id.clone();
        replay_arguments.expected_producer_manifest_sha256 = rebuilt
            .materialization
            .producer_content_manifest_sha256
            .clone();
        let mut replayed = Vec::new();
        let replay = crate::stats_query_store::deterministic_cleaning::run(
            &cleaned_connection,
            &replay_arguments,
            &mut replayed,
        )
        .expect("replay deterministic cleaning against canonical output");
        drop(cleaned_connection);
        assert_eq!(replay.changed_row_count, 0);
        assert_eq!(replay.unchanged_row_count, 1);
        assert_eq!(replayed, cleaned);
        drop(cleaned_snapshot);
        fs::remove_dir(cleaned_directory).expect("remove cleaned output directory");

        drop(snapshot);
        fs::remove_dir(directory).expect("remove empty output directory");
    }

    #[test]
    fn row_hash_is_occurrence_bound_and_go_json_compatible() {
        let values = row();
        let raw = values
            .iter()
            .map(|value| (!value.is_empty()).then(|| value.clone()))
            .collect::<Vec<_>>();
        let first = raw_occurrence_digest(1, &raw).expect("first occurrence digest");
        let second = raw_occurrence_digest(2, &raw).expect("second occurrence digest");
        assert_ne!(first, second);
        assert_eq!(
            first,
            "96db55eac4c2279a7219787f34b619c069e3ecc06d4a2445b15817536c782fb8"
        );

        let escaped = go_compatible_json("\"<&  \"".as_bytes().to_vec());
        assert_eq!(escaped, b"\"\\u003c\\u0026\\u2028\\u2029\"");
    }

    #[cfg(unix)]
    #[test]
    fn source_row_projection_matches_go_canonical_row_golden() {
        let (directory, _former_path, mut output) = temp_unlinked_output("go-row-golden");
        let path = output_descriptor_path(&output);
        let values = go_cross_language_row();
        let raw = values
            .iter()
            .map(|value| (!value.is_empty()).then(|| value.clone()))
            .collect::<Vec<_>>();
        assert_eq!(
            raw_occurrence_digest(1, &raw).expect("row hash"),
            "e46255658f9b58cf505d9a74bbcc68095ffbdde559d54fa15bdf73739fd34b2d"
        );
        let source = csv(&values);
        run_funds_build_canonical_csv_snapshot_v1_in(arguments(), &source, &mut output, &directory)
            .expect("build Go golden snapshot");

        let page = crate::run_funds_transaction_source_row_page_v1(
            crate::FundsTransactionSourceRowPageV1Arguments {
                case_id: "case-a".to_string(),
                binding_key_digest: "2".repeat(64),
                parsed_generation_identity_sha256: "3".repeat(64),
                relation: "fc_transaction_raw".to_string(),
                max_rows: 100,
                cursor: None,
            },
            "case-a",
            &path,
        )
        .expect("project Go golden source row");
        assert_eq!(page.rows.len(), 1);
        assert_eq!(
            page.rows[0].canonical_row_sha256,
            "e092c1b936de8a85eb10747048f77c1343fe0ef8a8b9d22da2da298866c8b354"
        );
        drop(output);
        fs::remove_dir(directory).expect("remove empty output directory");
    }

    #[test]
    fn hostile_source_profiles_fail_closed() {
        let base = row();
        let mut cases = Vec::new();
        let mut bom = csv(&base);
        bom.splice(0..0, [0xef, 0xbb, 0xbf]);
        cases.push(bom);
        let mut nul = csv(&base);
        nul.push(0);
        cases.push(nul);
        cases.push(
            format!(
                "交易帐号,{}\n",
                COLUMNS[1..]
                    .iter()
                    .map(|column| column.header)
                    .collect::<Vec<_>>()
                    .join(",")
            )
            .into_bytes(),
        );
        let mut reversed = COLUMNS
            .iter()
            .map(|column| column.header)
            .collect::<Vec<_>>();
        reversed.swap(0, 1);
        cases.push(format!("{}\n{}\n", reversed.join(","), base.join(",")).into_bytes());
        cases.push(format!("{}\n{}\n", header(), base[..base.len() - 1].join(",")).into_bytes());
        let mut whitespace = base.clone();
        whitespace[10] = " 姓名".to_string();
        cases.push(csv(&whitespace));
        let mut format = base.clone();
        format[10] = "姓\u{200b}名".to_string();
        cases.push(csv(&format));

        for source in cases {
            assert!(walk_canonical_source(&source, |_, _, _| Ok(())).is_err());
        }
        assert!(
            validate_source_envelope(&vec![b'x'; FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1 + 1])
                .is_err()
        );
    }

    #[test]
    fn noncanonical_normalization_is_rejected() {
        let mut cases = Vec::new();
        for (index, value) in [
            (TXN_TIME_INDEX, "2026-1-02 03:04:05"),
            (TXN_TIME_INDEX, "2026-02-30 03:04:05"),
            (AMOUNT_INDEX, "+1"),
            (AMOUNT_INDEX, "01"),
            (AMOUNT_INDEX, "1.0"),
            (AMOUNT_INDEX, "1.234"),
            (BALANCE_INDEX, "-0"),
            (DIRECTION_INDEX, "入"),
            (CURRENCY_INDEX, "cny"),
            (ACCOUNT_INDEX, "1000-0000"),
            (CARD_INDEX, "6222 0212"),
        ] {
            let mut values = row();
            values[index] = value.to_string();
            cases.push(csv(&values));
        }
        let mut no_identity = row();
        no_identity[CARD_INDEX].clear();
        no_identity[ACCOUNT_INDEX].clear();
        cases.push(csv(&no_identity));
        for source in cases {
            assert!(walk_canonical_source(&source, |_, _, _| Ok(())).is_err());
        }
    }

    #[test]
    fn rfc4180_quotes_are_exact_but_quoted_controls_are_rejected() {
        let mut values = row();
        values[13] = "含,逗号和\"引号\"的摘要".to_string();
        let mut encoded = Vec::new();
        {
            let mut writer = csv::WriterBuilder::new()
                .terminator(csv::Terminator::CRLF)
                .from_writer(&mut encoded);
            writer
                .write_record(COLUMNS.iter().map(|column| column.header))
                .expect("write exact header");
            writer.write_record(&values).expect("write quoted record");
            writer.flush().expect("flush quoted CSV");
        }
        assert_eq!(
            walk_canonical_source(&encoded, |_, record, _| {
                assert_eq!(record.raw[13].as_deref(), Some("含,逗号和\"引号\"的摘要"));
                Ok(())
            })
            .expect("parse RFC4180 quoted row")
            .row_count,
            1
        );

        values[13] = "quoted\nnewline".to_string();
        encoded.clear();
        {
            let mut writer = csv::Writer::from_writer(&mut encoded);
            writer
                .write_record(COLUMNS.iter().map(|column| column.header))
                .expect("write exact header");
            writer
                .write_record(&values)
                .expect("write quoted newline record");
            writer.flush().expect("flush quoted newline CSV");
        }
        assert!(walk_canonical_source(&encoded, |_, _, _| Ok(())).is_err());
    }

    #[test]
    fn row_limit_matches_the_shared_fixed_maximum() {
        let row = row().join(",");
        let mut source = format!("{}\n", header());
        for _ in 0..FUNDS_CANONICAL_CSV_MAX_ROWS_V1 {
            source.push_str(&row);
            source.push('\n');
        }
        assert_eq!(
            walk_canonical_source(source.as_bytes(), |_, _, _| Ok(()))
                .expect("exact row cap")
                .row_count,
            FUNDS_CANONICAL_CSV_MAX_ROWS_V1
        );
        source.push_str(&row);
        source.push('\n');
        assert!(walk_canonical_source(source.as_bytes(), |_, _, _| Ok(())).is_err());
    }
}
