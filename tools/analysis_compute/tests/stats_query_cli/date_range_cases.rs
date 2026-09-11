use anyhow::{Context, Result};
use serde_json::json;

use crate::command::run_analysis_compute_command;
use crate::fixtures::{seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_stats_date_range_cli_returns_min_first_ts_and_max_last_ts() -> Result<()> {
    let db = TempDb::new("stats-date-range")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-stats-date-range",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
    ])?;

    let date_range = output["date_range"]
        .as_object()
        .context("date_range should be object")?;
    assert_eq!(date_range["min"], json!("2026-04-01 09:00:00"));
    assert_eq!(date_range["max"], json!("2026-04-02 11:00:00"));
    Ok(())
}
