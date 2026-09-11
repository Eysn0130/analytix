use anyhow::Result;
use duckdb::Connection;
use serde_json::json;

use crate::command::{run_analysis_compute_command, run_analysis_compute_failure};
use crate::fixtures::{
    insert_replacement_card_duplicate, refresh_stats_query_materialization_meta,
    seed_stats_query_tables, CASE_ID,
};
use crate::temp_db::TempDb;

#[test]
fn query_chart_dashboard_cli_returns_combined_dashboard_payload() -> Result<()> {
    let db = TempDb::new("chart-dashboard")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(2));
    assert_eq!(dashboard["summary"]["net_in_amount"], json!(7000.0));
    assert_eq!(dashboard["trend"]["points"][0]["value"], json!(13000.0));
    assert_eq!(
        dashboard["counterparties"]["views"]["name"]["top_items"][0]["label"],
        json!("对手甲")
    );
    assert_eq!(
        dashboard["structure"]["views"]["direction"],
        json!([
            {
                "label": "流入",
                "in_amount": 10000.0,
                "out_amount": 0.0,
                "net_amount": 10000.0,
                "total_amount": 10000.0,
                "value": 10000.0,
                "count": 1
            },
            {
                "label": "流出",
                "in_amount": 0.0,
                "out_amount": 3000.0,
                "net_amount": -3000.0,
                "total_amount": 3000.0,
                "value": 3000.0,
                "count": 1
            }
        ])
    );
    assert_eq!(dashboard["heatmap"]["weekdays"][0]["value"], json!(13000.0));
    assert_eq!(
        dashboard["distribution"]["views"]["bank"][0]["label"],
        json!("测试银行")
    );
    assert_eq!(dashboard["flow"]["totals"]["out_amount"], json!(3000.0));
    assert_eq!(
        dashboard["anomaly"]["amount_buckets"],
        json!([
            {"label": "1千-1万", "amount": 3000.0, "count": 1},
            {"label": "1万-5万", "amount": 10000.0, "count": 1}
        ])
    );
    assert_eq!(
        dashboard["anomaly"]["txn_type_top"][0],
        json!({"label": "转账", "amount": 13000.0, "count": 2})
    );
    assert_eq!(
        dashboard["anomaly"]["ip_top"][0],
        json!({"label": "未知IP", "amount": 13000.0, "count": 2})
    );
    assert_eq!(
        dashboard["anomaly"]["failure_reason_top"][0],
        json!({"label": "无反馈原因", "amount": 13000.0, "count": 2})
    );
    assert_eq!(
        dashboard["anomaly"]["summary_keyword_top"],
        json!([
            {"label": "工资入账", "count": 1},
            {"label": "转出", "count": 1}
        ])
    );
    assert_eq!(dashboard["anomaly"]["large_txn_count"], json!(0));
    assert_eq!(
        dashboard["anomaly"]["large_txns"][0]["amount"],
        json!(10000.0)
    );
    assert_eq!(
        dashboard["anomaly"]["large_txns"][0]["is_large"],
        json!(false)
    );
    assert_eq!(
        dashboard["anomaly"]["quality"]["ip_non_empty_rate"],
        json!(0.0)
    );
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_rejects_missing_scope_instead_of_zero_payload() -> Result<()> {
    let db = TempDb::new("chart-dashboard-missing-scope")?;
    seed_stats_query_tables(db.path())?;

    let failure = run_analysis_compute_failure(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
    ])?;
    assert!(
        failure.stderr.contains("stats_query_scope_required"),
        "stderr={}",
        failure.stderr
    );
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_dedupes_replacement_card_rows_across_all_views() -> Result<()> {
    let db = TempDb::new("chart-dashboard-replacement-card-dedupe")?;
    seed_stats_query_tables(db.path())?;
    insert_replacement_card_duplicate(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--selected-key",
        "CARD-003",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "multi-card",
        "--large-txn-threshold",
        "5000",
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["total_in_amount"], json!(10000.0));
    assert_eq!(dashboard["summary"]["total_out_amount"], json!(3000.0));
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(2));
    assert_eq!(dashboard["trend"]["points"][0]["value"], json!(13000.0));
    assert_eq!(
        dashboard["counterparties"]["views"]["name"]["top_items"][0]["in_count"],
        json!(1)
    );
    assert_eq!(
        dashboard["structure"]["views"]["direction"][0]["count"],
        json!(1)
    );
    assert_eq!(dashboard["heatmap"]["weekdays"][0]["value"], json!(13000.0));
    assert_eq!(
        dashboard["distribution"]["views"]["bank"][0]["value"],
        json!(13000.0)
    );
    assert_eq!(dashboard["flow"]["totals"]["in_amount"], json!(10000.0));
    assert_eq!(dashboard["flow"]["totals"]["in_count"], json!(1));
    assert_eq!(dashboard["anomaly"]["large_txn_count"], json!(1));
    assert_eq!(dashboard["anomaly"]["amount_buckets"][1]["count"], json!(1));
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_applies_supported_chart_filters() -> Result<()> {
    let db = TempDb::new("chart-dashboard-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
        "--chart-filters-json",
        r#"[{"source_panel_id":"counterparties","dimension":"counterparty_name","value":"对手甲","label":"对手甲","payload":{}}]"#,
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(1));
    assert_eq!(dashboard["summary"]["net_in_amount"], json!(10000.0));
    assert_eq!(dashboard["trend"]["points"][0]["value"], json!(10000.0));
    assert_eq!(
        dashboard["counterparties"]["views"]["name"]["all_items"],
        json!([
            {
                "label": "对手甲",
                "bank": "测试银行",
                "name": "对手甲",
                "account": "CP-001",
                "in_amount": 10000.0,
                "out_amount": 0.0,
                "net_amount": 10000.0,
                "total_amount": 10000.0,
                "in_count": 1,
                "out_count": 0,
                "total_count": 1,
                "value": 10000.0
            }
        ])
    );
    assert_eq!(
        dashboard["structure"]["views"]["direction"][0]["label"],
        json!("流入")
    );
    assert_eq!(dashboard["heatmap"]["weekdays"][0]["value"], json!(10000.0));
    assert_eq!(
        dashboard["distribution"]["views"]["bank"][0]["value"],
        json!(10000.0)
    );
    assert_eq!(dashboard["flow"]["totals"]["out_amount"], json!(0.0));
    assert_eq!(
        dashboard["anomaly"]["amount_buckets"],
        json!([{"label": "1万-5万", "amount": 10000.0, "count": 1}])
    );
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_applies_summary_keyword_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-dashboard-summary-keyword-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
        "--chart-filters-json",
        r#"[{"source_panel_id":"anomaly","dimension":"summary_keyword","value":"工资入账","label":"工资入账","payload":{}}]"#,
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(1));
    assert_eq!(dashboard["summary"]["total_in_amount"], json!(10000.0));
    assert_eq!(dashboard["summary"]["total_out_amount"], json!(0.0));
    assert_eq!(
        dashboard["anomaly"]["summary_keyword_top"],
        json!([{"label": "工资入账", "count": 1}])
    );
    assert_eq!(dashboard["flow"]["totals"]["out_count"], json!(0));
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_applies_remark_keyword_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-dashboard-remark-keyword-filtered")?;
    seed_stats_query_tables(db.path())?;
    {
        let conn = Connection::open(db.path())?;
        conn.execute_batch(
            "
            UPDATE analysis_txn_detail_idx
               SET remark = 'ATM取现过路费'
             WHERE txn_id = 'txn-rust-out';
            INSERT INTO analysis_txn_keyword_idx VALUES
              (2, 'txn-rust-out', 'remark', 'ATM', 0),
              (2, 'txn-rust-out', 'remark', '取现', 1),
              (2, 'txn-rust-out', 'remark', '过路费', 2);
            ",
        )?;
    }
    refresh_stats_query_materialization_meta(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
        "--chart-filters-json",
        r#"[{"source_panel_id":"anomaly","dimension":"remark_keyword","value":"过路费","label":"过路费","payload":{}}]"#,
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(1));
    assert_eq!(dashboard["summary"]["total_in_amount"], json!(0.0));
    assert_eq!(dashboard["summary"]["total_out_amount"], json!(3000.0));
    assert_eq!(dashboard["flow"]["totals"]["out_count"], json!(1));
    assert_eq!(
        dashboard["anomaly"]["remark_keyword_top"][0],
        json!({"label": "ATM", "count": 1})
    );
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_applies_time_bucket_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-dashboard-time-bucket-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "hour",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
        "--chart-filters-json",
        r#"[{"source_panel_id":"trend","dimension":"time_bucket","value":"2026-04-01 09:00","label":"2026-04-01 09:00","payload":{"granularity":"hour","bucket":"2026-04-01 09:00"}}]"#,
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(1));
    assert_eq!(dashboard["summary"]["net_in_amount"], json!(10000.0));
    assert_eq!(
        dashboard["trend"]["points"][0]["bucket"],
        json!("2026-04-01 09:00")
    );
    assert_eq!(dashboard["flow"]["totals"]["out_count"], json!(0));
    Ok(())
}

#[test]
fn query_chart_dashboard_cli_applies_heatmap_cell_chart_filter() -> Result<()> {
    let db = TempDb::new("chart-dashboard-heatmap-filtered")?;
    seed_stats_query_tables(db.path())?;

    let output = run_analysis_compute_command(&[
        "query-chart-dashboard",
        "--case-id",
        CASE_ID,
        "--db-path",
        &db.path().to_string_lossy(),
        "--selected-key",
        "CARD-001",
        "--metric-mode",
        "amount",
        "--direction-mode",
        "all",
        "--granularity",
        "day",
        "--success-filter",
        "all",
        "--cash-filter",
        "all",
        "--selection-mode",
        "single-card",
        "--large-txn-threshold",
        "50000",
        "--chart-filters-json",
        r#"[{"source_panel_id":"heatmap","dimension":"heatmap_cell","value":"2-9","label":"周三 09:00","payload":{"weekday":2,"hour":9}}]"#,
    ])?;

    let dashboard = &output["dashboard"];
    assert_eq!(dashboard["summary"]["txn_total_count"], json!(1));
    assert_eq!(dashboard["summary"]["total_out_amount"], json!(0.0));
    assert_eq!(dashboard["heatmap"]["cells"][0]["hour"], json!(9));
    assert_eq!(
        dashboard["counterparties"]["views"]["name"]["all_items"][0]["label"],
        json!("对手甲")
    );
    Ok(())
}
