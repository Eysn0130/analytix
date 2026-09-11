use crate::command::run_analysis_compute_failure;
use crate::fixtures::{seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;
use anyhow::Result;

#[test]
fn query_stats_rows_cli_rejects_missing_selection_without_zero_summary() -> Result<()> {
    let db = TempDb::new("stats-rows-empty-selection")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inAccount",
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_required"),
        "stderr={}",
        failure.stderr
    );

    let output_path = db.path().with_extension("empty-rows-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();
    let failure = run_analysis_compute_failure(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inAccount",
        "--output-json",
        &output_path_text,
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_required"),
        "stderr={}",
        failure.stderr
    );
    assert!(!output_path.exists());
    Ok(())
}

#[test]
fn query_stats_rows_cli_rejects_empty_offset_page() -> Result<()> {
    let db = TempDb::new("stats-rows-offset")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inAccount",
        "--selected-key",
        "CARD-001",
        "--row-sort-col",
        "total_amount",
        "--row-sort-dir",
        "desc",
        "--row-offset",
        "5",
        "--row-limit",
        "2",
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_stats_rows_cli_does_not_write_empty_offset_page() -> Result<()> {
    let db = TempDb::new("stats-rows-output-offset")?;
    seed_stats_query_tables(db.path())?;
    let output_path = db.path().with_extension("rows-offset-output.json");
    let output_path_text = output_path.to_string_lossy().to_string();

    let failure = run_analysis_compute_failure(&[
        "query-stats-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--mode",
        "inAccount",
        "--selected-key",
        "CARD-001",
        "--row-sort-col",
        "total_amount",
        "--row-sort-dir",
        "desc",
        "--row-offset",
        "5",
        "--row-limit",
        "2",
        "--output-json",
        &output_path_text,
    ])?;

    assert!(
        failure.stderr.contains("stats_query_scope_empty"),
        "stderr={}",
        failure.stderr
    );
    assert!(!output_path.exists());
    Ok(())
}
