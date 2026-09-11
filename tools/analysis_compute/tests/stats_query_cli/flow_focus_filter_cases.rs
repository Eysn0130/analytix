use anyhow::Result;
use serde_json::json;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{seed_stats_query_tables, CASE_ID};
use crate::temp_db::TempDb;

#[test]
fn query_flow_focus_rows_cli_filters_direction_and_date_window() -> Result<()> {
    let db = TempDb::new("flow-focus-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-flow-focus-rows",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--query-seed-id",
        "CARD-001",
        "--selected-focus-id",
        "CP-001",
        "--selected-focus-id",
        "CP-002",
        "--include-missing-counterparty",
        "false",
        "--direction",
        "out",
        "--date-start",
        "2026-04-01",
        "--date-end-excl",
        "2026-04-02",
    ])?;

    assert_eq!(
        output,
        json!({
            "ok": true,
            "case_id": CASE_ID,
            "rows": [
                [
                    "CARD-001", "CP-002", "CP-002", "出", 1, 3000.0,
                    "2026-04-01 10:00:00", "2026-04-01 10:00:00", "张三", "对手乙"
                ],
            ],
        })
    );
    Ok(())
}

#[test]
fn query_flow_focus_rows_cli_rejects_empty_focus_selection() -> Result<()> {
    let db = TempDb::new("flow-focus-empty-selection")?;
    seed_stats_query_tables(db.path())?;

    for include_missing in ["false", "true"] {
        let failure = run_analysis_compute_failure(&[
            "query-flow-focus-rows",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db.path().to_string_lossy(),
            "--query-seed-id",
            "CARD-001",
            "--include-missing-counterparty",
            include_missing,
        ])?;

        assert!(
            failure.stderr.contains("stats_query_scope_required"),
            "stderr={}",
            failure.stderr
        );
    }

    Ok(())
}

#[test]
fn query_flow_focus_rows_cli_rejects_empty_seed_selection() -> Result<()> {
    let db = TempDb::new("flow-focus-empty-seed")?;
    seed_stats_query_tables(db.path())?;
    let db_path = db.path().to_string_lossy().into_owned();

    let cases: &[&[&str]] = &[
        &[
            "--selected-focus-id",
            "CP-001",
            "--include-missing-counterparty",
            "false",
        ],
        &[
            "--selected-placeholder-kind",
            "empty",
            "--include-missing-counterparty",
            "true",
        ],
    ];

    for case_args in cases {
        let mut args = vec![
            "query-flow-focus-rows",
            "--case-id",
            CASE_ID,
            "--db-path",
            &db_path,
        ];
        args.extend_from_slice(case_args);

        let failure = run_analysis_compute_failure(&args)?;

        assert!(
            failure.stderr.contains("stats_query_scope_required"),
            "stderr={}",
            failure.stderr
        );
    }

    Ok(())
}
