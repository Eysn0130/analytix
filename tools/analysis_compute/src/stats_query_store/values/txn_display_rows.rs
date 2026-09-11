use anyhow::Result;
use duckdb::{Connection, Row};
use serde::{Serialize, Serializer};
use serde_json::{json, Value};
use std::io::Write;

use super::super::perf::{elapsed_ms, StatsQueryDiagnostics};
use super::json_write::write_json_string;

pub(in crate::stats_query_store) struct TxnRowWithCursor {
    pub(in crate::stats_query_store) row: TxnDisplayRow,
    pub(in crate::stats_query_store) id: i64,
    pub(in crate::stats_query_store) amount: Option<f64>,
    pub(in crate::stats_query_store) txn_ts: Option<String>,
}

#[derive(Serialize)]
pub(in crate::stats_query_store) struct TxnDisplayRow {
    id: Option<i64>,
    card_no: String,
    acct_no: String,
    account_open_name: String,
    opener_id_no: String,
    txn_time: String,
    #[serde(serialize_with = "serialize_optional_f64_or_empty")]
    amount: Option<f64>,
    #[serde(serialize_with = "serialize_optional_f64_or_empty")]
    balance: Option<f64>,
    dc_flag: String,
    counterparty_acct: String,
    cash_flag: String,
    counterparty_name: String,
    counterparty_id_no: String,
    counterparty_bank: String,
    summary: String,
    currency: String,
    branch_name: String,
    branch_code: String,
    location: String,
    is_success: String,
    voucher_no: String,
    terminal_no: String,
    ip_addr: String,
    mac_addr: String,
    #[serde(serialize_with = "serialize_optional_f64_or_empty")]
    counterparty_balance: Option<f64>,
    txn_id: String,
    log_id: String,
    voucher_type: String,
    voucher_id: String,
    teller_no: String,
    merchant_name: String,
    merchant_no: String,
    remark: String,
    txn_type: String,
    query_feedback_reason: String,
}

pub(in crate::stats_query_store) struct TxnRowsJsonWriteResult {
    pub(in crate::stats_query_store) row_count: usize,
    pub(in crate::stats_query_store) last_id: i64,
    pub(in crate::stats_query_store) last_amount: Option<f64>,
    pub(in crate::stats_query_store) last_txn_ts: Option<String>,
}

pub(in crate::stats_query_store) struct TxnRowsWithTotalJsonWriteResult {
    pub(in crate::stats_query_store) row_count: usize,
    pub(in crate::stats_query_store) total: i64,
}

#[derive(Clone, Copy, PartialEq, Eq)]
pub(in crate::stats_query_store) enum TxnRowsJsonRowFormat {
    Object,
    Array,
}

pub(in crate::stats_query_store) struct TxnRowsJsonOutputSpec {
    pub(in crate::stats_query_store) row_format: TxnRowsJsonRowFormat,
    fields: Vec<&'static str>,
}

impl TxnRowsJsonOutputSpec {
    pub(in crate::stats_query_store) fn fields(&self) -> &[&'static str] {
        &self.fields
    }
}

const TXN_DISPLAY_ROW_FIELD_NAMES: [&str; 35] = [
    "id",
    "card_no",
    "acct_no",
    "account_open_name",
    "opener_id_no",
    "txn_time",
    "amount",
    "balance",
    "dc_flag",
    "counterparty_acct",
    "cash_flag",
    "counterparty_name",
    "counterparty_id_no",
    "counterparty_bank",
    "summary",
    "currency",
    "branch_name",
    "branch_code",
    "location",
    "is_success",
    "voucher_no",
    "terminal_no",
    "ip_addr",
    "mac_addr",
    "counterparty_balance",
    "txn_id",
    "log_id",
    "voucher_type",
    "voucher_id",
    "teller_no",
    "merchant_name",
    "merchant_no",
    "remark",
    "txn_type",
    "query_feedback_reason",
];

pub(in crate::stats_query_store) fn txn_rows_json_output_spec(
    row_format: &str,
    fields: &[String],
) -> TxnRowsJsonOutputSpec {
    let row_format = if row_format.trim().eq_ignore_ascii_case("array") {
        TxnRowsJsonRowFormat::Array
    } else {
        TxnRowsJsonRowFormat::Object
    };
    let mut selected_fields = Vec::new();
    for field in fields {
        let Some(name) = normalize_txn_display_row_field(field) else {
            continue;
        };
        if !selected_fields.contains(&name) {
            selected_fields.push(name);
        }
    }
    if selected_fields.is_empty() {
        selected_fields.extend(TXN_DISPLAY_ROW_FIELD_NAMES);
    }
    TxnRowsJsonOutputSpec {
        row_format,
        fields: selected_fields,
    }
}

pub(in crate::stats_query_store) fn write_txn_row_fields_json<W: Write>(
    writer: &mut W,
    output_spec: &TxnRowsJsonOutputSpec,
) -> Result<()> {
    serde_json::to_writer(writer, &output_spec.fields)?;
    Ok(())
}

pub(in crate::stats_query_store) fn txn_display_row_to_json_value(
    row: &TxnDisplayRow,
    output_spec: &TxnRowsJsonOutputSpec,
) -> Value {
    if output_spec.row_format == TxnRowsJsonRowFormat::Array {
        return Value::Array(
            output_spec
                .fields
                .iter()
                .map(|field| txn_display_row_field_value(row, field))
                .collect(),
        );
    }
    let mut object = serde_json::Map::new();
    for field in &output_spec.fields {
        object.insert(
            (*field).to_string(),
            txn_display_row_field_value(row, field),
        );
    }
    Value::Object(object)
}

pub(in crate::stats_query_store) fn query_stats_txn_row_values(
    conn: &Connection,
    sql: &str,
) -> Result<Vec<TxnRowWithCursor>> {
    let mut stmt = conn.prepare(sql)?;
    let mapped = stmt.query_map([], |row| txn_row_with_cursor_from_row(row))?;

    let mut out = Vec::new();
    for item in mapped {
        out.push(item?);
    }
    Ok(out)
}

pub(in crate::stats_query_store) fn write_stats_txn_row_values_json<W: Write>(
    conn: &Connection,
    sql: &str,
    writer: &mut W,
    diagnostics: &mut StatsQueryDiagnostics,
    output_spec: &TxnRowsJsonOutputSpec,
) -> Result<TxnRowsJsonWriteResult> {
    let stage_started = std::time::Instant::now();
    let mut stmt = conn.prepare(sql)?;
    diagnostics.record_elapsed("sql.txn.prepare", stage_started);

    let stage_started = std::time::Instant::now();
    let mapped = stmt.query_map([], |row| txn_row_with_cursor_from_row(row))?;
    diagnostics.record_elapsed("sql.txn.query_map", stage_started);

    writer.write_all(b"[")?;
    let mut first = true;
    let mut row_count = 0_usize;
    let mut last_id = 0_i64;
    let mut last_amount = None;
    let mut last_txn_ts = None;
    let mut row_map_ms = 0.0_f64;
    let mut row_json_write_ms = 0.0_f64;
    for item in mapped {
        let stage_started = std::time::Instant::now();
        let item = item?;
        row_map_ms += elapsed_ms(stage_started);
        if !first {
            writer.write_all(b",")?;
        }
        let stage_started = std::time::Instant::now();
        write_txn_display_row_json(writer, &item.row, output_spec)?;
        row_json_write_ms += elapsed_ms(stage_started);
        first = false;
        row_count += 1;
        last_id = item.id;
        last_amount = item.amount;
        last_txn_ts = item.txn_ts;
    }
    writer.write_all(b"]")?;
    diagnostics.record_duration_ms("sql.txn.row_map", row_map_ms);
    diagnostics.record_duration_ms("json.txn.row_write", row_json_write_ms);

    Ok(TxnRowsJsonWriteResult {
        row_count,
        last_id,
        last_amount,
        last_txn_ts,
    })
}

pub(in crate::stats_query_store) fn write_stats_txn_row_values_json_with_total<W: Write>(
    conn: &Connection,
    sql: &str,
    writer: &mut W,
) -> Result<TxnRowsWithTotalJsonWriteResult> {
    let output_spec = txn_rows_json_output_spec("object", &[]);
    let mut stmt = conn.prepare(sql)?;
    let mapped = stmt.query_map([], |row| {
        let item = txn_row_with_cursor_from_row(row)?;
        let total_count = row.get::<_, i64>(36)?;
        Ok((item, total_count))
    })?;

    writer.write_all(b"[")?;
    let mut first = true;
    let mut row_count = 0_usize;
    let mut total = 0_i64;
    for item in mapped {
        let (item, total_count) = item?;
        if !first {
            writer.write_all(b",")?;
        }
        write_txn_display_row_json(writer, &item.row, &output_spec)?;
        first = false;
        row_count += 1;
        if total == 0 {
            total = total_count;
        }
    }
    writer.write_all(b"]")?;

    Ok(TxnRowsWithTotalJsonWriteResult { row_count, total })
}

pub(in crate::stats_query_store) fn query_stats_txn_row_values_with_total(
    conn: &Connection,
    sql: &str,
) -> Result<(Vec<TxnRowWithCursor>, i64)> {
    let mut stmt = conn.prepare(sql)?;
    let mapped = stmt.query_map([], |row| {
        let item = txn_row_with_cursor_from_row(row)?;
        let total_count = row.get::<_, i64>(36)?;
        Ok((item, total_count))
    })?;

    let mut out = Vec::new();
    let mut total = 0_i64;
    for item in mapped {
        let (row, total_count) = item?;
        if total == 0 {
            total = total_count;
        }
        out.push(row);
    }
    Ok((out, total))
}

fn txn_row_with_cursor_from_row(row: &Row<'_>) -> duckdb::Result<TxnRowWithCursor> {
    let id: i64 = row.get(0)?;
    let card_no: Option<String> = row.get(1)?;
    let acct_no: Option<String> = row.get(2)?;
    let account_open_name: Option<String> = row.get(3)?;
    let opener_id_no: Option<String> = row.get(4)?;
    let txn_time: Option<String> = row.get(5)?;
    let amount: Option<f64> = row.get(6)?;
    let balance: Option<f64> = row.get(7)?;
    let dc_flag: Option<String> = row.get(8)?;
    let counterparty_acct: Option<String> = row.get(9)?;
    let cash_flag: Option<String> = row.get(10)?;
    let counterparty_name: Option<String> = row.get(11)?;
    let counterparty_id_no: Option<String> = row.get(12)?;
    let counterparty_bank: Option<String> = row.get(13)?;
    let summary: Option<String> = row.get(14)?;
    let currency: Option<String> = row.get(15)?;
    let branch_name: Option<String> = row.get(16)?;
    let branch_code: Option<String> = row.get(17)?;
    let location: Option<String> = row.get(18)?;
    let is_success: Option<String> = row.get(19)?;
    let voucher_no: Option<String> = row.get(20)?;
    let terminal_no: Option<String> = row.get(21)?;
    let ip_addr: Option<String> = row.get(22)?;
    let mac_addr: Option<String> = row.get(23)?;
    let counterparty_balance: Option<f64> = row.get(24)?;
    let txn_id: Option<String> = row.get(25)?;
    let log_id: Option<String> = row.get(26)?;
    let voucher_type: Option<String> = row.get(27)?;
    let voucher_id: Option<String> = row.get(28)?;
    let teller_no: Option<String> = row.get(29)?;
    let merchant_name: Option<String> = row.get(30)?;
    let merchant_no: Option<String> = row.get(31)?;
    let remark: Option<String> = row.get(32)?;
    let txn_type: Option<String> = row.get(33)?;
    let query_feedback_reason: Option<String> = row.get(34)?;
    let txn_ts: Option<String> = row.get(35)?;
    let txn_time_value = txn_time
        .filter(|value| !value.is_empty())
        .or_else(|| txn_ts.clone())
        .unwrap_or_default();
    Ok(TxnRowWithCursor {
        row: TxnDisplayRow {
            id: Some(id),
            card_no: string_or_empty(card_no),
            acct_no: string_or_empty(acct_no),
            account_open_name: string_or_empty(account_open_name),
            opener_id_no: string_or_empty(opener_id_no),
            txn_time: txn_time_value,
            amount,
            balance,
            dc_flag: string_or_empty(dc_flag),
            counterparty_acct: string_or_empty(counterparty_acct),
            cash_flag: string_or_empty(cash_flag),
            counterparty_name: string_or_empty(counterparty_name),
            counterparty_id_no: string_or_empty(counterparty_id_no),
            counterparty_bank: string_or_empty(counterparty_bank),
            summary: string_or_empty(summary),
            currency: string_or_empty(currency),
            branch_name: string_or_empty(branch_name),
            branch_code: string_or_empty(branch_code),
            location: string_or_empty(location),
            is_success: string_or_empty(is_success),
            voucher_no: string_or_empty(voucher_no),
            terminal_no: string_or_empty(terminal_no),
            ip_addr: string_or_empty(ip_addr),
            mac_addr: string_or_empty(mac_addr),
            counterparty_balance,
            txn_id: string_or_empty(txn_id),
            log_id: string_or_empty(log_id),
            voucher_type: string_or_empty(voucher_type),
            voucher_id: string_or_empty(voucher_id),
            teller_no: string_or_empty(teller_no),
            merchant_name: string_or_empty(merchant_name),
            merchant_no: string_or_empty(merchant_no),
            remark: string_or_empty(remark),
            txn_type: string_or_empty(txn_type),
            query_feedback_reason: string_or_empty(query_feedback_reason),
        },
        id,
        amount,
        txn_ts,
    })
}

fn string_or_empty(value: Option<String>) -> String {
    value.unwrap_or_default()
}

fn serialize_optional_f64_or_empty<S>(value: &Option<f64>, serializer: S) -> Result<S::Ok, S::Error>
where
    S: Serializer,
{
    match value {
        Some(item) if item.is_finite() => serializer.serialize_f64(*item),
        Some(_) => serializer.serialize_none(),
        None => serializer.serialize_str(""),
    }
}

fn write_txn_display_row_json<W: Write>(
    writer: &mut W,
    row: &TxnDisplayRow,
    output_spec: &TxnRowsJsonOutputSpec,
) -> Result<()> {
    if output_spec.row_format == TxnRowsJsonRowFormat::Array {
        writer.write_all(b"[")?;
        for (index, field) in output_spec.fields.iter().enumerate() {
            if index > 0 {
                writer.write_all(b",")?;
            }
            write_txn_display_row_field_json(writer, row, field)?;
        }
        writer.write_all(b"]")?;
        return Ok(());
    }

    writer.write_all(b"{")?;
    for (index, field) in output_spec.fields.iter().enumerate() {
        if index > 0 {
            writer.write_all(b",")?;
        }
        write_json_string(writer, field)?;
        writer.write_all(b":")?;
        write_txn_display_row_field_json(writer, row, field)?;
    }
    writer.write_all(b"}")?;
    Ok(())
}

fn normalize_txn_display_row_field(value: &str) -> Option<&'static str> {
    let field = value.trim();
    TXN_DISPLAY_ROW_FIELD_NAMES
        .iter()
        .copied()
        .find(|candidate| *candidate == field)
}

fn txn_display_row_field_value(row: &TxnDisplayRow, field: &str) -> Value {
    match field {
        "id" => row.id.map(Value::from).unwrap_or(Value::Null),
        "amount" => optional_f64_or_empty_value(row.amount),
        "balance" => optional_f64_or_empty_value(row.balance),
        "counterparty_balance" => optional_f64_or_empty_value(row.counterparty_balance),
        "card_no" => json!(row.card_no),
        "acct_no" => json!(row.acct_no),
        "account_open_name" => json!(row.account_open_name),
        "opener_id_no" => json!(row.opener_id_no),
        "txn_time" => json!(row.txn_time),
        "dc_flag" => json!(row.dc_flag),
        "counterparty_acct" => json!(row.counterparty_acct),
        "cash_flag" => json!(row.cash_flag),
        "counterparty_name" => json!(row.counterparty_name),
        "counterparty_id_no" => json!(row.counterparty_id_no),
        "counterparty_bank" => json!(row.counterparty_bank),
        "summary" => json!(row.summary),
        "currency" => json!(row.currency),
        "branch_name" => json!(row.branch_name),
        "branch_code" => json!(row.branch_code),
        "location" => json!(row.location),
        "is_success" => json!(row.is_success),
        "voucher_no" => json!(row.voucher_no),
        "terminal_no" => json!(row.terminal_no),
        "ip_addr" => json!(row.ip_addr),
        "mac_addr" => json!(row.mac_addr),
        "txn_id" => json!(row.txn_id),
        "log_id" => json!(row.log_id),
        "voucher_type" => json!(row.voucher_type),
        "voucher_id" => json!(row.voucher_id),
        "teller_no" => json!(row.teller_no),
        "merchant_name" => json!(row.merchant_name),
        "merchant_no" => json!(row.merchant_no),
        "remark" => json!(row.remark),
        "txn_type" => json!(row.txn_type),
        "query_feedback_reason" => json!(row.query_feedback_reason),
        _ => Value::Null,
    }
}

fn write_txn_display_row_field_json<W: Write>(
    writer: &mut W,
    row: &TxnDisplayRow,
    field: &str,
) -> Result<()> {
    match field {
        "id" => write_json_optional_i64(writer, row.id)?,
        "amount" => write_json_optional_f64_or_empty(writer, row.amount)?,
        "balance" => write_json_optional_f64_or_empty(writer, row.balance)?,
        "counterparty_balance" => {
            write_json_optional_f64_or_empty(writer, row.counterparty_balance)?
        }
        "card_no" => write_json_string(writer, &row.card_no)?,
        "acct_no" => write_json_string(writer, &row.acct_no)?,
        "account_open_name" => write_json_string(writer, &row.account_open_name)?,
        "opener_id_no" => write_json_string(writer, &row.opener_id_no)?,
        "txn_time" => write_json_string(writer, &row.txn_time)?,
        "dc_flag" => write_json_string(writer, &row.dc_flag)?,
        "counterparty_acct" => write_json_string(writer, &row.counterparty_acct)?,
        "cash_flag" => write_json_string(writer, &row.cash_flag)?,
        "counterparty_name" => write_json_string(writer, &row.counterparty_name)?,
        "counterparty_id_no" => write_json_string(writer, &row.counterparty_id_no)?,
        "counterparty_bank" => write_json_string(writer, &row.counterparty_bank)?,
        "summary" => write_json_string(writer, &row.summary)?,
        "currency" => write_json_string(writer, &row.currency)?,
        "branch_name" => write_json_string(writer, &row.branch_name)?,
        "branch_code" => write_json_string(writer, &row.branch_code)?,
        "location" => write_json_string(writer, &row.location)?,
        "is_success" => write_json_string(writer, &row.is_success)?,
        "voucher_no" => write_json_string(writer, &row.voucher_no)?,
        "terminal_no" => write_json_string(writer, &row.terminal_no)?,
        "ip_addr" => write_json_string(writer, &row.ip_addr)?,
        "mac_addr" => write_json_string(writer, &row.mac_addr)?,
        "txn_id" => write_json_string(writer, &row.txn_id)?,
        "log_id" => write_json_string(writer, &row.log_id)?,
        "voucher_type" => write_json_string(writer, &row.voucher_type)?,
        "voucher_id" => write_json_string(writer, &row.voucher_id)?,
        "teller_no" => write_json_string(writer, &row.teller_no)?,
        "merchant_name" => write_json_string(writer, &row.merchant_name)?,
        "merchant_no" => write_json_string(writer, &row.merchant_no)?,
        "remark" => write_json_string(writer, &row.remark)?,
        "txn_type" => write_json_string(writer, &row.txn_type)?,
        "query_feedback_reason" => write_json_string(writer, &row.query_feedback_reason)?,
        _ => writer.write_all(b"null")?,
    }
    Ok(())
}

fn optional_f64_or_empty_value(value: Option<f64>) -> Value {
    match value {
        Some(item) if item.is_finite() => json!(item),
        Some(_) => Value::Null,
        None => json!(""),
    }
}

fn write_json_optional_i64<W: Write>(writer: &mut W, value: Option<i64>) -> Result<()> {
    match value {
        Some(item) => write!(writer, "{item}")?,
        None => writer.write_all(b"null")?,
    }
    Ok(())
}

fn write_json_optional_f64_or_empty<W: Write>(writer: &mut W, value: Option<f64>) -> Result<()> {
    match value {
        Some(item) if item.is_finite() => serde_json::to_writer(writer, &item)?,
        Some(_) => writer.write_all(b"null")?,
        None => writer.write_all(b"\"\"")?,
    }
    Ok(())
}
