#![allow(dead_code)]

use anyhow::{anyhow, bail, Result};
use serde_json::{json, Map, Value};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;

mod duckdb_utils;
mod flow_analysis_graph_data;
mod flow_graph_contract;
mod flow_graph_data_merge_contract;
mod flow_graph_layout_contract;
mod flow_graph_parity_contract;
mod flow_graph_render_plan;
mod flow_graph_search;
mod flow_graph_search_core;
mod flow_graph_search_filter_contract;
mod flow_layout_cluster_plan;
mod flow_layout_network_angle_score;
mod flow_layout_network_boundary_score;
mod flow_layout_network_bucket_score;
mod flow_layout_network_candidate_score;
mod flow_layout_network_hard_corridor_score;
mod flow_layout_network_hard_zone_score;
mod flow_layout_network_model;
mod flow_layout_network_node_metrics;
mod flow_layout_network_occupied_score;
mod flow_layout_network_ownership;
mod flow_layout_network_plan;
mod flow_layout_network_ray_score;
mod flow_layout_network_sector;
mod flow_layout_network_sector_envelope;
mod flow_layout_network_sector_placement;
mod flow_layout_network_sector_ring;
mod flow_layout_network_sector_uniformity;
mod flow_layout_network_territory;
mod flow_layout_network_topology_plan;
mod flow_layout_node_plan;
mod flow_layout_role_graph;
mod flow_projection_layout_seed;
mod flow_projection_layout_sync;
mod flow_projection_model;
mod flow_same_name_merge;
mod funds_canonical_csv_snapshot_v1;
mod funds_materialize_txn_daily_v1;
mod funds_transaction_source_row_page_v1;
mod rule_pattern_args;
mod rule_pattern_features;
mod rule_pattern_store;
mod rule_txn_index_store;
mod sql_expr;
mod stats_query_store;
mod txn_daily_store;

pub(crate) use duckdb_utils::{
    canonical_case_table_signature, configure_connection, ensure_meta_table, ensure_required_table,
    is_sha256_hex, open_account_flow_readonly_connection, open_readonly_connection, scalar_i64,
    table_columns, table_exists,
};
use rule_pattern_store::materialize_rule_pattern_index;
use rule_txn_index_store::materialize_rule_txn_index;
pub(crate) use sql_expr::{
    account_key_expr, amount_value_expr, clean_row_acceptance_expr, coalesce_expr,
    counterparty_key_expr, counterparty_raw_expr, dc_value_expr, finite_num_value_expr,
    first_text_expr, has_col, placeholder_kind_sql, placeholder_label_sql, placeholder_token_sql,
    sql_literal, text_col_expr, text_col_expr_minlen, trim_expr, txn_ts_expr,
};
use stats_query_store::{
    query_chart_counterparties, query_chart_dashboard, query_chart_detail_rows,
    query_chart_detail_rows_to_json_writer, query_chart_distribution, query_chart_flow,
    query_chart_heatmap, query_chart_summary, query_chart_trend, query_flow_focus_graph,
    query_flow_focus_rows, query_stats_date_range, query_stats_rows,
    query_stats_rows_to_json_writer, query_stats_tree, query_stats_txn_rows,
    query_stats_txn_rows_to_json_writer, run_stats_query_worker,
};
use txn_daily_store::{materialize_txn_daily, verify_txn_daily};

pub use funds_canonical_csv_snapshot_v1::{
    run_funds_build_canonical_csv_snapshot_v1,
    validate_funds_build_canonical_csv_snapshot_v1_arguments,
    FundsBuildCanonicalCSVSnapshotV1Arguments, FundsBuildCanonicalCSVSnapshotV1Result,
    FUNDS_BUILD_CANONICAL_CSV_SNAPSHOT_V1_COMMAND, FUNDS_CANONICAL_CSV_MAX_ROWS_V1,
    FUNDS_CANONICAL_CSV_MAX_SOURCE_BYTES_V1, FUNDS_CANONICAL_DIRECT_CSV_PROFILE_V1,
};
pub use funds_materialize_txn_daily_v1::{
    run_funds_materialize_txn_daily_v1, validate_funds_materialize_txn_daily_v1_arguments,
    FundsMaterializeTxnDailyV1Arguments, FundsMaterializeTxnDailyV1Result,
    FUNDS_MATERIALIZE_TXN_DAILY_V1_COMMAND,
};
pub use funds_transaction_source_row_page_v1::{
    run_funds_transaction_source_row_page_v1,
    validate_funds_transaction_source_row_page_v1_arguments, FundsTransactionSourceRowCursorV1,
    FundsTransactionSourceRowPageV1Arguments, FundsTransactionSourceRowPageV1Result,
    FUNDS_TRANSACTION_SOURCE_ROW_PAGE_V1_COMMAND,
};

pub const FUNDS_ANALYZE_ACCOUNT_FLOWS_COMMAND: &str = "funds.analyze_account_flows";
pub const FUNDS_RESOLVE_ACCOUNT_INGRESS_COMMAND: &str = "funds.resolve_account_ingress";
pub const FUNDS_DIRECT_SOURCE_PREVIEW_COMMAND: &str = "funds.direct_source_preview";
pub const FUNDS_DETERMINISTIC_CLEANING_COMMAND: &str =
    stats_query_store::deterministic_cleaning::DETERMINISTIC_CLEANING_COMMAND;
pub const FUNDS_DETERMINISTIC_CLEANING_RULE_CONTRACT: &str =
    stats_query_store::deterministic_cleaning::DETERMINISTIC_CLEANING_RULE_CONTRACT;

pub(crate) const AGG_TABLE: &str = "analysis_txn_daily_agg";
pub(crate) const AGG_STAGING_TABLE: &str = "analysis_txn_daily_agg__staging";
pub(crate) const DETAIL_TABLE: &str = "analysis_txn_detail_idx";
pub(crate) const DETAIL_STAGING_TABLE: &str = "analysis_txn_detail_idx__staging";
pub(crate) const KEYWORD_TABLE: &str = "analysis_txn_keyword_idx";
pub(crate) const KEYWORD_STAGING_TABLE: &str = "analysis_txn_keyword_idx__staging";
pub(crate) const ACCOUNT_DIM_TABLE: &str = "analysis_account_dim";
pub(crate) const ACCOUNT_DIM_STAGING_TABLE: &str = "analysis_account_dim__staging";
pub(crate) const RULE_TXN_INDEX_TABLE: &str = "analysis_rule_txn_idx";
pub(crate) const RULE_TXN_INDEX_STAGING_TABLE: &str = "analysis_rule_txn_idx__staging";
pub(crate) const RULE_PATTERN_TABLE: &str = "analysis_rule_pattern_idx";
pub(crate) const RULE_PATTERN_STAGING_TABLE: &str = "analysis_rule_pattern_idx__staging";
pub(crate) const META_TABLE: &str = "analysis_materialization_meta";
pub(crate) const AGG_VERSION: i64 = 12;
pub(crate) const MATERIALIZATION_IDENTITY_SCHEMA_VERSION: i64 = 2;
pub(crate) const MATERIALIZATION_IDENTITY_PREFIX: &str = "txn_daily_snapshot:v12:";
pub(crate) const RULE_TXN_INDEX_VERSION: i64 = 2;
pub(crate) const RULE_TXN_INDEX_CACHE_KEY: &str = "rule_txn_index:v2";
pub(crate) const RULE_PATTERN_VERSION: i64 = 9;
pub(crate) const RULE_PATTERN_CACHE_KEY: &str = "rule_pattern_index:v9";
pub(crate) const NULL_TEXT: &str = "CAST(NULL AS VARCHAR)";
pub(crate) const NULL_DOUBLE: &str = "CAST(NULL AS DOUBLE)";
pub(crate) const NULL_TIMESTAMP: &str = "CAST(NULL AS TIMESTAMP)";

pub(crate) const ACCOUNT_KEY_CANDIDATES: &[&str] = &[
    "clean_acct_no",
    "acct_no_norm",
    "acct_no",
    "clean_card_no",
    "card_no_norm",
    "card_no",
];

pub fn run_cli(iter: impl Iterator<Item = String>) -> Result<()> {
    let mut iter = iter;
    let command = iter.next().ok_or_else(|| anyhow!("missing command"))?;
    match command.as_str() {
        "materialize-txn-daily" => {
            let args = txn_daily_store::parse_args(iter)?;
            let result = materialize_txn_daily(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "row_count": result.row_count,
                    "agg_name": &result.identity.value,
                    "agg_version": AGG_VERSION,
                    "materialization_identity": &result.identity.value,
                    "producer_content_id": &result.producer_content_id,
                    "phases_s": result.phases_s,
                    "phase_groups_s": result.phase_groups_s,
                })
            );
            Ok(())
        }
        "verify-txn-daily" => {
            let args = txn_daily_store::parse_verify_args(iter)?;
            let result = verify_txn_daily(&args)?;
            println!("{}", verify_txn_daily_result_json(&result));
            Ok(())
        }
        "query-stats-rows" => {
            let args = stats_query_store::parse_query_stats_rows_args(iter)?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_stats_rows_to_json_writer(&args, writer)
                })?;
                println!(
                    "{}",
                    json!({
                        "ok": true,
                        "case_id": args.case_id,
                        "group_key": result.group_key,
                        "total": result.total,
                        "row_count": result.row_count,
                        "output_json": output_json.display().to_string(),
                        "diagnostics": result.diagnostics,
                    })
                );
                return Ok(());
            }
            let result = query_stats_rows(&args)?;
            let mut payload = Map::new();
            payload.insert("ok".to_string(), Value::Bool(true));
            payload.insert("case_id".to_string(), Value::String(args.case_id));
            payload.insert("group_key".to_string(), Value::String(result.group_key));
            payload.insert("total".to_string(), Value::from(result.total));
            payload.insert("row_summary".to_string(), result.row_summary);
            payload.insert("rows".to_string(), Value::Array(result.rows));
            if let Some(row_fields) = result.row_fields {
                payload.insert("row_fields".to_string(), row_fields);
            }
            payload.insert("diagnostics".to_string(), result.diagnostics);
            println!("{}", Value::Object(payload));
            Ok(())
        }
        "query-stats-tree" => {
            let args = stats_query_store::parse_query_stats_tree_args(iter)?;
            let groups = query_stats_tree(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "groups": groups,
                })
            );
            Ok(())
        }
        "query-stats-date-range" => {
            let args = stats_query_store::parse_query_stats_date_range_args(iter)?;
            let date_range = query_stats_date_range(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "date_range": date_range,
                })
            );
            Ok(())
        }
        "query-stats-txn-rows" => {
            let args = stats_query_store::parse_query_stats_txn_rows_args(iter)?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_stats_txn_rows_to_json_writer(&args, writer)
                })?;
                println!(
                    "{}",
                    json!({
                        "ok": true,
                        "case_id": args.case_id,
                        "row_count": result.row_count,
                        "done": result.done,
                        "output_json": output_json.display().to_string(),
                        "diagnostics": result.diagnostics,
                    })
                );
                return Ok(());
            }
            let result = query_stats_txn_rows(&args)?;
            let mut payload = Map::new();
            payload.insert("ok".to_string(), Value::Bool(true));
            payload.insert("case_id".to_string(), Value::String(args.case_id));
            payload.insert("rows".to_string(), Value::Array(result.rows));
            if let Some(row_fields) = result.row_fields {
                payload.insert("row_fields".to_string(), row_fields);
            }
            payload.insert("diagnostics".to_string(), result.diagnostics);
            if let Some(done) = result.done {
                payload.insert("done".to_string(), Value::Bool(done));
                payload.insert(
                    "nextCursor".to_string(),
                    result.next_cursor.unwrap_or(Value::Null),
                );
            }
            println!("{}", Value::Object(payload));
            Ok(())
        }
        "stats-query-worker" => run_stats_query_worker(),
        "query-chart-detail-rows" => {
            let args = stats_query_store::parse_query_chart_detail_rows_args(iter)?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_chart_detail_rows_to_json_writer(&args, writer)
                })?;
                println!(
                    "{}",
                    json!({
                        "ok": true,
                        "case_id": args.case_id,
                        "total": result.total,
                        "row_count": result.row_count,
                        "output_json": output_json.display().to_string(),
                    })
                );
                return Ok(());
            }
            let result = query_chart_detail_rows(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "rows": result.rows,
                    "total": result.total,
                })
            );
            Ok(())
        }
        "query-chart-flow" => {
            let args = stats_query_store::parse_query_chart_flow_args(iter)?;
            let flow = query_chart_flow(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "flow": flow,
                })
            );
            Ok(())
        }
        "query-chart-counterparties" => {
            let args = stats_query_store::parse_query_chart_counterparties_args(iter)?;
            let counterparties = query_chart_counterparties(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "counterparties": counterparties,
                })
            );
            Ok(())
        }
        "query-chart-dashboard" => {
            let args = stats_query_store::parse_query_chart_dashboard_args(iter)?;
            let dashboard = query_chart_dashboard(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "dashboard": dashboard,
                })
            );
            Ok(())
        }
        "query-chart-distribution" => {
            let args = stats_query_store::parse_query_chart_distribution_args(iter)?;
            let distribution = query_chart_distribution(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "distribution": distribution,
                })
            );
            Ok(())
        }
        "query-chart-heatmap" => {
            let args = stats_query_store::parse_query_chart_heatmap_args(iter)?;
            let heatmap = query_chart_heatmap(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "heatmap": heatmap,
                })
            );
            Ok(())
        }
        "query-chart-summary" => {
            let args = stats_query_store::parse_query_chart_summary_args(iter)?;
            let summary = query_chart_summary(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "summary": summary,
                })
            );
            Ok(())
        }
        "query-chart-trend" => {
            let args = stats_query_store::parse_query_chart_trend_args(iter)?;
            let trend = query_chart_trend(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "trend": trend,
                })
            );
            Ok(())
        }
        "query-flow-focus-rows" => {
            let args = stats_query_store::parse_query_flow_focus_rows_args(iter)?;
            let rows = query_flow_focus_rows(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "rows": rows,
                })
            );
            Ok(())
        }
        "query-flow-focus-graph" => {
            let args = stats_query_store::parse_query_flow_focus_graph_args(iter)?;
            if let Some(output_json) = args.output_json.as_ref() {
                let graph = query_flow_focus_graph(&args)?;
                let has_graph = graph.is_some();
                let node_count = graph_stat_i64(graph.as_ref(), "node_count");
                let edge_count = graph_stat_i64(graph.as_ref(), "edge_count");
                write_json_file_with(output_json, |writer| {
                    serde_json::to_writer(writer, &json!({"graph": graph}))?;
                    Ok(())
                })?;
                println!(
                    "{}",
                    json!({
                        "ok": true,
                        "case_id": args.rows.case_id,
                        "has_graph": has_graph,
                        "node_count": node_count,
                        "edge_count": edge_count,
                        "output_json": output_json.display().to_string(),
                    })
                );
                return Ok(());
            }
            let graph = query_flow_focus_graph(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.rows.case_id,
                    "graph": graph,
                })
            );
            Ok(())
        }
        "project-flow-skeleton-clusters" => {
            let args = flow_projection_model::parse_args(iter)?;
            let projection = flow_projection_model::project_flow_skeleton_clusters(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "projection": projection,
                })
            );
            Ok(())
        }
        "project-layout-node-plan" => {
            let args = flow_layout_node_plan::parse_args(iter)?;
            let result = flow_layout_node_plan::project_layout_node_plan_result(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "nodePlan": result.get("nodePlan").cloned().unwrap_or(Value::Null),
                    "updates": result.get("updates").cloned().unwrap_or_else(|| json!([])),
                })
            );
            Ok(())
        }
        "project-layout-cache-key" => {
            let args = flow_layout_node_plan::parse_args(iter)?;
            let cache_key = flow_layout_node_plan::project_layout_cache_key(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "cacheKey": cache_key,
                })
            );
            Ok(())
        }
        "apply-layout-node-plan" => {
            let args = flow_layout_node_plan::parse_args(iter)?;
            let result = flow_layout_node_plan::apply_layout_node_plan_payload(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "result": result,
                })
            );
            Ok(())
        }
        "apply-layout-worker-node-plan" => {
            let args = flow_layout_node_plan::parse_args(iter)?;
            let result =
                flow_layout_node_plan::apply_layout_worker_node_plan_payload(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "result": result,
                })
            );
            Ok(())
        }
        "clear-layout-node-meta" => {
            let args = flow_layout_node_plan::parse_args(iter)?;
            let nodes = flow_layout_node_plan::clear_layout_meta_payload(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "nodes": nodes,
                })
            );
            Ok(())
        }
        "project-layout-role-graph" => {
            let args = flow_layout_role_graph::parse_args(iter)?;
            let projection = flow_layout_role_graph::project_layout_role_graph(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "projection": projection,
                })
            );
            Ok(())
        }
        "project-analysis-graph-data" => {
            let args = flow_analysis_graph_data::parse_args(iter)?;
            let graph_data = flow_analysis_graph_data::project_analysis_graph_data(&args.payload)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "graphData": graph_data,
                })
            );
            Ok(())
        }
        "merge-flow-same-name-graph" => {
            let args = flow_same_name_merge::parse_args(iter)?;
            let merge_result = flow_same_name_merge::merge_same_name_graph(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "mergeResult": merge_result,
                })
            );
            Ok(())
        }
        "contract-flow-graph-workflow" => {
            let args = flow_graph_contract::parse_args(iter)?;
            let contract = flow_graph_contract::project_graph_workflow_contract(&args.payload)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "contract": contract,
                })
            );
            Ok(())
        }
        "contract-flow-graph" => {
            let args = flow_graph_parity_contract::parse_args(iter)?;
            let graph_contract = flow_graph_parity_contract::project_graph_parity_contract(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "graphContract": graph_contract,
                })
            );
            Ok(())
        }
        "contract-flow-graph-data-merge" | "project-flow-graph-data-merge" => {
            let args = flow_graph_data_merge_contract::parse_args(iter)?;
            let data_merge =
                flow_graph_data_merge_contract::project_graph_data_merge_contract(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "dataMerge": data_merge,
                })
            );
            Ok(())
        }
        "contract-flow-graph-layout-projection" => {
            let args = flow_graph_layout_contract::parse_args(iter)?;
            let layout_projection =
                flow_graph_layout_contract::project_graph_layout_contract(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "layoutProjection": layout_projection,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-territory" => {
            let args = flow_layout_network_territory::parse_args(iter)?;
            let territory =
                flow_layout_network_territory::project_network_territory_contract(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "territory": territory,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-angle-score" => {
            let args = flow_layout_network_angle_score::parse_args(iter)?;
            let angle_score = flow_layout_network_angle_score::project_network_angle_score_contract(
                &args.payload,
            );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "angleScore": angle_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-boundary-score" => {
            let args = flow_layout_network_boundary_score::parse_args(iter)?;
            let boundary_score =
                flow_layout_network_boundary_score::project_network_boundary_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "boundaryScore": boundary_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-hard-zone-score" => {
            let args = flow_layout_network_hard_zone_score::parse_args(iter)?;
            let hard_zone_score =
                flow_layout_network_hard_zone_score::project_network_hard_zone_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "hardZoneScore": hard_zone_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-hard-corridor-score" => {
            let args = flow_layout_network_hard_corridor_score::parse_args(iter)?;
            let hard_corridor_score =
                flow_layout_network_hard_corridor_score::project_network_hard_corridor_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "hardCorridorScore": hard_corridor_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-occupied-score" => {
            let args = flow_layout_network_occupied_score::parse_args(iter)?;
            let occupied_score =
                flow_layout_network_occupied_score::project_network_occupied_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "occupiedScore": occupied_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-ray-score" => {
            let args = flow_layout_network_ray_score::parse_args(iter)?;
            let ray_score =
                flow_layout_network_ray_score::project_network_ray_score_contract(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "rayScore": ray_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-candidate-score" => {
            let args = flow_layout_network_candidate_score::parse_args(iter)?;
            let candidate_score =
                flow_layout_network_candidate_score::project_network_candidate_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "candidateScore": candidate_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-candidate-selection" => {
            let args = flow_layout_network_candidate_score::parse_args(iter)?;
            let candidate_selection =
                flow_layout_network_candidate_score::project_network_candidate_selection_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "candidateSelection": candidate_selection,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-candidate-attempt-selection" => {
            let args = flow_layout_network_candidate_score::parse_args(iter)?;
            let candidate_selection =
                flow_layout_network_candidate_score::project_network_candidate_attempt_selection_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "candidateSelection": candidate_selection,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-sector-envelope" => {
            let args = flow_layout_network_sector_envelope::parse_args(iter)?;
            let sector_envelope =
                flow_layout_network_sector_envelope::project_network_sector_envelope_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "sectorEnvelope": sector_envelope,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-bucket-score" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let bucket_score =
                flow_layout_network_bucket_score::project_network_bucket_score_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "bucketScore": bucket_score,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-bucket-window" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let bucket_window =
                flow_layout_network_bucket_score::project_network_bucket_window_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "bucketWindow": bucket_window,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-bucket-sector" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let bucket_sector =
                flow_layout_network_bucket_score::project_network_bucket_sector_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "bucketSector": bucket_sector,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-bucket-projection" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let bucket_projection =
                flow_layout_network_bucket_score::project_network_bucket_projection_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "bucketProjection": bucket_projection,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-sector-ring" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let sector_ring = flow_layout_network_sector_ring::project_network_sector_ring_contract(
                &args.payload,
            );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "sectorRing": sector_ring,
                })
            );
            Ok(())
        }
        "contract-flow-layout-network-node-metrics" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let node_metrics =
                flow_layout_network_node_metrics::project_network_node_metrics_contract(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "nodeMetrics": node_metrics,
                })
            );
            Ok(())
        }
        "project-flow-layout-network-sector-placement" => {
            let args = flow_layout_network_bucket_score::parse_args(iter)?;
            let sector_placement =
                flow_layout_network_sector_placement::project_network_sector_placement(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "sectorPlacement": sector_placement,
                })
            );
            Ok(())
        }
        "project-flow-layout-network-plan" => {
            let args = flow_layout_network_topology_plan::parse_args(iter)?;
            let network_plan =
                flow_layout_network_topology_plan::project_flow_layout_network_plan(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "networkPlan": network_plan,
                })
            );
            Ok(())
        }
        "project-flow-layout-network-community-quality" => {
            let args = flow_layout_network_topology_plan::parse_args(iter)?;
            let network_community_quality =
                flow_layout_network_topology_plan::project_flow_layout_network_community_quality(
                    &args.payload,
                );
            println!(
                "{}",
                json!({
                    "ok": true,
                    "networkCommunityQuality": network_community_quality,
                })
            );
            Ok(())
        }
        "project-flow-graph-render-plan" => {
            let args = flow_graph_render_plan::parse_args(iter)?;
            let render = flow_graph_render_plan::project_graph_render_plan(&args.payload);
            if let Some(output_json) = args.output_json.as_ref() {
                let render_result_count = render
                    .get("renderResults")
                    .and_then(Value::as_array)
                    .map(Vec::len)
                    .unwrap_or(0);
                let has_viewport_expand_targets = render.get("viewportExpandTargets").is_some();
                write_json_file_with(output_json, |writer| {
                    serde_json::to_writer(writer, &json!({"render": render}))?;
                    Ok(())
                })?;
                println!(
                    "{}",
                    json!({
                        "ok": true,
                        "render_result_count": render_result_count,
                        "has_viewport_expand_targets": has_viewport_expand_targets,
                        "output_json": output_json.display().to_string(),
                    })
                );
                return Ok(());
            }
            println!(
                "{}",
                json!({
                    "ok": true,
                    "render": render,
                })
            );
            Ok(())
        }
        "project-flow-graph-search" => {
            let args = flow_graph_search::parse_args(iter)?;
            let search = flow_graph_search::project_graph_search(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "search": search,
                })
            );
            Ok(())
        }
        "contract-flow-graph-search-filter" => {
            let args = flow_graph_search_filter_contract::parse_args(iter)?;
            let search_filter =
                flow_graph_search_filter_contract::project_graph_search_filter_contract(
                    &args.payload,
                )?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "searchFilter": search_filter,
                })
            );
            Ok(())
        }
        "project-flow-projection-layout-sync" => {
            let args = flow_projection_layout_sync::parse_args(iter)?;
            let projection = flow_projection_layout_sync::project_projection_layout_sync(&args);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "projection": projection,
                })
            );
            Ok(())
        }
        "project-flow-projection-layout-seed" => {
            let args = flow_projection_layout_seed::parse_args(iter)?;
            let seed = flow_projection_layout_seed::project_projection_layout_seed(&args.payload);
            println!(
                "{}",
                json!({
                    "ok": true,
                    "seed": seed,
                })
            );
            Ok(())
        }
        "materialize-rule-txn-index" => {
            let args = rule_txn_index_store::parse_args(iter)?;
            let result = materialize_rule_txn_index(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "row_count": result.row_count,
                    "rebuilt": result.rebuilt,
                    "agg_name": RULE_TXN_INDEX_CACHE_KEY,
                    "agg_version": RULE_TXN_INDEX_VERSION,
                })
            );
            Ok(())
        }
        "materialize-rule-pattern-index" => {
            let args = rule_pattern_args::parse_args(iter)?;
            let result = materialize_rule_pattern_index(&args)?;
            println!(
                "{}",
                json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "row_count": result.row_count,
                    "rebuilt": result.rebuilt,
                    "input_coverage": result.input_coverage,
                    "feature_readiness": result.feature_readiness,
                    "agg_name": RULE_PATTERN_CACHE_KEY,
                    "agg_version": RULE_PATTERN_VERSION,
                })
            );
            Ok(())
        }
        "--help" | "-h" => {
            println!(
                "Usage: analytix-analysis-compute <materialize-txn-daily|verify-txn-daily|query-stats-rows|query-stats-tree|query-stats-date-range|query-stats-txn-rows|stats-query-worker|query-chart-dashboard|query-chart-detail-rows|query-chart-counterparties|query-chart-distribution|query-chart-flow|query-chart-heatmap|query-chart-summary|query-chart-trend|query-flow-focus-rows|query-flow-focus-graph|project-flow-skeleton-clusters|project-layout-node-plan|project-layout-cache-key|apply-layout-node-plan|apply-layout-worker-node-plan|clear-layout-node-meta|project-layout-role-graph|project-analysis-graph-data|merge-flow-same-name-graph|project-flow-graph-data-merge|project-flow-graph-search|contract-flow-graph|contract-flow-graph-workflow|contract-flow-graph-data-merge|contract-flow-graph-layout-projection|contract-flow-layout-network-territory|contract-flow-layout-network-angle-score|contract-flow-layout-network-boundary-score|contract-flow-layout-network-hard-zone-score|contract-flow-layout-network-occupied-score|contract-flow-layout-network-ray-score|contract-flow-layout-network-candidate-score|contract-flow-layout-network-candidate-selection|contract-flow-layout-network-candidate-attempt-selection|contract-flow-layout-network-sector-envelope|contract-flow-layout-network-bucket-score|contract-flow-layout-network-bucket-window|contract-flow-layout-network-bucket-sector|contract-flow-layout-network-bucket-projection|contract-flow-layout-network-sector-ring|contract-flow-layout-network-node-metrics|project-flow-layout-network-sector-placement|project-flow-layout-network-plan|project-flow-layout-network-community-quality|project-flow-graph-render-plan|contract-flow-graph-search-filter|project-flow-projection-layout-sync|project-flow-projection-layout-seed|materialize-rule-txn-index|materialize-rule-pattern-index> ..."
            );
            Ok(())
        }
        other => bail!("unsupported command: {other}"),
    }
}

fn write_json_file_with<T>(
    path: &Path,
    write_payload: impl FnOnce(&mut BufWriter<File>) -> Result<T>,
) -> Result<T> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    let tmp_path = path.with_extension("json.tmp");
    let write_result = (|| {
        let file = File::create(&tmp_path)?;
        let mut writer = BufWriter::new(file);
        let value = write_payload(&mut writer)?;
        writer.flush()?;
        Ok(value)
    })();
    let value = match write_result {
        Ok(value) => value,
        Err(err) => {
            let _ = fs::remove_file(&tmp_path);
            return Err(err);
        }
    };
    replace_file(&tmp_path, path)?;
    Ok(value)
}

fn replace_file(tmp_path: &Path, path: &Path) -> Result<()> {
    match fs::rename(tmp_path, path) {
        Ok(()) => Ok(()),
        Err(_rename_err) if path.exists() => {
            fs::remove_file(path)?;
            fs::rename(tmp_path, path)?;
            Ok(())
        }
        Err(rename_err) => Err(rename_err.into()),
    }
}

fn graph_stat_i64(graph: Option<&Value>, key: &str) -> i64 {
    graph
        .and_then(|item| item.get("stats"))
        .and_then(|stats| stats.get(key))
        .and_then(Value::as_i64)
        .unwrap_or(0)
}

pub(crate) fn required_value(
    iter: &mut impl Iterator<Item = String>,
    flag: &str,
) -> Result<String> {
    iter.next()
        .ok_or_else(|| anyhow!("{flag} requires a value"))
        .map(|value| value.trim().to_string())
}

pub(crate) fn round2(value: f64) -> f64 {
    (value * 100.0).round() / 100.0
}

pub(crate) fn parse_bool(value: &str) -> bool {
    matches!(
        value.trim().to_ascii_lowercase().as_str(),
        "1" | "true" | "yes" | "on"
    )
}

pub fn run_data_engine_analysis_command(command: &str, args: Value) -> Result<Value> {
    let argv = data_engine_argv(args)?;
    match command {
        "query-stats-rows" => {
            let args = stats_query_store::parse_query_stats_rows_args(argv.into_iter())?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_stats_rows_to_json_writer(&args, writer)
                })?;
                return Ok(json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "group_key": result.group_key,
                    "total": result.total,
                    "row_count": result.row_count,
                    "output_json": output_json.display().to_string(),
                    "diagnostics": result.diagnostics,
                }));
            }
            let result = query_stats_rows(&args)?;
            let mut payload = Map::new();
            payload.insert("ok".to_string(), Value::Bool(true));
            payload.insert("case_id".to_string(), Value::String(args.case_id));
            payload.insert("group_key".to_string(), Value::String(result.group_key));
            payload.insert("total".to_string(), Value::from(result.total));
            payload.insert("row_summary".to_string(), result.row_summary);
            payload.insert("rows".to_string(), Value::Array(result.rows));
            if let Some(row_fields) = result.row_fields {
                payload.insert("row_fields".to_string(), row_fields);
            }
            payload.insert("diagnostics".to_string(), result.diagnostics);
            Ok(Value::Object(payload))
        }
        "query-stats-tree" => {
            let args = stats_query_store::parse_query_stats_tree_args(argv.into_iter())?;
            let groups = query_stats_tree(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "groups": groups,
            }))
        }
        "query-stats-date-range" => {
            let args = stats_query_store::parse_query_stats_date_range_args(argv.into_iter())?;
            let date_range = query_stats_date_range(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "date_range": date_range,
            }))
        }
        "query-stats-txn-rows" => {
            let args = stats_query_store::parse_query_stats_txn_rows_args(argv.into_iter())?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_stats_txn_rows_to_json_writer(&args, writer)
                })?;
                return Ok(json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "row_count": result.row_count,
                    "done": result.done,
                    "output_json": output_json.display().to_string(),
                    "diagnostics": result.diagnostics,
                }));
            }
            let result = query_stats_txn_rows(&args)?;
            let mut payload = Map::new();
            payload.insert("ok".to_string(), Value::Bool(true));
            payload.insert("case_id".to_string(), Value::String(args.case_id));
            payload.insert("rows".to_string(), Value::Array(result.rows));
            if let Some(row_fields) = result.row_fields {
                payload.insert("row_fields".to_string(), row_fields);
            }
            payload.insert("diagnostics".to_string(), result.diagnostics);
            if let Some(done) = result.done {
                payload.insert("done".to_string(), Value::Bool(done));
                payload.insert(
                    "nextCursor".to_string(),
                    result.next_cursor.unwrap_or(Value::Null),
                );
            }
            Ok(Value::Object(payload))
        }
        "query-chart-dashboard" => {
            let args = stats_query_store::parse_query_chart_dashboard_args(argv.into_iter())?;
            let dashboard = query_chart_dashboard(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "dashboard": dashboard,
            }))
        }
        "query-chart-detail-rows" => {
            let args = stats_query_store::parse_query_chart_detail_rows_args(argv.into_iter())?;
            if let Some(output_json) = args.output_json.as_ref() {
                let result = write_json_file_with(output_json, |writer| {
                    query_chart_detail_rows_to_json_writer(&args, writer)
                })?;
                return Ok(json!({
                    "ok": true,
                    "case_id": args.case_id,
                    "total": result.total,
                    "row_count": result.row_count,
                    "output_json": output_json.display().to_string(),
                }));
            }
            let result = query_chart_detail_rows(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "rows": result.rows,
                "total": result.total,
            }))
        }
        "query-chart-counterparties" => {
            let args = stats_query_store::parse_query_chart_counterparties_args(argv.into_iter())?;
            let counterparties = query_chart_counterparties(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "counterparties": counterparties,
            }))
        }
        "query-chart-distribution" => {
            let args = stats_query_store::parse_query_chart_distribution_args(argv.into_iter())?;
            let distribution = query_chart_distribution(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "distribution": distribution,
            }))
        }
        "query-chart-flow" => {
            let args = stats_query_store::parse_query_chart_flow_args(argv.into_iter())?;
            let flow = query_chart_flow(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "flow": flow,
            }))
        }
        "query-chart-heatmap" => {
            let args = stats_query_store::parse_query_chart_heatmap_args(argv.into_iter())?;
            let heatmap = query_chart_heatmap(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "heatmap": heatmap,
            }))
        }
        "query-chart-summary" => {
            let args = stats_query_store::parse_query_chart_summary_args(argv.into_iter())?;
            let summary = query_chart_summary(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "summary": summary,
            }))
        }
        "query-chart-trend" => {
            let args = stats_query_store::parse_query_chart_trend_args(argv.into_iter())?;
            let trend = query_chart_trend(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "trend": trend,
            }))
        }
        "query-flow-focus-rows" => {
            let args = stats_query_store::parse_query_flow_focus_rows_args(argv.into_iter())?;
            let rows = query_flow_focus_rows(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "rows": rows,
            }))
        }
        "query-flow-focus-graph" => {
            let args = stats_query_store::parse_query_flow_focus_graph_args(argv.into_iter())?;
            if let Some(output_json) = args.output_json.as_ref() {
                let graph = query_flow_focus_graph(&args)?;
                let has_graph = graph.is_some();
                let node_count = graph_stat_i64(graph.as_ref(), "node_count");
                let edge_count = graph_stat_i64(graph.as_ref(), "edge_count");
                write_json_file_with(output_json, |writer| {
                    serde_json::to_writer(writer, &json!({"graph": graph}))?;
                    Ok(())
                })?;
                return Ok(json!({
                    "ok": true,
                    "case_id": args.rows.case_id,
                    "has_graph": has_graph,
                    "node_count": node_count,
                    "edge_count": edge_count,
                    "output_json": output_json.display().to_string(),
                }));
            }
            let graph = query_flow_focus_graph(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.rows.case_id,
                "graph": graph,
            }))
        }
        "materialize-rule-txn-index" => {
            let args = rule_txn_index_store::parse_args(argv.into_iter())?;
            let result = materialize_rule_txn_index(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "row_count": result.row_count,
                "rebuilt": result.rebuilt,
                "agg_name": RULE_TXN_INDEX_CACHE_KEY,
                "agg_version": RULE_TXN_INDEX_VERSION,
            }))
        }
        "materialize-rule-pattern-index" => {
            let args = rule_pattern_args::parse_args(argv.into_iter())?;
            let result = materialize_rule_pattern_index(&args)?;
            Ok(json!({
                "ok": true,
                "case_id": args.case_id,
                "row_count": result.row_count,
                "rebuilt": result.rebuilt,
                "input_coverage": result.input_coverage,
                "feature_readiness": result.feature_readiness,
                "agg_name": RULE_PATTERN_CACHE_KEY,
                "agg_version": RULE_PATTERN_VERSION,
            }))
        }
        other => bail!("unsupported data engine analysis command: {other}"),
    }
}

pub fn run_data_engine_analysis_verify_command(command: &str, args: Value) -> Result<Value> {
    let argv = data_engine_argv(args)?;
    match command {
        "verify-txn-daily" => {
            let args = txn_daily_store::parse_verify_args(argv.into_iter())?;
            let result = verify_txn_daily(&args)?;
            Ok(verify_txn_daily_result_json(&result))
        }
        other => bail!("unsupported data engine analysis verify command: {other}"),
    }
}

fn verify_txn_daily_result_json(result: &txn_daily_store::VerifyTxnDailyResult) -> Value {
    json!({
        "ok": true,
        "case_id": &result.case_id,
        "source_revision": result.source_revision,
        "source_row_count": result.source_row_count,
        "accepted_row_count": result.accepted_row_count,
        "rejected_row_count": result.rejected_row_count,
        "duplicate_row_count": result.duplicate_row_count,
        "aggregate_row_count": result.aggregate_row_count,
        "materialization_identity": &result.materialization_identity,
        "result_signature": &result.result_signature,
        "producer_content_contract": &result.producer_content_contract,
        "producer_content_id": &result.producer_content_id,
        "producer_manifest_sha256": &result.producer_manifest_sha256,
        "duckdb_version": &result.duckdb_version,
    })
}

fn data_engine_argv(args: Value) -> Result<Vec<String>> {
    let object = args
        .as_object()
        .filter(|object| object.len() == 1 && object.contains_key("argv"))
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let items = object
        .get("argv")
        .and_then(Value::as_array)
        .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
    let mut argv = Vec::with_capacity(items.len());
    for item in items {
        let text = item
            .as_str()
            .ok_or_else(|| anyhow!("data_engine_request_contract_invalid"))?;
        argv.push(text.to_string());
    }
    Ok(argv)
}

pub fn run_data_engine_stats_query(command: &str, args: Value) -> Result<Value> {
    stats_query_store::run_data_engine_stats_query(command, args)
}

pub fn validate_data_engine_stats_query_args(command: &str, args: &Value) -> Result<()> {
    stats_query_store::validate_data_engine_stats_query_args(command, args)
}

// This host-private facade deliberately keeps the account-flow request and
// result types crate-private. The provider/MCP contract must never acquire the
// resolved account key, database path, or private evidence locators carried by
// this native frame.
pub fn validate_funds_analyze_account_flows_arguments(
    arguments: &Value,
    authoritative_case_id: &str,
) -> Result<()> {
    let parsed =
        stats_query_store::account_flow::parse_analyze_account_flows_host_arguments(arguments)?;
    if authoritative_case_id.is_empty()
        || authoritative_case_id != authoritative_case_id.trim()
        || authoritative_case_id.len() > 512
        || authoritative_case_id.chars().any(char::is_control)
        || parsed.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    stats_query_store::account_flow::validate_analyze_account_flows_host_arguments(&parsed)
}

// The Go host may call this fixed operation only while it owns the exact DSV2
// lease. Keeping the path as a call argument, rather than a serialized result,
// prevents the native response from turning into ambient database authority.
pub fn run_funds_analyze_account_flows(
    arguments: &Value,
    authoritative_case_id: &str,
    db_path: &Path,
) -> Result<Value> {
    validate_funds_analyze_account_flows_arguments(arguments, authoritative_case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let parsed =
        stats_query_store::account_flow::parse_analyze_account_flows_host_arguments(arguments)?;
    let conn = open_account_flow_readonly_connection(db_path)
        .map_err(|_| anyhow!("account_flow_snapshot_connection_invalid"))?;
    let result = stats_query_store::account_flow::analyze_account_flows(&conn, &parsed)?;
    serde_json::to_value(result).map_err(|_| anyhow!("account_flow_result_contract_invalid"))
}

pub fn validate_funds_direct_source_preview_arguments(
    arguments: &Value,
    authoritative_case_id: &str,
) -> Result<()> {
    let parsed =
        stats_query_store::direct_source_preview::parse_direct_source_preview_host_arguments(
            arguments,
        )?;
    if authoritative_case_id.is_empty()
        || authoritative_case_id != authoritative_case_id.trim()
        || authoritative_case_id.len() > 512
        || authoritative_case_id.chars().any(char::is_control)
        || parsed.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    stats_query_store::direct_source_preview::validate_direct_source_preview_host_arguments(&parsed)
}

pub fn run_funds_direct_source_preview(
    arguments: &Value,
    authoritative_case_id: &str,
    db_path: &Path,
) -> Result<Value> {
    validate_funds_direct_source_preview_arguments(arguments, authoritative_case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let parsed =
        stats_query_store::direct_source_preview::parse_direct_source_preview_host_arguments(
            arguments,
        )?;
    let conn = open_account_flow_readonly_connection(db_path)
        .map_err(|_| anyhow!("direct_source_preview_snapshot_connection_invalid"))?;
    let result = stats_query_store::direct_source_preview::direct_source_preview(&conn, &parsed)?;
    serde_json::to_value(result)
        .map_err(|_| anyhow!("direct_source_preview_result_contract_invalid"))
}

pub fn validate_funds_deterministic_cleaning_arguments(
    arguments: &Value,
    authoritative_case_id: &str,
) -> Result<()> {
    let parsed = stats_query_store::deterministic_cleaning::parse_arguments(arguments)?;
    if authoritative_case_id.is_empty()
        || authoritative_case_id != authoritative_case_id.trim()
        || authoritative_case_id.len() > 512
        || authoritative_case_id.chars().any(char::is_control)
        || parsed.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    stats_query_store::deterministic_cleaning::validate_arguments(&parsed)
}

pub fn run_funds_deterministic_cleaning(
    arguments: &Value,
    authoritative_case_id: &str,
    db_path: &Path,
    output: &mut impl Write,
) -> Result<Value> {
    validate_funds_deterministic_cleaning_arguments(arguments, authoritative_case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let parsed = stats_query_store::deterministic_cleaning::parse_arguments(arguments)?;
    let conn = open_account_flow_readonly_connection(db_path)
        .map_err(|_| anyhow!("deterministic_cleaning_snapshot_connection_invalid"))?;
    let result = stats_query_store::deterministic_cleaning::run(&conn, &parsed, output)?;
    serde_json::to_value(result)
        .map_err(|_| anyhow!("deterministic_cleaning_result_contract_invalid"))
}

pub fn validate_funds_resolve_account_ingress_arguments(
    arguments: &Value,
    authoritative_case_id: &str,
) -> Result<()> {
    let parsed = stats_query_store::account_ingress::parse_resolve_account_ingress_host_arguments(
        arguments,
    )?;
    if authoritative_case_id.is_empty()
        || authoritative_case_id != authoritative_case_id.trim()
        || authoritative_case_id.len() > 512
        || authoritative_case_id.chars().any(char::is_control)
        || parsed.case_id != authoritative_case_id
    {
        bail!("data_engine_context_mismatch");
    }
    stats_query_store::account_ingress::validate_resolve_account_ingress_host_arguments(&parsed)
}

// The complete candidates and inherited snapshot path remain confined to the
// host-private native invocation. The typed result contains only ordinals,
// closed dispositions, safe semantic labels, and exact snapshot provenance.
pub fn run_funds_resolve_account_ingress(
    arguments: &Value,
    authoritative_case_id: &str,
    db_path: &Path,
) -> Result<Value> {
    validate_funds_resolve_account_ingress_arguments(arguments, authoritative_case_id)?;
    if !db_path.is_absolute() {
        bail!("data_engine_request_contract_invalid");
    }
    let parsed = stats_query_store::account_ingress::parse_resolve_account_ingress_host_arguments(
        arguments,
    )?;
    let conn = open_account_flow_readonly_connection(db_path)
        .map_err(|_| anyhow!("account_ingress_snapshot_connection_invalid"))?;
    let result = stats_query_store::account_ingress::resolve_account_ingress(&conn, &parsed)?;
    serde_json::to_value(result).map_err(|_| anyhow!("account_ingress_result_contract_invalid"))
}

#[cfg(test)]
mod data_engine_contract_tests {
    use super::{
        data_engine_argv, run_funds_analyze_account_flows, run_funds_direct_source_preview,
        validate_funds_analyze_account_flows_arguments,
        validate_funds_direct_source_preview_arguments,
    };
    use serde_json::json;
    use std::path::Path;

    #[test]
    fn data_engine_argv_accepts_only_the_exact_versioned_shape() {
        assert_eq!(
            data_engine_argv(json!({"argv": ["--case-id", "case-a"]}))
                .expect("exact argv envelope"),
            vec!["--case-id".to_string(), "case-a".to_string()]
        );

        for invalid in [
            json!(["--case-id", "case-a"]),
            json!({"args": ["--case-id", "case-a"]}),
            json!({"argv": ["--case-id", "case-a"], "unexpected": true}),
            json!({"argv": "--case-id"}),
            json!({"argv": ["--case-id", 1]}),
        ] {
            let error = data_engine_argv(invalid).expect_err("malformed argv must fail");
            assert!(format!("{error:#}").contains("data_engine_request_contract_invalid"));
        }
    }

    fn account_flow_arguments() -> serde_json::Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "contextEpoch": 7,
            "contextDigest": "2".repeat(64),
            "caseBindingHash": "3".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "4".repeat(64)),
            "expectedProducerManifestSha256": "5".repeat(64),
            "subjectRef": format!("cer1_{}", "a".repeat(64)),
            "resolvedAccountKey": "private-account-key-a",
            "subjectResolutionDigest": "6".repeat(64),
            "startInclusive": "2026-01-01T00:00:00Z",
            "endInclusive": "2026-01-01T00:02:00Z",
            "evidenceRowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
            "scanCap": 100,
        })
    }

    fn direct_source_preview_arguments() -> serde_json::Value {
        json!({
            "caseId": "case-a",
            "datasetSnapshotId": format!("dsv2_{}", "1".repeat(64)),
            "caseBindingHash": "2".repeat(64),
            "expectedProducerContentId": format!("fpc1_{}", "3".repeat(64)),
            "expectedProducerManifestSha256": "4".repeat(64),
            "fields": ["account", "amountText", "currency"],
            "rowOffset": 0,
            "rowLimit": 10,
            "datasetUtcOffsetMinutes": 0,
            "expectedCurrency": "CNY",
            "minorUnitScale": 2,
        })
    }

    #[test]
    fn account_flow_facade_is_case_bound_and_rejects_ambient_authority_fields() {
        validate_funds_analyze_account_flows_arguments(&account_flow_arguments(), "case-a")
            .expect("strict host-private account-flow arguments");

        let mismatch =
            validate_funds_analyze_account_flows_arguments(&account_flow_arguments(), "case-b")
                .expect_err("outer case must be authoritative");
        assert_eq!(format!("{mismatch:#}"), "data_engine_context_mismatch");

        for field in ["sql", "dbPath", "authority", "unknown"] {
            let mut invalid = account_flow_arguments();
            invalid
                .as_object_mut()
                .expect("account-flow object")
                .insert(field.to_string(), json!("private-value"));
            let error = validate_funds_analyze_account_flows_arguments(&invalid, "case-a")
                .expect_err("ambient authority field must fail closed");
            assert_eq!(
                format!("{error:#}"),
                "account_flow_request_contract_invalid"
            );
        }
    }

    #[test]
    fn account_flow_facade_rejects_relative_database_path_without_echoing_private_values() {
        let arguments = account_flow_arguments();
        let error =
            run_funds_analyze_account_flows(&arguments, "case-a", Path::new("private/case.duckdb"))
                .expect_err("relative database path must fail before open");
        let message = format!("{error:#}");
        assert_eq!(message, "data_engine_request_contract_invalid");
        assert!(!message.contains("private-account-key-a"));
        assert!(!message.contains("case.duckdb"));
    }

    #[test]
    fn direct_source_preview_facade_is_case_bound_and_rejects_ambient_authority_fields() {
        validate_funds_direct_source_preview_arguments(
            &direct_source_preview_arguments(),
            "case-a",
        )
        .expect("strict host-private direct preview arguments");

        let mismatch = validate_funds_direct_source_preview_arguments(
            &direct_source_preview_arguments(),
            "case-b",
        )
        .expect_err("outer case must be authoritative");
        assert_eq!(format!("{mismatch:#}"), "data_engine_context_mismatch");

        for field in ["sql", "dbPath", "entityReference", "unknown"] {
            let mut invalid = direct_source_preview_arguments();
            invalid
                .as_object_mut()
                .expect("direct preview object")
                .insert(field.to_string(), json!("private-value"));
            let error = validate_funds_direct_source_preview_arguments(&invalid, "case-a")
                .expect_err("ambient authority field must fail closed");
            assert_eq!(
                format!("{error:#}"),
                "direct_source_preview_request_contract_invalid"
            );
        }
    }

    #[test]
    fn direct_source_preview_facade_rejects_relative_database_path_without_reflection() {
        let arguments = direct_source_preview_arguments();
        let error =
            run_funds_direct_source_preview(&arguments, "case-a", Path::new("private/case.duckdb"))
                .expect_err("relative database path must fail before open");
        let message = format!("{error:#}");
        assert_eq!(message, "data_engine_request_contract_invalid");
        assert!(!message.contains("case.duckdb"));
    }
}
