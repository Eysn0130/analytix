from __future__ import annotations

import json
import math
import shutil
import tempfile
from pathlib import Path
from typing import Any, Callable, Optional, Sequence

from app.core.analysis_compute_commands import (
    apply_layout_node_plan_args,
    apply_layout_worker_node_plan_args,
    clear_layout_node_meta_args,
    materialize_txn_daily_args,
    merge_flow_same_name_graph_args,
    project_analysis_graph_data_args,
    project_flow_graph_data_merge_args,
    project_flow_graph_search_args,
    project_layout_cache_key_args,
    project_flow_graph_render_plan_args,
    project_flow_layout_network_community_quality_args,
    project_flow_layout_network_sector_placement_args,
    project_flow_layout_network_plan_args,
    project_flow_projection_layout_seed_args,
    project_flow_projection_layout_sync_args,
    project_layout_node_plan_args,
    project_layout_role_graph_args,
    project_flow_skeleton_clusters_args,
    query_chart_dashboard_args,
    query_chart_detail_rows_args,
    query_flow_focus_graph_args,
    query_flow_focus_rows_args,
    query_stats_rows_args,
    query_stats_txn_rows_args,
)
from app.core.analysis_compute_rule_commands import (
    materialize_rule_pattern_index_args,
    materialize_rule_txn_index_args,
)
from app.core.analysis_compute_diagnostics import (
    AnalysisComputeDiagnostics,
    merge_analysis_compute_diagnostics,
)
from app.core.analysis_compute_runner import (
    AnalysisComputeUnavailableError,
    require_analysis_compute_capability,
    run_analysis_compute,
)
from app.core.analysis_compute_worker import (
    query_stats_date_range_worker,
    query_stats_rows_worker,
    query_stats_tree_worker,
    query_stats_txn_rows_worker,
)
from app.core.db_engine import copy_duckdb_database_snapshot_from_active_connection


def materialize_txn_daily(
    *,
    case_id: str,
    db_path: Path,
    source_revision: int,
    source_snapshot: tuple[int, str, int],
) -> dict:
    payload = run_analysis_compute(
        materialize_txn_daily_args(
            case_id=case_id,
            db_path=db_path,
            source_revision=source_revision,
            source_snapshot=source_snapshot,
        )
    )
    if not (payload and payload.get("ok") is True):
        raise AnalysisComputeUnavailableError("Rust analysis compute materialize-txn-daily failed")
    return payload


def try_materialize_txn_daily(
    *,
    case_id: str,
    db_path: Path,
    source_revision: int,
    source_snapshot: tuple[int, str, int],
) -> Optional[dict]:
    try:
        return materialize_txn_daily(
            case_id=case_id,
            db_path=db_path,
            source_revision=source_revision,
            source_snapshot=source_snapshot,
        )
    except AnalysisComputeUnavailableError:
        return None


def query_stats_rows(
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
) -> dict:
    payload = _run_stats_worker_with_readonly_db_snapshot(
        lambda active_db_path: query_stats_rows_worker(
            case_id=case_id,
            db_path=active_db_path,
            mode=mode,
            selected_keys=selected_keys,
            date_start=date_start,
            date_end=date_end,
            search_text=search_text,
            row_sort_col=row_sort_col,
            row_sort_dir=row_sort_dir,
            row_offset=row_offset,
            row_limit=row_limit,
            row_format=row_format,
            fields=fields,
            include_diagnostics=True,
        ),
        db_path,
        include_diagnostics=True,
    )
    if not (payload and payload.get("ok") is True and isinstance(payload.get("rows"), list)):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-rows failed")
    return payload


def query_stats_rows_to_file(
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
    output_json: Path,
    row_format: str = "object",
    fields: Sequence[str] = (),
) -> dict:
    args = query_stats_rows_args(
        case_id=case_id,
        db_path=db_path,
        mode=mode,
        selected_keys=selected_keys,
        date_start=date_start,
        date_end=date_end,
        search_text=search_text,
        row_sort_col=row_sort_col,
        row_sort_dir=row_sort_dir,
        row_offset=row_offset,
        row_limit=row_limit,
        row_format=row_format,
        fields=fields,
        output_json=output_json,
    )
    payload = _run_analysis_compute_with_readonly_db_snapshot(args, db_path, include_diagnostics=True)
    if not (
        payload
        and payload.get("ok") is True
        and output_json.is_file()
    ):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-rows output failed")
    return payload


def query_stats_tree(
    *,
    case_id: str,
    db_path: Path,
    tab: str,
) -> dict:
    payload = _run_stats_worker_with_readonly_db_snapshot(
        lambda active_db_path: query_stats_tree_worker(
            case_id=case_id,
            db_path=active_db_path,
            tab=tab,
            include_diagnostics=True,
        ),
        db_path,
        include_diagnostics=True,
    )
    if not (payload and payload.get("ok") is True and isinstance(payload.get("groups"), list)):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-tree failed")
    return payload


def query_stats_date_range(
    *,
    case_id: str,
    db_path: Path,
) -> dict:
    payload = _run_stats_worker_with_readonly_db_snapshot(
        lambda active_db_path: query_stats_date_range_worker(
            case_id=case_id,
            db_path=active_db_path,
            include_diagnostics=True,
        ),
        db_path,
        include_diagnostics=True,
    )
    if not (
        payload
        and payload.get("ok") is True
        and isinstance(payload.get("date_range"), dict)
    ):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-date-range failed")
    return payload


def query_stats_txn_rows(
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
) -> dict:
    payload = _run_stats_worker_with_readonly_db_snapshot(
        lambda active_db_path: query_stats_txn_rows_worker(
            case_id=case_id,
            db_path=active_db_path,
            key_type=key_type,
            key_value=key_value,
            key_values=key_values,
            selected_keys=selected_keys,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            start_time=start_time,
            end_time=end_time,
            row_format=row_format,
            fields=fields,
            include_diagnostics=True,
        ),
        db_path,
        include_diagnostics=True,
    )
    if not (payload and payload.get("ok") is True and isinstance(payload.get("rows"), list)):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-txn-rows failed")
    return payload


def query_stats_txn_rows_to_file(
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
    output_json: Path,
    key_values: Sequence[str] = (),
    start_time: str = "",
    end_time: str = "",
    row_format: str = "object",
    fields: Sequence[str] = (),
) -> dict:
    args = query_stats_txn_rows_args(
        case_id=case_id,
        db_path=db_path,
        key_type=key_type,
        key_value=key_value,
        key_values=key_values,
        selected_keys=selected_keys,
        date_start=date_start,
        date_end=date_end,
        direction=direction,
        sort_col=sort_col,
        sort_dir=sort_dir,
        limit=limit,
        cursor=cursor,
        start_time=start_time,
        end_time=end_time,
        row_format=row_format,
        fields=fields,
        output_json=output_json,
    )
    payload = _run_analysis_compute_with_readonly_db_snapshot(args, db_path, include_diagnostics=True)
    if not (
        payload
        and payload.get("ok") is True
        and output_json.is_file()
    ):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-stats-txn-rows output failed")
    return payload


def try_query_stats_txn_rows(
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
) -> Optional[dict]:
    try:
        return query_stats_txn_rows(
            case_id=case_id,
            db_path=db_path,
            key_type=key_type,
            key_value=key_value,
            key_values=key_values,
            selected_keys=selected_keys,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            start_time=start_time,
            end_time=end_time,
            row_format=row_format,
            fields=fields,
        )
    except AnalysisComputeUnavailableError:
        return None


def _run_analysis_compute_with_readonly_db_snapshot(
    args: list[str],
    db_path: Path,
    *,
    include_diagnostics: bool = False,
) -> Optional[dict]:
    snapshot_diagnostics = AnalysisComputeDiagnostics() if include_diagnostics else None
    primary_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
    try:
        return run_analysis_compute(args, include_diagnostics=include_diagnostics)
    except AnalysisComputeUnavailableError as exc:
        if not _is_duckdb_lock_error(exc):
            raise
        if snapshot_diagnostics is not None:
            snapshot_diagnostics.record_elapsed("primary.locked_run", primary_started)
    with tempfile.TemporaryDirectory(prefix="analytix-analysis-compute-db-") as temp_dir:
        snapshot_path = Path(temp_dir) / Path(db_path).name
        copy_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
        copy_mode = _copy_readonly_db_snapshot(db_path, snapshot_path)
        if snapshot_diagnostics is not None:
            snapshot_diagnostics.record_metric("snapshot_copy_mode", copy_mode)
            snapshot_diagnostics.record_elapsed("snapshot.copy", copy_started)
        fallback_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
        payload = run_analysis_compute(
            _replace_arg_value(args, "--db-path", str(snapshot_path)),
            include_diagnostics=include_diagnostics,
        )
        if snapshot_diagnostics is not None and isinstance(payload, dict):
            snapshot_diagnostics.record_elapsed("snapshot.run", fallback_started)
            snapshot_diagnostics.finish()
            merge_analysis_compute_diagnostics(
                payload,
                section="readonly_snapshot",
                diagnostics=snapshot_diagnostics,
            )
        return payload


def _run_stats_worker_with_readonly_db_snapshot(
    run_with_db_path: Callable[[Path], Optional[dict]],
    db_path: Path,
    *,
    include_diagnostics: bool = False,
) -> Optional[dict]:
    snapshot_diagnostics = AnalysisComputeDiagnostics() if include_diagnostics else None
    primary_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
    try:
        return run_with_db_path(db_path)
    except AnalysisComputeUnavailableError as exc:
        if not _is_duckdb_lock_error(exc):
            raise
        if snapshot_diagnostics is not None:
            snapshot_diagnostics.record_elapsed("primary.locked_worker", primary_started)
    with tempfile.TemporaryDirectory(prefix="analytix-analysis-compute-db-") as temp_dir:
        snapshot_path = Path(temp_dir) / Path(db_path).name
        copy_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
        copy_mode = _copy_readonly_db_snapshot(db_path, snapshot_path)
        if snapshot_diagnostics is not None:
            snapshot_diagnostics.record_metric("snapshot_copy_mode", copy_mode)
            snapshot_diagnostics.record_elapsed("snapshot.copy", copy_started)
        worker_started = snapshot_diagnostics.mark() if snapshot_diagnostics is not None else 0.0
        payload = run_with_db_path(snapshot_path)
        if snapshot_diagnostics is not None and isinstance(payload, dict):
            snapshot_diagnostics.record_elapsed("snapshot.worker_run", worker_started)
            snapshot_diagnostics.finish()
            merge_analysis_compute_diagnostics(
                payload,
                section="readonly_snapshot",
                diagnostics=snapshot_diagnostics,
            )
        return payload


def _copy_readonly_db_snapshot(db_path: Path, snapshot_path: Path) -> str:
    require_analysis_compute_capability()
    if copy_duckdb_database_snapshot_from_active_connection(db_path, snapshot_path):
        return "active_connection_copy"
    try:
        shutil.copy2(db_path, snapshot_path)
        return "file_copy"
    except PermissionError:
        if copy_duckdb_database_snapshot_from_active_connection(db_path, snapshot_path):
            return "active_connection_copy"
        raise


def _is_duckdb_lock_error(exc: AnalysisComputeUnavailableError) -> bool:
    message = str(exc)
    return (
        "Could not set lock on file" in message
        or "Conflicting lock is held" in message
        or "Failure while replaying WAL file" in message
        or "File is already open" in message
        or "Cannot open file" in message
        or "另一个程序正在使用此文件" in message
    )


def _replace_arg_value(args: list[str], flag: str, value: str) -> list[str]:
    out = list(args)
    try:
        index = out.index(flag)
    except ValueError:
        return out
    if index + 1 < len(out):
        out[index + 1] = value
    return out


def query_chart_dashboard(
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
) -> dict:
    payload = _run_analysis_compute_with_readonly_db_snapshot(
        query_chart_dashboard_args(
            case_id=case_id,
            db_path=db_path,
            selected_keys=selected_keys,
            date_start=date_start,
            date_end=date_end,
            metric_mode=metric_mode,
            direction_mode=direction_mode,
            granularity=granularity,
            success_filter=success_filter,
            cash_filter=cash_filter,
            selection_mode=selection_mode,
            large_txn_threshold=large_txn_threshold,
            chart_filters=chart_filters,
        ),
        db_path,
    )
    if not (payload and payload.get("ok") is True and isinstance(payload.get("dashboard"), dict)):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-chart-dashboard failed")
    return payload


def query_chart_detail_rows_to_file(
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
    output_json: Path,
) -> dict:
    args = query_chart_detail_rows_args(
        case_id=case_id,
        db_path=db_path,
        selected_keys=selected_keys,
        date_start=date_start,
        date_end=date_end,
        direction_mode=direction_mode,
        success_filter=success_filter,
        cash_filter=cash_filter,
        chart_filters=chart_filters,
        sort_col=sort_col,
        sort_dir=sort_dir,
        page=page,
        limit=limit,
        output_json=output_json,
    )
    payload = _run_analysis_compute_with_readonly_db_snapshot(args, db_path)
    if not (
        payload
        and payload.get("ok") is True
        and isinstance(payload.get("total"), int)
        and output_json.is_file()
    ):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-chart-detail-rows output failed")
    return payload


def query_flow_focus_rows(
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
) -> dict:
    payload = run_analysis_compute(
        query_flow_focus_rows_args(
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
    )
    if not (payload and payload.get("ok") is True and isinstance(payload.get("rows"), list)):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-flow-focus-rows failed")
    return payload


def try_query_flow_focus_rows(
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
) -> Optional[dict]:
    try:
        return query_flow_focus_rows(
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
    except AnalysisComputeUnavailableError:
        return None


def query_flow_focus_graph_to_file(
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
    output_json: Path,
) -> dict:
    payload = run_analysis_compute(
        query_flow_focus_graph_args(
            case_id=case_id,
            db_path=db_path,
            query_seed_ids=query_seed_ids,
            seed_ids=seed_ids,
            selected_focus_ids=selected_focus_ids,
            selected_placeholder_kinds=selected_placeholder_kinds,
            include_missing_counterparty=include_missing_counterparty,
            direction=direction,
            min_amount=min_amount,
            date_start=date_start,
            date_end_excl=date_end_excl,
            depth=depth,
            view_mode=view_mode,
            focus_id_raw=focus_id_raw,
            focus_label=focus_label,
            request_id=request_id,
            source=source,
            focus_key_type=focus_key_type,
            expected_total_amount=expected_total_amount,
            expected_row_count=expected_row_count,
            output_json=output_json,
        )
    )
    if not (payload and payload.get("ok") is True):
        raise AnalysisComputeUnavailableError("Rust analysis compute query-flow-focus-graph failed")
    if payload.get("has_graph") is not True or not output_json.is_file():
        raise AnalysisComputeUnavailableError("Rust analysis compute query-flow-focus-graph returned invalid graph")
    return payload


def project_flow_skeleton_clusters(
    *,
    nodes: Sequence[dict[str, Any]],
    edges: Sequence[dict[str, Any]],
    request_context: dict[str, Any],
    base_projection: Optional[dict[str, Any]],
    tile_size: int,
) -> dict:
    require_analysis_compute_capability()
    payload = {
        "nodes": list(nodes or []),
        "edges": list(edges or []),
        "request_context": request_context if isinstance(request_context, dict) else {},
        "base_projection": base_projection if isinstance(base_projection, dict) else {},
        "tile_size": max(1, int(tile_size or 1)),
    }
    with tempfile.TemporaryDirectory(prefix="analytix-flow-projection-") as temp_dir:
        input_path = Path(temp_dir) / "projection-input.json"
        input_path.write_text(
            json.dumps(payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(project_flow_skeleton_clusters_args(input_path=input_path))
    if not (result and result.get("ok") is True and isinstance(result.get("projection"), dict)):
        raise AnalysisComputeUnavailableError("Rust analysis compute project-flow-skeleton-clusters failed")
    return result["projection"]


def project_flow_graph_render_plan(
    *,
    render_plans: Optional[Sequence[dict[str, Any]]] = None,
    render_plan: Optional[dict[str, Any]] = None,
    viewport_expand_targets: Optional[dict[str, Any]] = None,
) -> dict:
    require_analysis_compute_capability()
    payload: dict[str, Any] = {}
    if render_plans:
        payload["renderPlans"] = list(render_plans or [])
    elif isinstance(render_plan, dict):
        payload["renderPlan"] = render_plan
    elif render_plans is not None:
        payload["renderPlans"] = []
    if isinstance(viewport_expand_targets, dict):
        payload["viewportExpandTargets"] = viewport_expand_targets
    with tempfile.TemporaryDirectory(prefix="analytix-flow-render-plan-") as temp_dir:
        input_path = Path(temp_dir) / "render-plan-input.json"
        output_path = Path(temp_dir) / "render-plan-output.json"
        input_path.write_text(
            json.dumps(payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(
            project_flow_graph_render_plan_args(input_path=input_path, output_json=output_path)
        )
        if not (result and result.get("ok") is True and output_path.is_file()):
            raise AnalysisComputeUnavailableError("Rust analysis compute project-flow-graph-render-plan failed")
        try:
            payload_out = json.loads(output_path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as exc:
            raise AnalysisComputeUnavailableError("Rust analysis compute project-flow-graph-render-plan failed") from exc
    if not (isinstance(payload_out, dict) and isinstance(payload_out.get("render"), dict)):
        raise AnalysisComputeUnavailableError("Rust analysis compute project-flow-graph-render-plan failed")
    return payload_out["render"]


def project_flow_layout_network_sector_placement(
    *,
    payload: dict[str, Any],
) -> dict:
    require_analysis_compute_capability()
    request_payload = _json_safe_payload(payload if isinstance(payload, dict) else {})
    with tempfile.TemporaryDirectory(prefix="analytix-network-sector-placement-") as temp_dir:
        input_path = Path(temp_dir) / "network-sector-placement-input.json"
        input_path.write_text(
            json.dumps(request_payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(
            project_flow_layout_network_sector_placement_args(input_path=input_path)
        )
    if not (result and result.get("ok") is True and isinstance(result.get("sectorPlacement"), dict)):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-layout-network-sector-placement failed"
        )
    return result["sectorPlacement"]


def project_flow_layout_network_plan(
    *,
    payload: dict[str, Any],
) -> dict:
    require_analysis_compute_capability()
    request_payload = _json_safe_payload(payload if isinstance(payload, dict) else {})
    with tempfile.TemporaryDirectory(prefix="analytix-network-layout-plan-") as temp_dir:
        input_path = Path(temp_dir) / "network-layout-plan-input.json"
        input_path.write_text(
            json.dumps(request_payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(project_flow_layout_network_plan_args(input_path=input_path))
    if not (result and result.get("ok") is True and isinstance(result.get("networkPlan"), dict)):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-layout-network-plan failed"
        )
    network_plan = result["networkPlan"]
    if not isinstance(network_plan.get("nodeUpdates"), list) or not isinstance(
        network_plan.get("quality"), dict
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-layout-network-plan returned invalid plan"
        )
    return network_plan


def project_flow_layout_network_community_quality(
    *,
    payload: dict[str, Any],
) -> dict:
    require_analysis_compute_capability()
    request_payload = _json_safe_payload(payload if isinstance(payload, dict) else {})
    with tempfile.TemporaryDirectory(prefix="analytix-network-community-quality-") as temp_dir:
        input_path = Path(temp_dir) / "network-community-quality-input.json"
        input_path.write_text(
            json.dumps(request_payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(
            project_flow_layout_network_community_quality_args(input_path=input_path)
        )
    if not (
        result
        and result.get("ok") is True
        and isinstance(result.get("networkCommunityQuality"), dict)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-layout-network-community-quality failed"
        )
    network_community_quality = result["networkCommunityQuality"]
    if not isinstance(network_community_quality.get("communityQuality"), list) or not isinstance(
        network_community_quality.get("report"), dict
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-layout-network-community-quality returned invalid result"
        )
    return network_community_quality


def project_layout_node_plan(
    *,
    nodes: Sequence[dict[str, Any]],
    meta_keys: Optional[Sequence[str]] = None,
) -> Optional[dict]:
    return project_layout_node_plan_result(nodes=nodes, meta_keys=meta_keys)["node_plan"]


def project_layout_node_plan_result(
    *,
    nodes: Sequence[dict[str, Any]],
    meta_keys: Optional[Sequence[str]] = None,
) -> dict[str, Any]:
    payload = {"nodes": list(nodes or [])}
    if meta_keys is not None:
        payload["metaKeys"] = list(meta_keys or [])
    result = _run_layout_node_plan_command(
        project_layout_node_plan_args,
        payload,
        "Rust analysis compute project-layout-node-plan failed",
    )
    node_plan = result.get("nodePlan")
    if node_plan is None:
        return {"node_plan": None, "updates": []}
    if not isinstance(node_plan, dict):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-layout-node-plan returned invalid nodePlan"
        )
    updates = result.get("updates")
    if not isinstance(updates, list):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-layout-node-plan returned invalid updates"
        )
    return {"node_plan": node_plan, "updates": updates}


def project_layout_cache_key(
    *,
    nodes: Sequence[dict[str, Any]],
    edges: Sequence[dict[str, Any]],
    algo_version: str,
    mode: str,
    focus_id: str = "",
    graph_mutation_seq: int = 0,
    layout_direction: Optional[dict[str, Any]] = None,
    seed_context: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    payload = {
        "nodes": list(nodes or []),
        "edges": list(edges or []),
        "algoVersion": algo_version,
        "mode": mode,
        "focusId": focus_id,
        "graphMutationSeq": graph_mutation_seq,
        "layoutDirection": layout_direction or {},
        "seedContext": seed_context or {},
    }
    result = _run_layout_node_plan_command(
        project_layout_cache_key_args,
        payload,
        "Rust analysis compute project-layout-cache-key failed",
    )
    cache_key = result.get("cacheKey")
    if not isinstance(cache_key, dict) or not isinstance(cache_key.get("key"), str):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-layout-cache-key returned invalid cacheKey"
        )
    return cache_key


def apply_layout_node_plan(
    *,
    nodes: Sequence[dict[str, Any]],
    nodes_by_id: dict[str, Any],
    meta_keys: Optional[Sequence[str]] = None,
) -> dict:
    payload = {
        "nodes": list(nodes or []),
        "nodesById": nodes_by_id if isinstance(nodes_by_id, dict) else {},
    }
    if meta_keys is not None:
        payload["metaKeys"] = list(meta_keys or [])
    result = _run_layout_node_plan_command(
        apply_layout_node_plan_args,
        payload,
        "Rust analysis compute apply-layout-node-plan failed",
    )
    apply_result = result.get("result")
    if not (
        isinstance(apply_result, dict)
        and isinstance(apply_result.get("applied"), bool)
        and isinstance(apply_result.get("nodes"), list)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute apply-layout-node-plan returned invalid result"
        )
    return apply_result


def apply_layout_worker_node_plan(
    *,
    nodes: Sequence[dict[str, Any]],
    worker_nodes: Sequence[dict[str, Any]],
    meta_keys: Optional[Sequence[str]] = None,
) -> dict:
    payload = {
        "nodes": list(nodes or []),
        "workerNodes": list(worker_nodes or []),
    }
    if meta_keys is not None:
        payload["metaKeys"] = list(meta_keys or [])
    result = _run_layout_node_plan_command(
        apply_layout_worker_node_plan_args,
        payload,
        "Rust analysis compute apply-layout-worker-node-plan failed",
    )
    apply_result = result.get("result")
    if not (
        isinstance(apply_result, dict)
        and isinstance(apply_result.get("applied"), bool)
        and isinstance(apply_result.get("updates"), list)
        and isinstance(apply_result.get("nodes"), list)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute apply-layout-worker-node-plan returned invalid result"
        )
    return apply_result


def clear_layout_node_meta(
    *,
    nodes: Sequence[dict[str, Any]],
    meta_keys: Optional[Sequence[str]] = None,
) -> list:
    payload = {"nodes": list(nodes or [])}
    if meta_keys is not None:
        payload["metaKeys"] = list(meta_keys or [])
    result = _run_layout_node_plan_command(
        clear_layout_node_meta_args,
        payload,
        "Rust analysis compute clear-layout-node-meta failed",
    )
    cleared_nodes = result.get("nodes")
    if not isinstance(cleared_nodes, list):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute clear-layout-node-meta returned invalid nodes"
        )
    return cleared_nodes


def project_layout_role_graph(
    *,
    nodes: Sequence[dict[str, Any]],
    edges: Sequence[dict[str, Any]],
    traversal: Optional[dict[str, Any]] = None,
    demotion: Optional[dict[str, Any]] = None,
    promotion: Optional[dict[str, Any]] = None,
    role_resolution: Optional[dict[str, Any]] = None,
    cluster_resolution: Optional[dict[str, Any]] = None,
    semantic_pipeline: Optional[dict[str, Any]] = None,
) -> dict:
    payload: dict[str, Any] = {
        "nodes": list(nodes or []),
        "edges": list(edges or []),
    }
    if traversal is not None:
        payload["traversal"] = traversal if isinstance(traversal, dict) else {}
    if demotion is not None:
        payload["demotion"] = demotion if isinstance(demotion, dict) else {}
    if promotion is not None:
        payload["promotion"] = promotion if isinstance(promotion, dict) else {}
    if role_resolution is not None:
        payload["roleResolution"] = role_resolution if isinstance(role_resolution, dict) else {}
    if cluster_resolution is not None:
        payload["clusterResolution"] = cluster_resolution if isinstance(cluster_resolution, dict) else {}
    if semantic_pipeline is not None:
        payload["semanticPipeline"] = semantic_pipeline if isinstance(semantic_pipeline, dict) else {}
    result = _run_layout_node_plan_command(
        project_layout_role_graph_args,
        payload,
        "Rust analysis compute project-layout-role-graph failed",
    )
    projection = result.get("projection")
    if not (
        isinstance(projection, dict)
        and isinstance(projection.get("roleGraph"), dict)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-layout-role-graph returned invalid projection"
        )
    return projection


def project_analysis_graph_data(
    *,
    source: dict[str, Any],
    filter_min: Optional[float] = None,
    filter_max: Optional[float] = None,
    collapse_children: bool = False,
) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "source": source if isinstance(source, dict) else {},
        "filterMin": filter_min,
        "filterMax": filter_max,
        "collapseChildren": bool(collapse_children),
    }
    result = _run_layout_node_plan_command(
        project_analysis_graph_data_args,
        _json_safe_payload(payload),
        "Rust analysis compute project-analysis-graph-data failed",
    )
    graph_data = result.get("graphData")
    if not (
        isinstance(graph_data, dict)
        and isinstance(graph_data.get("nodes"), list)
        and isinstance(graph_data.get("edges"), list)
        and isinstance(graph_data.get("coreIds"), list)
        and isinstance(graph_data.get("childMap"), dict)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-analysis-graph-data returned invalid graphData"
        )
    return graph_data


def merge_flow_same_name_graph(
    *,
    nodes: Sequence[dict[str, Any]],
    edges: Sequence[dict[str, Any]],
    mode: str = "gross",
) -> dict[str, Any]:
    normalized_mode = str(mode or "gross").strip().lower()
    if normalized_mode not in {"gross", "net"}:
        raise ValueError("same-name merge mode must be gross or net")
    payload: dict[str, Any] = {
        "nodes": list(nodes or []),
        "edges": list(edges or []),
        "mode": normalized_mode,
    }
    result = _run_layout_node_plan_command(
        merge_flow_same_name_graph_args,
        _json_safe_payload(payload),
        "Rust analysis compute merge-flow-same-name-graph failed",
    )
    merge_result = result.get("mergeResult")
    if not (
        isinstance(merge_result, dict)
        and isinstance(merge_result.get("nodes"), list)
        and isinstance(merge_result.get("edges"), list)
        and isinstance(merge_result.get("hasMergeTarget"), bool)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute merge-flow-same-name-graph returned invalid mergeResult"
        )
    return merge_result


def project_flow_graph_data_merge(
    *,
    base_graph_data: Optional[dict[str, Any]] = None,
    patch_payload: Optional[dict[str, Any]] = None,
    use_base_graph: bool = True,
) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "baseGraphData": base_graph_data if isinstance(base_graph_data, dict) else {},
        "patchContracts": [
            {
                "name": "runtime-patch",
                "patchPayload": patch_payload if isinstance(patch_payload, dict) else {},
                "useBaseGraph": bool(use_base_graph),
            }
        ],
    }
    result = _run_layout_node_plan_command(
        project_flow_graph_data_merge_args,
        _json_safe_payload(payload),
        "Rust analysis compute project-flow-graph-data-merge failed",
    )
    data_merge = result.get("dataMerge")
    patch_results = data_merge.get("patchResults") if isinstance(data_merge, dict) else None
    patch_result = (
        patch_results[0].get("result")
        if isinstance(patch_results, list)
        and patch_results
        and isinstance(patch_results[0], dict)
        else None
    )
    if not (
        isinstance(patch_result, dict)
        and "data" in patch_result
        and isinstance(patch_result.get("patchKind"), str)
        and isinstance(patch_result.get("errorMessage"), str)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-graph-data-merge returned invalid patch result"
        )
    data = patch_result.get("data")
    if data is not None and not (
        isinstance(data, dict)
        and isinstance(data.get("nodes"), list)
        and isinstance(data.get("edges"), list)
        and isinstance(data.get("runtime_graph"), dict)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-graph-data-merge returned invalid graph data"
        )
    return patch_result


def project_flow_graph_search(
    *,
    nodes: Sequence[dict[str, Any]],
    query: str,
    limit: Optional[int] = 1,
) -> dict[str, Any]:
    normalized_limit = int(limit or 0)
    payload: dict[str, Any] = {
        "graph": {"nodes": list(nodes or [])},
        "query": str(query or ""),
    }
    if normalized_limit > 0:
        payload["limit"] = normalized_limit
    result = _run_layout_node_plan_command(
        project_flow_graph_search_args,
        _json_safe_payload(payload),
        "Rust analysis compute project-flow-graph-search failed",
    )
    search = result.get("search")
    if not (
        isinstance(search, dict)
        and isinstance(search.get("nodeIds"), list)
        and "firstNodeId" in search
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-graph-search returned invalid search"
        )
    return search


def project_flow_projection_layout_seed(
    *,
    nodes: Sequence[dict[str, Any]],
    base_nodes: Sequence[dict[str, Any]],
) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "nodes": list(nodes or []),
        "baseNodes": list(base_nodes or []),
    }
    result = _run_layout_node_plan_command(
        project_flow_projection_layout_seed_args,
        _json_safe_payload(payload),
        "Rust analysis compute project-flow-projection-layout-seed failed",
    )
    seed = result.get("seed")
    if not (
        isinstance(seed, dict)
        and isinstance(seed.get("summary"), dict)
        and isinstance(seed.get("updates"), list)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-projection-layout-seed returned invalid seed"
        )
    return seed


def project_flow_projection_layout_sync(
    *,
    layout_index: dict[str, Any],
    current_nodes: Optional[Sequence[dict[str, Any]]] = None,
) -> dict:
    require_analysis_compute_capability()
    payload = _json_safe_payload(layout_index if isinstance(layout_index, dict) else {})
    if current_nodes is not None:
        payload["current_nodes"] = _json_safe_payload(list(current_nodes or []))
    with tempfile.TemporaryDirectory(prefix="analytix-projection-layout-sync-") as temp_dir:
        input_path = Path(temp_dir) / "projection-layout-sync-input.json"
        input_path.write_text(
            json.dumps(payload, ensure_ascii=False, separators=(",", ":"), allow_nan=False),
            encoding="utf-8",
        )
        result = run_analysis_compute(
            project_flow_projection_layout_sync_args(input_path=input_path)
        )
    projection = result.get("projection") if isinstance(result, dict) else None
    if not (result and result.get("ok") is True and isinstance(projection, dict)):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-projection-layout-sync failed"
        )
    if not (
        isinstance(projection.get("layout_by_id"), dict)
        and isinstance(projection.get("node_updates"), list)
        and isinstance(projection.get("layout_index_summary"), dict)
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute project-flow-projection-layout-sync returned invalid projection"
        )
    return projection


def _json_safe_payload(value: Any) -> Any:
    if isinstance(value, dict):
        return {str(key): _json_safe_payload(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_json_safe_payload(item) for item in value]
    if isinstance(value, float) and not math.isfinite(value):
        return None
    return value


def _run_layout_node_plan_command(
    arg_builder,
    payload: dict[str, Any],
    failure_message: str,
) -> dict:
    require_analysis_compute_capability()
    with tempfile.TemporaryDirectory(prefix="analytix-layout-node-plan-") as temp_dir:
        input_path = Path(temp_dir) / "layout-node-plan-input.json"
        input_path.write_text(
            json.dumps(payload, ensure_ascii=False, separators=(",", ":")),
            encoding="utf-8",
        )
        result = run_analysis_compute(arg_builder(input_path=input_path))
    if not (result and result.get("ok") is True):
        raise AnalysisComputeUnavailableError(failure_message)
    return result


def _exact_nonnegative_materializer_count(value: Any) -> int | None:
    if type(value) is not int or value < 0 or value > 9_007_199_254_740_991:
        return None
    return value


def _validate_rule_txn_materializer_envelope(payload: Any, *, case_id: str) -> dict:
    if (
        type(payload) is not dict
        or set(payload) != {"ok", "case_id", "row_count", "rebuilt", "agg_name", "agg_version"}
        or payload.get("ok") is not True
        or payload.get("case_id") != case_id
        or _exact_nonnegative_materializer_count(payload.get("row_count")) is None
        or type(payload.get("rebuilt")) is not bool
        or payload.get("agg_name") != "rule_txn_index:v2"
        or _exact_nonnegative_materializer_count(payload.get("agg_version")) != 2
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-txn-index returned an invalid envelope"
        )
    return payload


def _validate_rule_pattern_materializer_envelope(payload: Any, *, case_id: str) -> dict:
    if (
        type(payload) is not dict
        or set(payload)
        != {
            "ok",
            "case_id",
            "row_count",
            "rebuilt",
            "input_coverage",
            "feature_readiness",
            "agg_name",
            "agg_version",
        }
        or payload.get("ok") is not True
        or payload.get("case_id") != case_id
        or _exact_nonnegative_materializer_count(payload.get("row_count")) is None
        or type(payload.get("rebuilt")) is not bool
        or payload.get("agg_name") != "rule_pattern_index:v9"
        or _exact_nonnegative_materializer_count(payload.get("agg_version")) != 9
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned an invalid envelope"
        )

    coverage = payload.get("input_coverage")
    coverage_fields = {
        "total_rows",
        "accepted_rows",
        "rejected_rows",
        "account_key_covered_rows",
        "transaction_id_covered_rows",
        "transaction_time_covered_rows",
        "amount_covered_rows",
        "direction_covered_rows",
        "cash_covered_rows",
        "cash_unknown_rows",
        "cash_conflict_rows",
    }
    if type(coverage) is not dict or set(coverage) != coverage_fields:
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned invalid input coverage"
        )
    counts = {
        field: _exact_nonnegative_materializer_count(coverage.get(field))
        for field in coverage_fields
    }
    if any(value is None for value in counts.values()):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned invalid input coverage"
        )
    total_rows = counts["total_rows"]
    if (
        total_rows is None
        or counts["accepted_rows"] != total_rows
        or counts["rejected_rows"] != 0
        or any(
            counts[field] != total_rows
            for field in (
                "account_key_covered_rows",
                "transaction_id_covered_rows",
                "transaction_time_covered_rows",
                "amount_covered_rows",
                "direction_covered_rows",
            )
        )
        or counts["cash_covered_rows"] + counts["cash_unknown_rows"] + counts["cash_conflict_rows"]
        != total_rows
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned inconsistent input coverage"
        )

    readiness = payload.get("feature_readiness")
    if type(readiness) is not dict or set(readiness) != {
        "cash_dependent_rules",
        "cash_independent_rules",
    }:
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned invalid feature readiness"
        )
    cash_dependent = readiness.get("cash_dependent_rules")
    cash_independent = readiness.get("cash_independent_rules")
    if type(cash_dependent) is not dict or set(cash_dependent) != {
        "status",
        "requested_rows",
        "eligible_rows",
        "unknown_cash_rows",
        "conflict_cash_rows",
        "blocker",
    }:
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned invalid cash readiness"
        )
    if type(cash_independent) is not dict or set(cash_independent) != {
        "status",
        "requested_rows",
        "eligible_rows",
        "blocker",
    }:
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned invalid independent readiness"
        )
    cash_counts = {
        field: _exact_nonnegative_materializer_count(cash_dependent.get(field))
        for field in (
            "requested_rows",
            "eligible_rows",
            "unknown_cash_rows",
            "conflict_cash_rows",
        )
    }
    independent_requested = _exact_nonnegative_materializer_count(
        cash_independent.get("requested_rows")
    )
    independent_eligible = _exact_nonnegative_materializer_count(
        cash_independent.get("eligible_rows")
    )
    cash_complete = (
        counts["cash_covered_rows"] == total_rows
        and counts["cash_unknown_rows"] == 0
        and counts["cash_conflict_rows"] == 0
    )
    expected_cash_status = "complete" if cash_complete else "partial"
    expected_cash_blocker = None if cash_complete else "cash_classification_coverage_incomplete"
    if (
        any(value is None for value in cash_counts.values())
        or cash_counts["requested_rows"] != total_rows
        or cash_counts["eligible_rows"] != counts["cash_covered_rows"]
        or cash_counts["unknown_cash_rows"] != counts["cash_unknown_rows"]
        or cash_counts["conflict_cash_rows"] != counts["cash_conflict_rows"]
        or cash_dependent.get("status") != expected_cash_status
        or cash_dependent.get("blocker") != expected_cash_blocker
        or cash_independent.get("status") != "complete"
        or independent_requested != total_rows
        or independent_eligible != counts["accepted_rows"]
        or cash_independent.get("blocker") is not None
    ):
        raise AnalysisComputeUnavailableError(
            "Rust analysis compute materialize-rule-pattern-index returned inconsistent feature readiness"
        )
    return payload


def materialize_rule_txn_index(
    *,
    case_id: str,
    db_path: Path,
    force: bool = False,
) -> dict:
    payload = run_analysis_compute(
        materialize_rule_txn_index_args(case_id=case_id, db_path=db_path, force=force)
    )
    return _validate_rule_txn_materializer_envelope(payload, case_id=case_id)


def try_materialize_rule_txn_index(
    *,
    case_id: str,
    db_path: Path,
    force: bool = False,
) -> Optional[dict]:
    try:
        return materialize_rule_txn_index(case_id=case_id, db_path=db_path, force=force)
    except AnalysisComputeUnavailableError:
        return None


def materialize_rule_pattern_index(
    *,
    case_id: str,
    db_path: Path,
    param_signature: str,
    round_unit: float,
    round_min_amount: float,
    round_min_count: int,
    round_min_total_amount: float,
    small_fast_window_minutes: int,
    small_fast_ratio: float,
    small_fast_min_amount: float,
    small_fast_max_amount: float,
    cash_quick_window_minutes: int,
    cash_quick_min_amount: float,
    cash_candidate_window_minutes: int,
    cash_candidate_min_amount: float,
    cash_candidate_min_ratio: float,
    cash_candidate_max_ratio: float,
    near_threshold_amount: float,
    near_threshold_lower_rate: float,
    near_threshold_window_minutes: int,
    near_threshold_min_count: int,
    near_threshold_min_total_amount: float,
    repeated_amount_window_minutes: int,
    repeated_amount_min_amount: float,
    repeated_amount_min_count: int,
    repeated_amount_min_total_amount: float,
    threshold_split_window_minutes: int,
    threshold_split_amount: float,
    threshold_split_tolerance_rate: float,
    threshold_split_min_count: int,
    high_freq_small_amount_threshold: float,
    high_freq_window_minutes: int,
    high_freq_count_threshold: int,
    high_freq_min_total_amount: float,
    night_start_hour: int,
    night_end_hour: int,
    night_min_count: int,
    night_min_total_amount: float,
    force: bool = False,
) -> dict:
    payload = run_analysis_compute(
        materialize_rule_pattern_index_args(
            case_id=case_id,
            db_path=db_path,
            param_signature=param_signature,
            round_unit=round_unit,
            round_min_amount=round_min_amount,
            round_min_count=round_min_count,
            round_min_total_amount=round_min_total_amount,
            small_fast_window_minutes=small_fast_window_minutes,
            small_fast_ratio=small_fast_ratio,
            small_fast_min_amount=small_fast_min_amount,
            small_fast_max_amount=small_fast_max_amount,
            cash_quick_window_minutes=cash_quick_window_minutes,
            cash_quick_min_amount=cash_quick_min_amount,
            cash_candidate_window_minutes=cash_candidate_window_minutes,
            cash_candidate_min_amount=cash_candidate_min_amount,
            cash_candidate_min_ratio=cash_candidate_min_ratio,
            cash_candidate_max_ratio=cash_candidate_max_ratio,
            near_threshold_amount=near_threshold_amount,
            near_threshold_lower_rate=near_threshold_lower_rate,
            near_threshold_window_minutes=near_threshold_window_minutes,
            near_threshold_min_count=near_threshold_min_count,
            near_threshold_min_total_amount=near_threshold_min_total_amount,
            repeated_amount_window_minutes=repeated_amount_window_minutes,
            repeated_amount_min_amount=repeated_amount_min_amount,
            repeated_amount_min_count=repeated_amount_min_count,
            repeated_amount_min_total_amount=repeated_amount_min_total_amount,
            threshold_split_window_minutes=threshold_split_window_minutes,
            threshold_split_amount=threshold_split_amount,
            threshold_split_tolerance_rate=threshold_split_tolerance_rate,
            threshold_split_min_count=threshold_split_min_count,
            high_freq_small_amount_threshold=high_freq_small_amount_threshold,
            high_freq_window_minutes=high_freq_window_minutes,
            high_freq_count_threshold=high_freq_count_threshold,
            high_freq_min_total_amount=high_freq_min_total_amount,
            night_start_hour=night_start_hour,
            night_end_hour=night_end_hour,
            night_min_count=night_min_count,
            night_min_total_amount=night_min_total_amount,
            force=force,
        )
    )
    return _validate_rule_pattern_materializer_envelope(payload, case_id=case_id)


def try_materialize_rule_pattern_index(
    *,
    case_id: str,
    db_path: Path,
    param_signature: str,
    round_unit: float,
    round_min_amount: float,
    round_min_count: int,
    round_min_total_amount: float,
    small_fast_window_minutes: int,
    small_fast_ratio: float,
    small_fast_min_amount: float,
    small_fast_max_amount: float,
    cash_quick_window_minutes: int,
    cash_quick_min_amount: float,
    cash_candidate_window_minutes: int,
    cash_candidate_min_amount: float,
    cash_candidate_min_ratio: float,
    cash_candidate_max_ratio: float,
    near_threshold_amount: float,
    near_threshold_lower_rate: float,
    near_threshold_window_minutes: int,
    near_threshold_min_count: int,
    near_threshold_min_total_amount: float,
    repeated_amount_window_minutes: int,
    repeated_amount_min_amount: float,
    repeated_amount_min_count: int,
    repeated_amount_min_total_amount: float,
    threshold_split_window_minutes: int,
    threshold_split_amount: float,
    threshold_split_tolerance_rate: float,
    threshold_split_min_count: int,
    high_freq_small_amount_threshold: float,
    high_freq_window_minutes: int,
    high_freq_count_threshold: int,
    high_freq_min_total_amount: float,
    night_start_hour: int,
    night_end_hour: int,
    night_min_count: int,
    night_min_total_amount: float,
    force: bool = False,
) -> Optional[dict]:
    try:
        return materialize_rule_pattern_index(
            case_id=case_id,
            db_path=db_path,
            param_signature=param_signature,
            round_unit=round_unit,
            round_min_amount=round_min_amount,
            round_min_count=round_min_count,
            round_min_total_amount=round_min_total_amount,
            small_fast_window_minutes=small_fast_window_minutes,
            small_fast_ratio=small_fast_ratio,
            small_fast_min_amount=small_fast_min_amount,
            small_fast_max_amount=small_fast_max_amount,
            cash_quick_window_minutes=cash_quick_window_minutes,
            cash_quick_min_amount=cash_quick_min_amount,
            cash_candidate_window_minutes=cash_candidate_window_minutes,
            cash_candidate_min_amount=cash_candidate_min_amount,
            cash_candidate_min_ratio=cash_candidate_min_ratio,
            cash_candidate_max_ratio=cash_candidate_max_ratio,
            near_threshold_amount=near_threshold_amount,
            near_threshold_lower_rate=near_threshold_lower_rate,
            near_threshold_window_minutes=near_threshold_window_minutes,
            near_threshold_min_count=near_threshold_min_count,
            near_threshold_min_total_amount=near_threshold_min_total_amount,
            repeated_amount_window_minutes=repeated_amount_window_minutes,
            repeated_amount_min_amount=repeated_amount_min_amount,
            repeated_amount_min_count=repeated_amount_min_count,
            repeated_amount_min_total_amount=repeated_amount_min_total_amount,
            threshold_split_window_minutes=threshold_split_window_minutes,
            threshold_split_amount=threshold_split_amount,
            threshold_split_tolerance_rate=threshold_split_tolerance_rate,
            threshold_split_min_count=threshold_split_min_count,
            high_freq_small_amount_threshold=high_freq_small_amount_threshold,
            high_freq_window_minutes=high_freq_window_minutes,
            high_freq_count_threshold=high_freq_count_threshold,
            high_freq_min_total_amount=high_freq_min_total_amount,
            night_start_hour=night_start_hour,
            night_end_hour=night_end_hour,
            night_min_count=night_min_count,
            night_min_total_amount=night_min_total_amount,
            force=force,
        )
    except AnalysisComputeUnavailableError:
        return None
