use anyhow::{Context, Result};
use duckdb::Connection;

pub(crate) fn update_count(conn: &Connection, sql: &str) -> Result<i64> {
    let stmt = sql.trim().trim_end_matches(';');
    conn.execute(stmt, [])
        .map(|changed| changed as i64)
        .with_context(|| format!("count updated rows for SQL: {}", compact_sql(stmt, 320)))
}

fn compact_sql(sql: &str, limit: usize) -> String {
    let compact = sql.split_whitespace().collect::<Vec<_>>().join(" ");
    if compact.len() <= limit {
        compact
    } else {
        format!("{}...", &compact[..limit])
    }
}
