use anyhow::Result;

use crate::command::run_analysis_compute_failure;
use crate::fixtures::{seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_flow_focus_rows_cli_rejects_min_amount_filter() -> Result<()> {
    let db = TempDb::new("flow-focus-min-amount")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-flow-focus-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--include-missing-counterparty",
        "false",
        "--min-amount",
        "100.0",
    ])?;

    assert!(failure.stdout.trim().is_empty());
    assert!(
        failure
            .stderr
            .contains("query-flow-focus-rows does not support per-transaction min_amount filters"),
        "unexpected stderr: {}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_flow_focus_cli_rejects_nonfinite_numeric_arguments() -> Result<()> {
    let db = TempDb::new("flow-focus-nonfinite-numeric")?;
    seed_stats_query_tables(db.path())?;

    for value in ["NaN", "inf", "-inf"] {
        let rows_failure = run_analysis_compute_failure(&[
            "query-flow-focus-rows",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--query-seed-id",
            "CARD-001",
            "--selected-focus-id",
            "CP-001",
            "--min-amount",
            value,
        ])?;
        assert!(
            rows_failure.stderr.contains("--min-amount must be finite"),
            "value={value} stderr={}",
            rows_failure.stderr
        );

        let graph_failure = run_analysis_compute_failure(&[
            "query-flow-focus-graph",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--query-seed-id",
            "CARD-001",
            "--seed-id",
            "CARD-001",
            "--selected-focus-id",
            "CP-001",
            "--expected-total-amount",
            value,
        ])?;
        assert!(
            graph_failure
                .stderr
                .contains("--expected-total-amount must be finite"),
            "value={value} stderr={}",
            graph_failure.stderr
        );
    }
    Ok(())
}
