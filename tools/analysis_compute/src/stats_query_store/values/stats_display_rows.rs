use anyhow::Result;
use duckdb::{Connection, Row};
use serde::Serialize;
use serde_json::{json, Value};
use std::io::Write;

use super::super::perf::{elapsed_ms, StatsQueryDiagnostics};
use super::json_write::{write_json_f64, write_json_string};
use super::placeholder_values::{placeholder_kind_from_token_value, placeholder_label_for_kind};

pub(in crate::stats_query_store) struct StatsRowsValues {
    pub(in crate::stats_query_store) rows: Vec<Value>,
    pub(in crate::stats_query_store) total: i64,
    pub(in crate::stats_query_store) row_summary: Value,
}

pub(in crate::stats_query_store) struct StatsRowsJsonWriteResult {
    pub(in crate::stats_query_store) row_count: usize,
    pub(in crate::stats_query_store) total: i64,
    pub(in crate::stats_query_store) row_summary: Value,
}

#[derive(Clone, Copy, PartialEq, Eq)]
pub(in crate::stats_query_store) enum StatsRowsJsonRowFormat {
    Object,
    Array,
}

pub(in crate::stats_query_store) struct StatsRowsJsonOutputSpec {
    pub(in crate::stats_query_store) row_format: StatsRowsJsonRowFormat,
    pub(in crate::stats_query_store) fields: Vec<&'static str>,
}

pub(in crate::stats_query_store) const STATS_DISPLAY_ROW_FIELD_NAMES: [&str; 23] = [
    "id",
    "counterparty_account",
    "counterparty_name",
    "relation",
    "location",
    "bank",
    "doc",
    "total_amount",
    "total_count",
    "net_in",
    "net_out",
    "in_amount",
    "in_count",
    "out_amount",
    "out_count",
    "first_time",
    "last_time",
    "keyType",
    "keyValue",
    "keyLabel",
    "drillKeyType",
    "drillKeyValue",
    "placeholderKind",
];

#[derive(Serialize)]
struct StatsDisplayRow {
    id: String,
    counterparty_account: String,
    counterparty_name: String,
    relation: String,
    location: String,
    bank: String,
    doc: String,
    total_amount: f64,
    total_count: i64,
    net_in: f64,
    net_out: f64,
    in_amount: f64,
    in_count: i64,
    out_amount: f64,
    out_count: i64,
    first_time: String,
    last_time: String,
    #[serde(rename = "keyType")]
    key_type: &'static str,
    #[serde(rename = "keyValue")]
    key_value: String,
    #[serde(rename = "keyLabel")]
    key_label: String,
    #[serde(rename = "drillKeyType")]
    drill_key_type: &'static str,
    #[serde(rename = "drillKeyValue")]
    drill_key_value: String,
    #[serde(rename = "placeholderKind")]
    placeholder_kind: String,
}

pub(in crate::stats_query_store) fn query_stats_row_values(
    conn: &Connection,
    sql: &str,
    group_key: &str,
    with_total: bool,
    output_spec: &StatsRowsJsonOutputSpec,
) -> Result<StatsRowsValues> {
    let mut stmt = conn.prepare(sql)?;
    let mapped = stmt.query_map([], |row| {
        stats_row_with_total_from_row(row, group_key, with_total)
    })?;

    let mut rows = Vec::new();
    let mut total = 0_i64;
    let mut summary_total_amount = 0.0_f64;
    let mut summary_total_count = 0_i64;
    for item in mapped {
        let (row, total_count) = item?;
        if with_total && total == 0 {
            total = total_count;
        }
        summary_total_amount += row.total_amount;
        summary_total_count += row.total_count;
        rows.push(stats_display_row_to_json_value(&row, output_spec)?);
    }
    Ok(StatsRowsValues {
        rows,
        total,
        row_summary: json!({
            "total_amount": round2(summary_total_amount),
            "total_count": summary_total_count,
        }),
    })
}

pub(in crate::stats_query_store) fn write_stats_row_values_json<W: Write>(
    conn: &Connection,
    sql: &str,
    group_key: &str,
    with_total: bool,
    output_spec: &StatsRowsJsonOutputSpec,
    writer: &mut W,
    diagnostics: &mut StatsQueryDiagnostics,
) -> Result<StatsRowsJsonWriteResult> {
    let stage_started = std::time::Instant::now();
    let mut stmt = conn.prepare(sql)?;
    diagnostics.record_elapsed("sql.rows.prepare", stage_started);

    let stage_started = std::time::Instant::now();
    let mapped = stmt.query_map([], |row| {
        stats_row_with_total_from_row(row, group_key, with_total)
    })?;
    diagnostics.record_elapsed("sql.rows.query_map", stage_started);

    writer.write_all(b"[")?;
    let mut first = true;
    let mut row_count = 0_usize;
    let mut total = 0_i64;
    let mut summary_total_amount = 0.0_f64;
    let mut summary_total_count = 0_i64;
    let mut row_map_ms = 0.0_f64;
    let mut row_json_write_ms = 0.0_f64;
    for item in mapped {
        let stage_started = std::time::Instant::now();
        let (row, total_count) = item?;
        row_map_ms += elapsed_ms(stage_started);
        if with_total && total == 0 {
            total = total_count;
        }
        summary_total_amount += row.total_amount;
        summary_total_count += row.total_count;
        if !first {
            writer.write_all(b",")?;
        }
        let stage_started = std::time::Instant::now();
        write_stats_display_row_json(writer, &row, output_spec)?;
        row_json_write_ms += elapsed_ms(stage_started);
        first = false;
        row_count += 1;
    }
    writer.write_all(b"]")?;
    diagnostics.record_duration_ms("sql.rows.row_map", row_map_ms);
    diagnostics.record_duration_ms("json.rows.row_write", row_json_write_ms);

    Ok(StatsRowsJsonWriteResult {
        row_count,
        total,
        row_summary: json!({
            "total_amount": round2(summary_total_amount),
            "total_count": summary_total_count,
        }),
    })
}

pub(in crate::stats_query_store) fn stats_rows_json_output_spec(
    row_format: &str,
    fields: &[String],
) -> StatsRowsJsonOutputSpec {
    let row_format = if row_format.trim().eq_ignore_ascii_case("array") {
        StatsRowsJsonRowFormat::Array
    } else {
        StatsRowsJsonRowFormat::Object
    };
    let mut selected_fields = Vec::new();
    if row_format == StatsRowsJsonRowFormat::Array {
        for field in fields {
            if let Some(name) = stats_display_row_field_name(field) {
                if !selected_fields.contains(&name) {
                    selected_fields.push(name);
                }
            }
        }
    }
    if selected_fields.is_empty() {
        selected_fields.extend(STATS_DISPLAY_ROW_FIELD_NAMES);
    }
    StatsRowsJsonOutputSpec {
        row_format,
        fields: selected_fields,
    }
}

pub(in crate::stats_query_store) fn write_stats_row_fields_json<W: Write>(
    writer: &mut W,
    output_spec: &StatsRowsJsonOutputSpec,
) -> Result<()> {
    serde_json::to_writer(writer, &output_spec.fields)?;
    Ok(())
}

fn stats_row_with_total_from_row(
    row: &Row<'_>,
    group_key: &str,
    with_total: bool,
) -> duckdb::Result<(StatsDisplayRow, i64)> {
    let key_value: Option<String> = row.get(0)?;
    let counterparty_account: Option<String> = row.get(1)?;
    let counterparty_name: Option<String> = row.get(2)?;
    let bank: Option<String> = row.get(3)?;
    let location: Option<String> = row.get(4)?;
    let placeholder_kind: Option<String> = row.get(5)?;
    let key_label: Option<String> = row.get(6)?;
    let in_amount: f64 = row.get(7)?;
    let out_amount: f64 = row.get(8)?;
    let in_count: i64 = row.get(9)?;
    let out_count: i64 = row.get(10)?;
    let first_time: Option<String> = row.get(11)?;
    let last_time: Option<String> = row.get(12)?;
    let doc_label: Option<String> = row.get(13)?;
    let total_count = if with_total {
        row.get::<_, i64>(14)?
    } else {
        0
    };
    Ok((
        stats_display_row_from_values(
            group_key,
            key_value,
            counterparty_account,
            counterparty_name,
            bank,
            location,
            placeholder_kind,
            key_label,
            in_amount,
            out_amount,
            in_count,
            out_count,
            first_time,
            last_time,
            doc_label,
        ),
        total_count,
    ))
}

#[allow(clippy::too_many_arguments)]
fn stats_display_row_from_values(
    group_key: &str,
    key_value: Option<String>,
    counterparty_account: Option<String>,
    counterparty_name: Option<String>,
    bank: Option<String>,
    location: Option<String>,
    placeholder_kind: Option<String>,
    key_label: Option<String>,
    in_amount: f64,
    out_amount: f64,
    in_count: i64,
    out_count: i64,
    first_time: Option<String>,
    last_time: Option<String>,
    doc_label: Option<String>,
) -> StatsDisplayRow {
    let key_value = string_or_empty(key_value).trim().to_string();
    let mut placeholder_kind = string_or_empty(placeholder_kind).trim().to_string();
    if placeholder_kind.is_empty() {
        if let Some(kind) = placeholder_kind_from_token_value(&key_value) {
            placeholder_kind = kind.to_string();
        }
    }
    let mut key_label = key_label
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| key_value.clone())
        .trim()
        .to_string();
    let mut counterparty_account = string_or_empty(counterparty_account).trim().to_string();
    let mut counterparty_name = string_or_empty(counterparty_name).trim().to_string();
    let bank = string_or_empty(bank).trim().to_string();
    let location = string_or_empty(location).trim().to_string();
    if !placeholder_kind.is_empty() {
        let label = placeholder_label_for_kind(&placeholder_kind);
        if !label.is_empty() {
            key_label = label.to_string();
        }
        if placeholder_kind == "empty" {
            counterparty_account.clear();
            if group_key == "cp_name" {
                counterparty_name.clear();
            }
        } else {
            counterparty_account = key_label.clone();
            if group_key == "cp_name" {
                counterparty_name = key_label.clone();
            }
        }
    }

    let total_amount = in_amount + out_amount;
    let total_count = in_count + out_count;
    let key_type = if group_key == "cp_key" {
        "account"
    } else {
        "name"
    };
    StatsDisplayRow {
        id: format!("{group_key}:{key_value}"),
        counterparty_account,
        counterparty_name,
        relation: String::new(),
        location,
        bank,
        doc: doc_label.unwrap_or_else(|| "未调单".to_string()),
        total_amount: round2(total_amount),
        total_count,
        net_in: round2(in_amount - out_amount),
        net_out: round2(out_amount - in_amount),
        in_amount: round2(in_amount),
        in_count,
        out_amount: round2(out_amount),
        out_count,
        first_time: string_or_empty(first_time),
        last_time: string_or_empty(last_time),
        key_type,
        key_value: key_value.clone(),
        key_label,
        drill_key_type: key_type,
        drill_key_value: key_value,
        placeholder_kind,
    }
}

fn round2(value: f64) -> f64 {
    (value * 100.0).round() / 100.0
}

fn string_or_empty(value: Option<String>) -> String {
    value.unwrap_or_default()
}

fn stats_display_row_field_name(raw: &str) -> Option<&'static str> {
    let field = raw.trim();
    STATS_DISPLAY_ROW_FIELD_NAMES
        .iter()
        .copied()
        .find(|candidate| *candidate == field)
}

fn stats_display_row_to_json_value(
    row: &StatsDisplayRow,
    output_spec: &StatsRowsJsonOutputSpec,
) -> Result<Value> {
    if output_spec.row_format == StatsRowsJsonRowFormat::Array {
        return Ok(Value::Array(
            output_spec
                .fields
                .iter()
                .map(|field| stats_display_row_field_value(row, field))
                .collect(),
        ));
    }
    Ok(serde_json::to_value(row)?)
}

fn stats_display_row_field_value(row: &StatsDisplayRow, field: &str) -> Value {
    match field {
        "id" => json!(row.id),
        "counterparty_account" => json!(row.counterparty_account),
        "counterparty_name" => json!(row.counterparty_name),
        "relation" => json!(row.relation),
        "location" => json!(row.location),
        "bank" => json!(row.bank),
        "doc" => json!(row.doc),
        "total_amount" => json!(row.total_amount),
        "total_count" => json!(row.total_count),
        "net_in" => json!(row.net_in),
        "net_out" => json!(row.net_out),
        "in_amount" => json!(row.in_amount),
        "in_count" => json!(row.in_count),
        "out_amount" => json!(row.out_amount),
        "out_count" => json!(row.out_count),
        "first_time" => json!(row.first_time),
        "last_time" => json!(row.last_time),
        "keyType" => json!(row.key_type),
        "keyValue" => json!(row.key_value),
        "keyLabel" => json!(row.key_label),
        "drillKeyType" => json!(row.drill_key_type),
        "drillKeyValue" => json!(row.drill_key_value),
        "placeholderKind" => json!(row.placeholder_kind),
        _ => Value::Null,
    }
}

fn write_stats_display_row_json<W: Write>(
    writer: &mut W,
    row: &StatsDisplayRow,
    output_spec: &StatsRowsJsonOutputSpec,
) -> Result<()> {
    if output_spec.row_format == StatsRowsJsonRowFormat::Array {
        return write_stats_display_row_array_json(writer, row, output_spec);
    }
    write_stats_display_row_object_json(writer, row)
}

fn write_stats_display_row_array_json<W: Write>(
    writer: &mut W,
    row: &StatsDisplayRow,
    output_spec: &StatsRowsJsonOutputSpec,
) -> Result<()> {
    writer.write_all(b"[")?;
    for (index, field) in output_spec.fields.iter().enumerate() {
        if index > 0 {
            writer.write_all(b",")?;
        }
        write_stats_display_row_field_json(writer, row, field)?;
    }
    writer.write_all(b"]")?;
    Ok(())
}

fn write_stats_display_row_field_json<W: Write>(
    writer: &mut W,
    row: &StatsDisplayRow,
    field: &str,
) -> Result<()> {
    match field {
        "id" => write_json_string(writer, &row.id)?,
        "counterparty_account" => write_json_string(writer, &row.counterparty_account)?,
        "counterparty_name" => write_json_string(writer, &row.counterparty_name)?,
        "relation" => write_json_string(writer, &row.relation)?,
        "location" => write_json_string(writer, &row.location)?,
        "bank" => write_json_string(writer, &row.bank)?,
        "doc" => write_json_string(writer, &row.doc)?,
        "total_amount" => write_json_f64(writer, row.total_amount)?,
        "total_count" => write!(writer, "{}", row.total_count)?,
        "net_in" => write_json_f64(writer, row.net_in)?,
        "net_out" => write_json_f64(writer, row.net_out)?,
        "in_amount" => write_json_f64(writer, row.in_amount)?,
        "in_count" => write!(writer, "{}", row.in_count)?,
        "out_amount" => write_json_f64(writer, row.out_amount)?,
        "out_count" => write!(writer, "{}", row.out_count)?,
        "first_time" => write_json_string(writer, &row.first_time)?,
        "last_time" => write_json_string(writer, &row.last_time)?,
        "keyType" => write_json_string(writer, row.key_type)?,
        "keyValue" => write_json_string(writer, &row.key_value)?,
        "keyLabel" => write_json_string(writer, &row.key_label)?,
        "drillKeyType" => write_json_string(writer, row.drill_key_type)?,
        "drillKeyValue" => write_json_string(writer, &row.drill_key_value)?,
        "placeholderKind" => write_json_string(writer, &row.placeholder_kind)?,
        _ => writer.write_all(b"null")?,
    }
    Ok(())
}

fn write_stats_display_row_object_json<W: Write>(
    writer: &mut W,
    row: &StatsDisplayRow,
) -> Result<()> {
    writer.write_all(b"{\"id\":")?;
    write_json_string(writer, &row.id)?;
    writer.write_all(b",\"counterparty_account\":")?;
    write_json_string(writer, &row.counterparty_account)?;
    writer.write_all(b",\"counterparty_name\":")?;
    write_json_string(writer, &row.counterparty_name)?;
    writer.write_all(b",\"relation\":")?;
    write_json_string(writer, &row.relation)?;
    writer.write_all(b",\"location\":")?;
    write_json_string(writer, &row.location)?;
    writer.write_all(b",\"bank\":")?;
    write_json_string(writer, &row.bank)?;
    writer.write_all(b",\"doc\":")?;
    write_json_string(writer, &row.doc)?;
    writer.write_all(b",\"total_amount\":")?;
    write_json_f64(writer, row.total_amount)?;
    writer.write_all(b",\"total_count\":")?;
    write!(writer, "{}", row.total_count)?;
    writer.write_all(b",\"net_in\":")?;
    write_json_f64(writer, row.net_in)?;
    writer.write_all(b",\"net_out\":")?;
    write_json_f64(writer, row.net_out)?;
    writer.write_all(b",\"in_amount\":")?;
    write_json_f64(writer, row.in_amount)?;
    writer.write_all(b",\"in_count\":")?;
    write!(writer, "{}", row.in_count)?;
    writer.write_all(b",\"out_amount\":")?;
    write_json_f64(writer, row.out_amount)?;
    writer.write_all(b",\"out_count\":")?;
    write!(writer, "{}", row.out_count)?;
    writer.write_all(b",\"first_time\":")?;
    write_json_string(writer, &row.first_time)?;
    writer.write_all(b",\"last_time\":")?;
    write_json_string(writer, &row.last_time)?;
    writer.write_all(b",\"keyType\":")?;
    write_json_string(writer, row.key_type)?;
    writer.write_all(b",\"keyValue\":")?;
    write_json_string(writer, &row.key_value)?;
    writer.write_all(b",\"keyLabel\":")?;
    write_json_string(writer, &row.key_label)?;
    writer.write_all(b",\"drillKeyType\":")?;
    write_json_string(writer, row.drill_key_type)?;
    writer.write_all(b",\"drillKeyValue\":")?;
    write_json_string(writer, &row.drill_key_value)?;
    writer.write_all(b",\"placeholderKind\":")?;
    write_json_string(writer, &row.placeholder_kind)?;
    writer.write_all(b"}")?;
    Ok(())
}
