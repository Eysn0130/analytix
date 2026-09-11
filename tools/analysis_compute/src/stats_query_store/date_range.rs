use anyhow::{Context, Result};
use serde_json::{json, Value};

use super::args::QueryStatsDateRangeArgs;
use super::session::VerifiedStatsQuerySession;

pub(crate) fn query_stats_date_range(args: &QueryStatsDateRangeArgs) -> Result<Value> {
    let conn = crate::open_readonly_connection(&args.db_path)
        .with_context(|| format!("open DuckDB {}", args.db_path.display()))?;
    crate::configure_connection(&conn)?;
    let mut session = VerifiedStatsQuerySession::begin(&conn, args)?;
    let result = query_stats_date_range_with_session(&mut session)?;
    session.commit()?;
    Ok(result)
}

pub(super) fn query_stats_date_range_with_session(
    session: &mut VerifiedStatsQuerySession<'_>,
) -> Result<Value> {
    let sql = format!(
        "
        SELECT
          CAST(MIN(COALESCE(first_ts, last_ts)) AS VARCHAR) AS min_ts,
          CAST(MAX(COALESCE(last_ts, first_ts)) AS VARCHAR) AS max_ts
        FROM {}
        WHERE first_ts IS NOT NULL OR last_ts IS NOT NULL
        ",
        crate::AGG_TABLE
    );
    let range_sql = format!(
        "SELECT 1 FROM {} WHERE first_ts IS NOT NULL OR last_ts IS NOT NULL",
        crate::AGG_TABLE
    );
    session.require_nonempty_query("date_range", &range_sql)?;
    let mut stmt = session.conn().prepare(&sql)?;
    let (min_ts, max_ts) = stmt.query_row([], |row| {
        Ok((
            row.get::<_, Option<String>>(0)?.unwrap_or_default(),
            row.get::<_, Option<String>>(1)?.unwrap_or_default(),
        ))
    })?;
    Ok(json!({
        "min": min_ts,
        "max": max_ts,
    }))
}
