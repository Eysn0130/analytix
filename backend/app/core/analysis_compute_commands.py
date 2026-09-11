from __future__ import annotations

import json
import math
from pathlib import Path
from typing import Optional, Sequence


def _required_nonnegative_int(value: object, *, field: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise ValueError(f"{field}_invalid")
    return value


def _required_positive_int(value: object, *, field: str) -> int:
    normalized = _required_nonnegative_int(value, field=field)
    if normalized < 1:
        raise ValueError(f"{field}_invalid")
    return normalized


def _required_nonnegative_finite_float(value: object, *, field: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{field}_invalid")
    normalized = float(value)
    if not math.isfinite(normalized) or normalized < 0:
        raise ValueError(f"{field}_invalid")
    return normalized


def materialize_txn_daily_args(
    *,
    case_id: str,
    db_path: Path,
    source_revision: int,
    source_snapshot: tuple[int, str, int],
) -> list[str]:
    row_count, max_txn_ts, max_id = source_snapshot
    return [
        "materialize-txn-daily",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--source-revision",
        str(_required_positive_int(source_revision, field="source_revision")),
        "--source-row-count",
        str(_required_nonnegative_int(row_count, field="source_row_count")),
        "--source-max-txn-ts",
        str(max_txn_ts or ""),
        "--source-max-id",
        str(_required_nonnegative_int(max_id, field="source_max_id")),
    ]


def query_stats_rows_args(
    *,
    case_id: str,
    db_path: Path,
    mode: str,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    search_text: str,
    row_sort_col: str,
    row_sort_dir: str,
    row_offset: int,
    row_limit: int,
    row_format: str = "object",
    fields: Sequence[str] = (),
    output_json: Optional[Path] = None,
) -> list[str]:
    args = [
        "query-stats-rows",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--mode",
        _clean(mode),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--search-text",
        _clean(search_text),
        "--row-sort-col",
        _clean(row_sort_col),
        "--row-sort-dir",
        _clean(row_sort_dir or "desc"),
        "--row-offset",
        str(_required_nonnegative_int(row_offset, field="row_offset")),
        "--row-limit",
        str(_required_nonnegative_int(row_limit, field="row_limit")),
    ]
    if _clean(row_format) and _clean(row_format) != "object":
        args.extend(["--row-format", _clean(row_format)])
    _extend_repeatable(args, "--selected-key", selected_keys)
    _extend_repeatable(args, "--field", fields)
    if output_json is not None:
        args.extend(["--output-json", str(output_json)])
    return args


def query_stats_tree_args(
    *,
    case_id: str,
    db_path: Path,
    tab: str,
) -> list[str]:
    return [
        "query-stats-tree",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--tab",
        _clean(tab or "byName") or "byName",
    ]


def query_stats_date_range_args(
    *,
    case_id: str,
    db_path: Path,
) -> list[str]:
    return [
        "query-stats-date-range",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
    ]


def query_stats_txn_rows_args(
    *,
    case_id: str,
    db_path: Path,
    key_type: str,
    key_value: str,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    direction: str,
    sort_col: str,
    sort_dir: str,
    limit: int,
    cursor: Optional[dict],
    key_values: Sequence[str] = (),
    start_time: str = "",
    end_time: str = "",
    row_format: str = "object",
    fields: Sequence[str] = (),
    output_json: Optional[Path] = None,
) -> list[str]:
    key_value_args: list[str] = []
    normalized_key_values = _clean_key_values(key_values)
    primary_key_value = _clean(key_value)
    if normalized_key_values and primary_key_value and primary_key_value not in normalized_key_values:
        normalized_key_values.insert(0, primary_key_value)
    if normalized_key_values:
        for value in normalized_key_values:
            key_value_args.extend(["--key-value", value])
    else:
        key_value_args.extend(["--key-value", primary_key_value])
    args = [
        "query-stats-txn-rows",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--key-type",
        _clean(key_type or "account") or "account",
        *key_value_args,
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--direction",
        _clean(direction or "all"),
        "--sort-col",
        _clean(sort_col or "txn_time") or "txn_time",
        "--sort-dir",
        _clean(sort_dir or "asc"),
        "--limit",
        str(_required_nonnegative_int(limit, field="limit")),
    ]
    if _clean(start_time):
        args.extend(["--start-time", _clean(start_time)])
    if _clean(end_time):
        args.extend(["--end-time", _clean(end_time)])
    if _clean(row_format) and _clean(row_format) != "object":
        args.extend(["--row-format", _clean(row_format)])
    _extend_repeatable(args, "--selected-key", selected_keys)
    _extend_repeatable(args, "--field", fields)
    if cursor is not None:
        args.extend(
            ["--cursor-json", json.dumps(cursor, ensure_ascii=False, separators=(",", ":"))]
        )
    if output_json is not None:
        args.extend(["--output-json", str(output_json)])
    return args


def query_chart_dashboard_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    metric_mode: str,
    direction_mode: str,
    granularity: str,
    success_filter: str,
    cash_filter: str,
    selection_mode: str,
    large_txn_threshold: float,
    chart_filters: Sequence[dict] = (),
) -> list[str]:
    args = [
        "query-chart-dashboard",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--metric-mode",
        _clean(metric_mode or "amount"),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--granularity",
        _clean(granularity or "day"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--selection-mode",
        _clean(selection_mode),
        "--large-txn-threshold",
        str(
            _required_nonnegative_finite_float(
                large_txn_threshold,
                field="large_txn_threshold",
            )
        ),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    active_chart_filters = [
        token for token in (chart_filters or []) if isinstance(token, dict) and _clean(token.get("dimension"))
    ]
    if active_chart_filters:
        args.extend(
            [
                "--chart-filters-json",
                json.dumps(active_chart_filters, ensure_ascii=False, separators=(",", ":")),
            ]
        )
    return args


def query_chart_detail_rows_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    direction_mode: str,
    success_filter: str,
    cash_filter: str,
    chart_filters: Sequence[dict],
    sort_col: str,
    sort_dir: str,
    page: int,
    limit: int,
    output_json: Optional[Path] = None,
) -> list[str]:
    args = [
        "query-chart-detail-rows",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--sort-col",
        _clean(sort_col or "txn_time") or "txn_time",
        "--sort-dir",
        _clean(sort_dir or "asc"),
        "--page",
        str(_required_positive_int(page, field="page")),
        "--limit",
        str(min(_required_positive_int(limit, field="limit"), 5000)),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    active_chart_filters = [
        token for token in (chart_filters or []) if isinstance(token, dict) and _clean(token.get("dimension"))
    ]
    if active_chart_filters:
        args.extend(
            [
                "--chart-filters-json",
                json.dumps(active_chart_filters, ensure_ascii=False, separators=(",", ":")),
            ]
        )
    if output_json is not None:
        args.extend(["--output-json", str(output_json)])
    return args


def query_chart_counterparties_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    metric_mode: str,
    direction_mode: str,
    success_filter: str,
    cash_filter: str,
) -> list[str]:
    args = [
        "query-chart-counterparties",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--metric-mode",
        _clean(metric_mode or "amount"),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_chart_distribution_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    metric_mode: str,
    direction_mode: str,
    success_filter: str,
    cash_filter: str,
    selection_mode: str,
) -> list[str]:
    args = [
        "query-chart-distribution",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--metric-mode",
        _clean(metric_mode or "amount"),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--selection-mode",
        _clean(selection_mode),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_chart_flow_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    direction: str,
    success_filter: str,
    cash_filter: str,
    selection_mode: str,
) -> list[str]:
    args = [
        "query-chart-flow",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--direction",
        _clean(direction or "all"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--selection-mode",
        _clean(selection_mode),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_chart_heatmap_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    metric_mode: str,
    direction_mode: str,
    success_filter: str,
    cash_filter: str,
) -> list[str]:
    args = [
        "query-chart-heatmap",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--metric-mode",
        _clean(metric_mode or "amount"),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_chart_summary_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    success_filter: str,
    cash_filter: str,
    selection_mode: str,
    large_txn_threshold: float,
) -> list[str]:
    args = [
        "query-chart-summary",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--selection-mode",
        _clean(selection_mode),
        "--large-txn-threshold",
        str(
            _required_nonnegative_finite_float(
                large_txn_threshold,
                field="large_txn_threshold",
            )
        ),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_chart_trend_args(
    *,
    case_id: str,
    db_path: Path,
    selected_keys: Sequence[str],
    date_start: str,
    date_end: str,
    metric_mode: str,
    direction_mode: str,
    granularity: str,
    success_filter: str,
    cash_filter: str,
    selection_mode: str,
) -> list[str]:
    args = [
        "query-chart-trend",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--date-start",
        _clean(date_start),
        "--date-end",
        _clean(date_end),
        "--metric-mode",
        _clean(metric_mode or "amount"),
        "--direction-mode",
        _clean(direction_mode or "all"),
        "--granularity",
        _clean(granularity or "day"),
        "--success-filter",
        _clean(success_filter or "all"),
        "--cash-filter",
        _clean(cash_filter or "all"),
        "--selection-mode",
        _clean(selection_mode),
    ]
    _extend_repeatable(args, "--selected-key", selected_keys)
    return args


def query_flow_focus_rows_args(
    *,
    case_id: str,
    db_path: Path,
    query_seed_ids: Sequence[str],
    selected_focus_ids: Sequence[str],
    selected_placeholder_kinds: Sequence[str],
    include_missing_counterparty: bool,
    direction: str,
    min_amount: float,
    date_start: str,
    date_end_excl: str,
) -> list[str]:
    args = [
        "query-flow-focus-rows",
        "--case-id",
        _clean(case_id),
        "--db-path",
        str(db_path),
        "--include-missing-counterparty",
        "true" if include_missing_counterparty else "false",
        "--direction",
        _clean(direction or "all"),
        "--min-amount",
        str(_required_nonnegative_finite_float(min_amount, field="min_amount")),
        "--date-start",
        _clean(date_start),
        "--date-end-excl",
        _clean(date_end_excl),
    ]
    _extend_repeatable(args, "--query-seed-id", query_seed_ids)
    _extend_repeatable(args, "--selected-focus-id", selected_focus_ids)
    _extend_repeatable(args, "--selected-placeholder-kind", selected_placeholder_kinds)
    return args


def query_flow_focus_graph_args(
    *,
    case_id: str,
    db_path: Path,
    query_seed_ids: Sequence[str],
    seed_ids: Sequence[str],
    selected_focus_ids: Sequence[str],
    selected_placeholder_kinds: Sequence[str],
    include_missing_counterparty: bool,
    direction: str,
    min_amount: float,
    date_start: str,
    date_end_excl: str,
    depth: int,
    view_mode: str,
    focus_id_raw: str,
    focus_label: str,
    request_id: str,
    source: str,
    focus_key_type: str,
    expected_total_amount: Optional[float],
    expected_row_count: Optional[int],
    output_json: Optional[Path] = None,
) -> list[str]:
    args = query_flow_focus_rows_args(
        case_id=case_id,
        db_path=db_path,
        query_seed_ids=query_seed_ids,
        selected_focus_ids=selected_focus_ids,
        selected_placeholder_kinds=selected_placeholder_kinds,
        include_missing_counterparty=include_missing_counterparty,
        direction=direction,
        min_amount=min_amount,
        date_start=date_start,
        date_end_excl=date_end_excl,
    )
    args[0] = "query-flow-focus-graph"
    args.extend(
        [
            "--depth",
            str(_required_positive_int(depth, field="depth")),
            "--view-mode",
            _clean(view_mode or "relation") or "relation",
            "--focus-id-raw",
            _clean(focus_id_raw),
            "--focus-label",
            _clean(focus_label),
            "--request-id",
            _clean(request_id),
            "--source",
            _clean(source),
            "--focus-key-type",
            _clean(focus_key_type),
        ]
    )
    _extend_repeatable(args, "--seed-id", seed_ids)
    if expected_total_amount is not None:
        args.extend(
            [
                "--expected-total-amount",
                str(
                    _required_nonnegative_finite_float(
                        expected_total_amount,
                        field="expected_total_amount",
                    )
                ),
            ]
        )
    if expected_row_count is not None:
        args.extend(
            [
                "--expected-row-count",
                str(
                    _required_nonnegative_int(
                        expected_row_count,
                        field="expected_row_count",
                    )
                ),
            ]
        )
    if output_json is not None:
        args.extend(["--output-json", str(output_json)])
    return args


def project_flow_skeleton_clusters_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-skeleton-clusters",
        "--input-path",
        str(input_path),
    ]


def project_flow_projection_layout_sync_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-projection-layout-sync",
        "--input-path",
        str(input_path),
    ]


def project_flow_projection_layout_seed_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-projection-layout-seed",
        "--input-path",
        str(input_path),
    ]


def project_flow_graph_render_plan_args(*, input_path: Path, output_json: Optional[Path] = None) -> list[str]:
    args = [
        "project-flow-graph-render-plan",
        "--input-path",
        str(input_path),
    ]
    if output_json is not None:
        args.extend(["--output-json", str(output_json)])
    return args


def project_flow_layout_network_sector_placement_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-layout-network-sector-placement",
        "--input-path",
        str(input_path),
    ]


def project_flow_layout_network_plan_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-layout-network-plan",
        "--input-path",
        str(input_path),
    ]


def project_flow_layout_network_community_quality_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-layout-network-community-quality",
        "--input-path",
        str(input_path),
    ]


def project_flow_graph_data_merge_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-graph-data-merge",
        "--input-path",
        str(input_path),
    ]


def project_flow_graph_search_args(*, input_path: Path) -> list[str]:
    return [
        "project-flow-graph-search",
        "--input-path",
        str(input_path),
    ]


def project_layout_node_plan_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("project-layout-node-plan", input_path=input_path)


def project_layout_cache_key_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("project-layout-cache-key", input_path=input_path)


def apply_layout_node_plan_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("apply-layout-node-plan", input_path=input_path)


def apply_layout_worker_node_plan_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("apply-layout-worker-node-plan", input_path=input_path)


def clear_layout_node_meta_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("clear-layout-node-meta", input_path=input_path)


def project_layout_role_graph_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("project-layout-role-graph", input_path=input_path)


def project_analysis_graph_data_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("project-analysis-graph-data", input_path=input_path)


def merge_flow_same_name_graph_args(*, input_path: Path) -> list[str]:
    return _layout_node_plan_args("merge-flow-same-name-graph", input_path=input_path)


def _layout_node_plan_args(command: str, *, input_path: Path) -> list[str]:
    return [
        command,
        "--input-path",
        str(input_path),
    ]


def _extend_repeatable(args: list[str], flag: str, values: Sequence[str]) -> None:
    for item in values or []:
        value = _clean(item)
        if value:
            args.extend([flag, value])


def _clean_key_values(values: Sequence[str]) -> list[str]:
    out: list[str] = []
    for item in values or []:
        value = _clean(item)
        if value in out:
            continue
        out.append(value)
    return out


def _clean(value: object) -> str:
    return str(value or "").strip()
