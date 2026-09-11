use anyhow::Result;
use duckdb::{Connection, Row};
use serde_json::{json, Value};

#[derive(Clone, Debug)]
pub(in crate::stats_query_store) struct FlowFocusRow {
    acct_key: Option<String>,
    cp_key: Option<String>,
    cp_key_raw: Option<String>,
    dc_val: Option<String>,
    txn_count: i64,
    amt_sum: f64,
    first_ts: Option<String>,
    last_ts: Option<String>,
    open_name: Option<String>,
    cp_name: Option<String>,
}

pub(in crate::stats_query_store) fn query_flow_focus_row_values(
    conn: &Connection,
    sql: &str,
) -> Result<Vec<Value>> {
    Ok(query_flow_focus_rows_typed(conn, sql)?
        .into_iter()
        .map(FlowFocusRow::into_value)
        .collect())
}

pub(in crate::stats_query_store) fn query_flow_focus_rows_typed(
    conn: &Connection,
    sql: &str,
) -> Result<Vec<FlowFocusRow>> {
    let mut stmt = conn.prepare(sql)?;
    let mapped = stmt.query_map([], flow_focus_row_from_row)?;

    let mut out = Vec::new();
    for item in mapped {
        out.push(item?);
    }
    Ok(out)
}

fn flow_focus_row_from_row(row: &Row<'_>) -> duckdb::Result<FlowFocusRow> {
    Ok(FlowFocusRow {
        acct_key: row.get(0)?,
        cp_key: row.get(1)?,
        cp_key_raw: row.get(2)?,
        dc_val: row.get(3)?,
        txn_count: row.get::<_, i64>(4)?,
        amt_sum: row.get::<_, f64>(5)?,
        first_ts: row.get(6)?,
        last_ts: row.get(7)?,
        open_name: row.get(8)?,
        cp_name: row.get(9)?,
    })
}

impl FlowFocusRow {
    #[allow(clippy::too_many_arguments)]
    pub(in crate::stats_query_store) fn new(
        acct_key: String,
        cp_key: String,
        cp_key_raw: String,
        dc_val: String,
        txn_count: i64,
        amt_sum: f64,
        first_ts: String,
        last_ts: String,
        open_name: String,
        cp_name: String,
    ) -> Self {
        Self {
            acct_key: Some(acct_key),
            cp_key: Some(cp_key),
            cp_key_raw: Some(cp_key_raw),
            dc_val: Some(dc_val),
            txn_count,
            amt_sum,
            first_ts: Some(first_ts),
            last_ts: Some(last_ts),
            open_name: Some(open_name),
            cp_name: Some(cp_name),
        }
    }

    pub(in crate::stats_query_store) fn acct_key(&self) -> &str {
        trimmed(self.acct_key.as_deref())
    }

    pub(in crate::stats_query_store) fn cp_key(&self) -> &str {
        trimmed(self.cp_key.as_deref())
    }

    pub(in crate::stats_query_store) fn cp_key_raw(&self) -> &str {
        trimmed(self.cp_key_raw.as_deref())
    }

    pub(in crate::stats_query_store) fn dc_val(&self) -> &str {
        trimmed(self.dc_val.as_deref())
    }

    pub(in crate::stats_query_store) fn txn_count(&self) -> i64 {
        self.txn_count
    }

    pub(in crate::stats_query_store) fn amt_sum(&self) -> f64 {
        self.amt_sum
    }

    pub(in crate::stats_query_store) fn first_ts(&self) -> &str {
        trimmed(self.first_ts.as_deref())
    }

    pub(in crate::stats_query_store) fn last_ts(&self) -> &str {
        trimmed(self.last_ts.as_deref())
    }

    pub(in crate::stats_query_store) fn open_name(&self) -> &str {
        trimmed(self.open_name.as_deref())
    }

    pub(in crate::stats_query_store) fn cp_name(&self) -> &str {
        trimmed(self.cp_name.as_deref())
    }

    fn into_value(self) -> Value {
        Value::Array(vec![
            optional_string_value(self.acct_key),
            optional_string_value(self.cp_key),
            optional_string_value(self.cp_key_raw),
            optional_string_value(self.dc_val),
            json!(self.txn_count),
            json!(self.amt_sum),
            optional_string_value(self.first_ts),
            optional_string_value(self.last_ts),
            optional_string_value(self.open_name),
            optional_string_value(self.cp_name),
        ])
    }
}

fn optional_string_value(value: Option<String>) -> Value {
    value.map(Value::String).unwrap_or(Value::Null)
}

fn trimmed(value: Option<&str>) -> &str {
    value.unwrap_or_default().trim()
}
