from __future__ import annotations

import hashlib
import json
import math
import os
import stat
import tempfile
import uuid
from collections import defaultdict
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from typing import Any, Dict, Iterable, Iterator, List, Optional, Sequence, Tuple

from app.core.analysis_compute import (
    project_flow_graph_render_plan,
    project_flow_projection_layout_sync,
    project_flow_skeleton_clusters,
    query_flow_focus_graph_to_file,
)
from app.core.db_engine import DuckDBEngine
from app.core.fc_import_norm_insert import ts_norm_expr as _ts_norm_expr
from app.core.storage import CaseStorage
from app.repositories.txn_daily_aggregate import (
    TxnAmountCoverageIncompleteError,
    TxnAmountCoverageV1,
    TxnDailyAggregateStore,
)
from app.utils.counterparty_placeholders import (
    PLACEHOLDER_KIND_LABELS,
    PLACEHOLDER_TOKEN_PREFIX,
    classify_counterparty_placeholder,
    normalize_placeholder_kinds,
    placeholder_key_token,
    placeholder_kind_from_token,
    placeholder_kind_label,
    placeholder_kind_sql,
)
from app.utils.fs import (
    atomic_create_private_text,
    atomic_write_private_text,
    case_bound_storage_name,
    legacy_case_bound_storage_name_v1,
    list_private_directories,
    list_private_directory_entries,
    private_exclusive_file_lock,
    read_private_text,
    remove_path_no_follow_under,
    remove_private_regular_file_under_if_matches,
    safe_fs_name,
)
from app.utils.strict_json import StrictJSONError, dumps_canonical_json, loads_strict_json
from app.utils.time import utc_now

_ACCOUNT_KEY_CANDIDATES = (
    "acct_key",
    "clean_acct_no",
    "acct_no_norm",
    "acct_no",
    "clean_card_no",
    "card_no_norm",
    "card_no",
)

_STATS_TXN_MIRROR_TOLERANCE_SECONDS = 90
_GRAPH_TIER_SMALL_MAX_NODES = 1_000
_GRAPH_TIER_MEDIUM_MAX_NODES = 10_000
_GRAPH_TIER_LARGE_MAX_NODES = 100_000
_NETWORK_LAYOUT_LARGE_MIN_NODES = 1_200
_NETWORK_LAYOUT_LARGE_MIN_EDGES = 3_500
_CLUSTER_NODE_PREFIX = "__cluster__::"
_PROJECTION_TILE_SIZE_LARGE = 192
_PROJECTION_TILE_SIZE_XLARGE = 128
_PROJECTION_VIEWPORT_PADDING_RATIO = 0.18
_PROJECTION_VIEWPORT_MAX_TARGET_CLUSTERS = 48
_PROJECTION_VIEWPORT_MAX_TARGET_NODES = 480
_RESULT_SNAPSHOT_DELTA_MAX_CHAIN_DEPTH = 6
_RESULT_SNAPSHOT_GC_SOFT_LIMIT = 384
_RESULT_SNAPSHOT_GC_KEEP_LATEST = 192
_RESULT_SNAPSHOT_GC_TTL_SECONDS = 72 * 3600
_RESULT_SNAPSHOT_DEPENDENCY_MAX_NODES = 4_096
_RESULT_SNAPSHOT_DEPENDENCY_MAX_DEPTH = 512
_FLOW_RESULT_SNAPSHOT_VERSION = 3
_FLOW_JOB_RESULT_MANIFEST_VERSION = 2
_FLOW_JOB_RESULT_MANIFEST_MAX_BYTES = 64 * 1024
_FLOW_RESULT_SNAPSHOT_MAX_BYTES = 256 * 1024 * 1024
_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES = 5_000_000
_FLOW_JOB_RESULT_MANIFEST_DOMAIN = b"AnalytixFlowJobResultManifestV2\x00"
_FLOW_RESULT_SNAPSHOT_CONTENT_DOMAIN = b"AnalytixFlowResultSnapshotContentV3\x00"
_FLOW_RESULT_SNAPSHOT_ENVELOPE_DOMAIN = b"AnalytixFlowResultSnapshotEnvelopeV3\x00"
_FLOW_CASE_LIFECYCLE_BINDING_DOMAIN = b"AnalytixFlowCaseLifecycleBindingV1\x00"
_FLOW_VIEW_METADATA_VERSION = 2
_FLOW_VIEW_METADATA_MAX_BYTES = 4 * 1024 * 1024
_FLOW_VIEW_METADATA_MAX_JSON_NODES = 250_000


def _flow_case_lifecycle_binding_digest(case_id: str, generation: int) -> str:
    return hashlib.sha256(
        _FLOW_CASE_LIFECYCLE_BINDING_DOMAIN
        + str(case_id or "").strip().encode("utf-8", errors="strict")
        + b"\x00"
        + str(generation).encode("ascii", errors="strict")
    ).hexdigest()


@dataclass(frozen=True)
class FlowCaseLifecycleBindingV1:
    case_id: str
    generation: int
    binding_digest: str

    def to_dict(self) -> dict[str, Any]:
        return {
            "version": 1,
            "case_id": self.case_id,
            "lifecycle_generation": self.generation,
            "binding_digest": self.binding_digest,
        }


def _attach_candidate_amount_coverage(result: dict[str, Any], coverage: TxnAmountCoverageV1) -> dict[str, Any]:
    stats = dict(result.get("stats") or {})
    stats.update(
        {
            "amount_coverage_contract": coverage.contract,
            "amount_coverage_status": "complete",
            "amount_coverage_digest": coverage.digest,
            "amount_coverage_authority": "candidate_only_not_host_evidence_receipt",
            "amount_coverage_case_id": coverage.case_id,
            "amount_coverage_materialization_identity": coverage.materialization_identity,
            "amount_total_rows": coverage.total_rows,
            "amount_valid_rows": coverage.amount_valid_rows,
            "amount_missing_rows": coverage.amount_missing_rows,
            "amount_parse_failed_rows": coverage.amount_parse_failed_rows,
            "direction_covered_rows": coverage.direction_covered_rows,
            "fact_answer_allowed": False,
            "zero_result_status": "unresolved",
        }
    )
    result["stats"] = stats
    result["publication_status"] = "blocked"
    result["fact_answer_allowed"] = False
    return result


def _has_complete_candidate_amount_coverage(result: object, *, expected_case_id: str = "") -> bool:
    source = result if isinstance(result, dict) else {}
    stats = source.get("stats") if isinstance(source.get("stats"), dict) else {}
    digest = str(stats.get("amount_coverage_digest") or "").strip()
    try:
        total_rows = int(stats.get("amount_total_rows"))
        valid_rows = int(stats.get("amount_valid_rows"))
        missing_rows = int(stats.get("amount_missing_rows"))
        failed_rows = int(stats.get("amount_parse_failed_rows"))
        direction_rows = int(stats.get("direction_covered_rows"))
    except (TypeError, ValueError):
        return False
    case_id = str(stats.get("amount_coverage_case_id") or "").strip()
    materialization_identity = str(stats.get("amount_coverage_materialization_identity") or "").strip()
    candidate = TxnAmountCoverageV1(
        case_id=case_id,
        materialization_identity=materialization_identity,
        total_rows=total_rows,
        amount_valid_rows=valid_rows,
        amount_missing_rows=missing_rows,
        amount_parse_failed_rows=failed_rows,
        direction_covered_rows=direction_rows,
    )
    return (
        stats.get("amount_coverage_contract") == "TxnAmountCoverageV1"
        and stats.get("amount_coverage_status") == "complete"
        and stats.get("amount_coverage_authority") == "candidate_only_not_host_evidence_receipt"
        and stats.get("fact_answer_allowed") is False
        and source.get("fact_answer_allowed") is False
        and source.get("publication_status") == "blocked"
        and (not expected_case_id or case_id == str(expected_case_id).strip())
        and materialization_identity.startswith("txn_daily_snapshot:v12:")
        and len(materialization_identity) == len("txn_daily_snapshot:v12:") + 64
        and all(character in "0123456789abcdef" for character in materialization_identity.removeprefix("txn_daily_snapshot:v12:"))
        and candidate.complete
        and digest == candidate.digest
    )


def _trim_expr(expr: str) -> str:
    return f"NULLIF(TRIM({expr}), '')"


def _clean_text_expr(expr: str) -> str:
    cleaned = _trim_expr(expr)
    cleaned = f"NULLIF({cleaned}, '-')"
    cleaned = f"NULLIF({cleaned}, '—')"
    cleaned = f"NULLIF({cleaned}, '－')"
    return cleaned


def _coalesce_expr(exprs: Sequence[Optional[str]], default: str = "NULL") -> str:
    parts = [expr for expr in exprs if expr]
    if not parts:
        return default
    if len(parts) == 1:
        return parts[0]
    return "COALESCE(" + ",".join(parts) + ")"


def _dedupe_text_part(alias: str, column: str) -> str:
    return f"COALESCE(NULLIF(TRIM(CAST({alias}.{column} AS VARCHAR)), ''), '<null>')"


def _dedupe_coalesced_text_part(alias: str, columns: Sequence[str]) -> str:
    expressions = [f"NULLIF(TRIM(CAST({alias}.{column} AS VARCHAR)), '')" for column in columns]
    return f"COALESCE({', '.join(expressions)}, '<null>')"


def _stats_replacement_dedupe_key_expr(alias: str) -> str:
    counterparty_account = _dedupe_coalesced_text_part(alias, ("counterparty_acct", "cp_raw", "cp_key"))
    parts = [
        _dedupe_text_part(alias, "account_open_name"),
        _dedupe_text_part(alias, "opener_id_no"),
        _dedupe_text_part(alias, "dc_val"),
        f"COALESCE(CAST({alias}.txn_ts AS VARCHAR), '<null>')",
        f"COALESCE(CAST(ROUND(ABS({alias}.amount), 2) AS VARCHAR), '<null>')",
        f"COALESCE(CAST(ROUND({alias}.balance, 2) AS VARCHAR), '<null>')",
        f"COALESCE(CAST(ROUND({alias}.counterparty_balance, 2) AS VARCHAR), '<null>')",
        counterparty_account,
        _dedupe_text_part(alias, "counterparty_name"),
        _dedupe_text_part(alias, "counterparty_id_no"),
        _dedupe_text_part(alias, "counterparty_bank"),
        _dedupe_text_part(alias, "is_success"),
        _dedupe_text_part(alias, "summary"),
        _dedupe_text_part(alias, "remark"),
        _dedupe_text_part(alias, "txn_type"),
        _dedupe_text_part(alias, "currency"),
        _dedupe_text_part(alias, "branch_name"),
        _dedupe_text_part(alias, "branch_code"),
        _dedupe_text_part(alias, "location"),
        _dedupe_text_part(alias, "cash_flag"),
        _dedupe_text_part(alias, "voucher_no"),
        _dedupe_text_part(alias, "terminal_no"),
        _dedupe_text_part(alias, "ip_addr"),
        _dedupe_text_part(alias, "mac_addr"),
        _dedupe_text_part(alias, "txn_id"),
        _dedupe_text_part(alias, "log_id"),
        _dedupe_text_part(alias, "voucher_type"),
        _dedupe_text_part(alias, "voucher_id"),
        _dedupe_text_part(alias, "teller_no"),
        _dedupe_text_part(alias, "merchant_name"),
        _dedupe_text_part(alias, "merchant_no"),
        _dedupe_text_part(alias, "query_feedback_reason"),
    ]
    key_expr = " || '|#|' || ".join(parts)
    return (
        "CASE "
        f"WHEN NULLIF(TRIM(COALESCE({alias}.account_open_name, '')), '') IS NOT NULL "
        f" AND NULLIF(TRIM(COALESCE({alias}.opener_id_no, '')), '') IS NOT NULL "
        f" AND NULLIF(TRIM(COALESCE({alias}.dc_val, '')), '') IS NOT NULL "
        f" AND {alias}.txn_ts IS NOT NULL "
        f" AND {alias}.balance IS NOT NULL "
        f" AND ABS({alias}.amount) > 0 "
        f"THEN {key_expr} ELSE NULL END"
    )


def _stats_name_replacement_dedupe_base_sql() -> str:
    stats_dedupe_key = _stats_replacement_dedupe_key_expr("d")
    return (
        "WITH source AS ("
        "  SELECT d.*, "
        f"         {stats_dedupe_key} AS stats_dedupe_key "
        "  FROM analysis_txn_detail_idx d"
        "), account_latest AS ("
        "  SELECT acct_key, MAX(txn_ts) AS acct_last_ts, MAX(id) AS acct_last_id "
        "  FROM analysis_txn_detail_idx "
        "  GROUP BY acct_key"
        "), family_source AS ("
        "  SELECT source.*, "
        "         COALESCE(NULLIF(TRIM(CAST(account_dim.bank_name AS VARCHAR)), ''), '<null>') AS stats_dedupe_bank, "
        "         COALESCE(NULLIF(TRIM(CAST(account_dim.acct_type AS VARCHAR)), ''), '<null>') AS stats_dedupe_acct_type, "
        "         COALESCE(NULLIF(TRIM(CAST(account_dim.card_display AS VARCHAR)), ''), '') AS stats_dedupe_card_display, "
        "         COALESCE(NULLIF(TRIM(CAST(account_dim.acct_display AS VARCHAR)), ''), '') AS stats_dedupe_acct_display, "
        "         account_latest.acct_last_ts AS stats_dedupe_acct_last_ts, "
        "         account_latest.acct_last_id AS stats_dedupe_acct_last_id "
        "  FROM source "
        "  LEFT JOIN analysis_account_dim account_dim ON account_dim.account_key = source.acct_key "
        "  LEFT JOIN account_latest ON account_latest.acct_key = source.acct_key"
        "), signature_stats AS ("
        "  SELECT stats_dedupe_key, COUNT(DISTINCT acct_key) AS acct_count "
        "  FROM family_source "
        "  WHERE stats_dedupe_key IS NOT NULL "
        "    AND stats_dedupe_bank <> '<null>' "
        "    AND stats_dedupe_acct_type <> '<null>' "
        "  GROUP BY stats_dedupe_key "
        "  HAVING COUNT(DISTINCT acct_key) > 1 "
        "     AND COUNT(DISTINCT stats_dedupe_bank) = 1 "
        "     AND COUNT(DISTINCT stats_dedupe_acct_type) = 1"
        "), ranked AS ("
        "  SELECT family_source.*, "
        "         CASE WHEN sig.stats_dedupe_key IS NULL THEN NULL ELSE "
        "           ROW_NUMBER() OVER ("
        "             PARTITION BY family_source.stats_dedupe_key "
        "             ORDER BY family_source.stats_dedupe_card_display DESC, "
        "                      family_source.stats_dedupe_acct_display DESC, "
        "                      family_source.acct_key DESC, "
        "                      family_source.stats_dedupe_acct_last_ts DESC NULLS LAST, "
        "                      family_source.stats_dedupe_acct_last_id DESC NULLS LAST, "
        "                      family_source.id ASC"
        "           ) "
        "         END AS stats_dedupe_rank "
        "  FROM family_source "
        "  LEFT JOIN signature_stats sig ON sig.stats_dedupe_key = family_source.stats_dedupe_key"
        "), filtered AS ("
        "  SELECT * FROM ranked WHERE stats_dedupe_rank IS NULL OR stats_dedupe_rank = 1"
        "), base AS ("
        "  SELECT "
        "    acct_key AS acct_key, "
        "    cp_key AS cp_key, "
        "    cp_raw AS cp_key_raw, "
        "    cp_placeholder_kind AS cp_placeholder_kind, "
        "    dc_val AS dc_val, "
        "    ABS(amount) AS amt_abs, "
        "    txn_ts AS txn_ts, "
        "    account_open_name AS open_name, "
        "    COALESCE(NULLIF(counterparty_name, ''), NULLIF(stats_name_key, '')) AS cp_name "
        "  FROM filtered"
        ") "
    )


def _trim_keep_empty_expr(expr: str) -> str:
    return f"CASE WHEN {expr} IS NULL THEN NULL ELSE TRIM({expr}) END"


def _text_col_expr(prefix: str, columns: set[str], col: str, *, clean_invalid: bool = False) -> Optional[str]:
    if col not in columns:
        return None
    if clean_invalid:
        return _clean_text_expr(f"{prefix}.{col}")
    return _trim_expr(f"{prefix}.{col}")


def _num_col_expr(prefix: str, columns: set[str], col: str) -> Optional[str]:
    if col not in columns:
        return None
    if col.endswith("_val"):
        return f"{prefix}.{col}"
    return f"TRY_CAST({_trim_expr(f'{prefix}.{col}')} AS DOUBLE)"


def _account_key_expr(prefix: str, columns: set[str], candidates: Sequence[str] = _ACCOUNT_KEY_CANDIDATES) -> str:
    return _coalesce_expr([_text_col_expr(prefix, columns, c, clean_invalid=True) for c in candidates])


def _counterparty_key_expr(prefix: str, columns: set[str]) -> str:
    return _coalesce_expr(
        [_text_col_expr(prefix, columns, c) for c in ("cp_key", "counterparty_acct_norm", "counterparty_acct")]
    )


def _counterparty_raw_expr(prefix: str, columns: set[str]) -> str:
    return _coalesce_expr(
        [
            _trim_keep_empty_expr(f"{prefix}.{c}")
            for c in ("cp_raw", "counterparty_acct", "counterparty_acct_norm")
            if c in columns
        ]
    )


def _dc_value_expr(prefix: str, columns: set[str]) -> str:
    return _coalesce_expr([_text_col_expr(prefix, columns, c) for c in ("dc_val", "dc_final", "clean_dc_flag", "dc_flag")])


def _amount_value_expr(prefix: str, columns: set[str]) -> str:
    return _coalesce_expr([_num_col_expr(prefix, columns, c) for c in ("clean_amount", "amount_val", "amount")], "NULL")


def _txn_ts_expr(prefix: str, columns: set[str]) -> str:
    parts: list[str] = []
    if "txn_ts" in columns:
        parts.append(f"{prefix}.txn_ts")
    if "txn_time" in columns:
        parts.append(_ts_norm_expr(f"{prefix}.txn_time"))
    return _coalesce_expr(parts, "NULL")


def _round2(value: Any) -> float:
    return round(_required_amount_float(value), 2)


def _resolve_graph_tier(node_count: Any) -> str:
    total = max(0, int(node_count or 0))
    if total <= _GRAPH_TIER_SMALL_MAX_NODES:
        return "small"
    if total <= _GRAPH_TIER_MEDIUM_MAX_NODES:
        return "medium"
    if total <= _GRAPH_TIER_LARGE_MAX_NODES:
        return "large"
    return "xlarge"


def _build_graph_render_hints(
    *,
    node_count: Any,
    edge_count: Any,
    view_mode: Any = "",
) -> dict[str, Any]:
    total_nodes = max(0, int(node_count or 0))
    total_edges = max(0, int(edge_count or 0))
    tier = _resolve_graph_tier(total_nodes)
    normalized_view_mode = str(view_mode or "").strip().lower() or "relation"

    if tier == "small":
        return {
            "tier": tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": True,
            "default_node_mode": "entity",
            "entity_node_limit": total_nodes,
            "focus_entity_limit": total_nodes,
            "t0_label_limit": total_nodes,
            "t1_reveal_batch_size": max(48, min(total_nodes or 48, 120)),
            "t1_reveal_delay_ms": 90,
            "t1_batch_gap_ms": 12,
            "show_edge_labels": total_edges <= 800,
            "show_detail_edge_labels": total_edges <= 480,
            "edge_mode": "full",
            "edge_batch_threshold": max(1_500, total_edges + 1),
            "edge_batch_size": 80,
            "edge_refresh_batch_size": 180,
            "animation_mode": "full",
            "layout_switch_animate_max_nodes": total_nodes,
            "layout_switch_animate_max_edges": total_edges,
            "prefer_fast_first_paint": True,
            "projection_auto_expand": False,
        }
    if tier == "medium":
        return {
            "tier": tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": True,
            "default_node_mode": "entity",
            "entity_node_limit": total_nodes,
            "focus_entity_limit": min(total_nodes, 420),
            "t0_label_limit": min(total_nodes, 420 if total_nodes <= 6_000 else 560),
            "t1_reveal_batch_size": 180 if total_nodes <= 3_000 else 240,
            "t1_reveal_delay_ms": 140 if total_nodes <= 3_000 else 180,
            "t1_batch_gap_ms": 18 if total_nodes <= 3_000 else 22,
            "show_edge_labels": total_edges <= 2_400,
            "show_detail_edge_labels": False,
            "edge_mode": "batched" if total_edges >= 1_500 else "full",
            "edge_batch_threshold": 1_500,
            "edge_batch_size": 120 if total_edges < 6_000 else 180,
            "edge_refresh_batch_size": 240,
            "animation_mode": "light",
            "layout_switch_animate_max_nodes": 1_200,
            "layout_switch_animate_max_edges": 3_600,
            "prefer_fast_first_paint": True,
            "projection_auto_expand": False,
        }
    if tier == "large":
        return {
            "tier": tier,
            "view_mode": normalized_view_mode,
            "all_nodes_visible": True,
            "default_node_mode": "mixed",
            "entity_node_limit": min(max(1_800, total_nodes // 6), 4_200),
            "focus_entity_limit": min(total_nodes, 720),
            "t0_label_limit": min(total_nodes, 240),
            "t1_reveal_batch_size": 220,
            "t1_reveal_delay_ms": 180,
            "t1_batch_gap_ms": 24,
            "show_edge_labels": False,
            "show_detail_edge_labels": False,
            "edge_mode": "batched",
            "edge_batch_threshold": 900,
            "edge_batch_size": 180,
            "edge_refresh_batch_size": 320,
            "animation_mode": "minimal",
            "layout_switch_animate_max_nodes": 0,
            "layout_switch_animate_max_edges": 0,
            "prefer_fast_first_paint": True,
            "projection_auto_expand": False,
        }
    return {
        "tier": tier,
        "view_mode": normalized_view_mode,
        "all_nodes_visible": True,
        "default_node_mode": "mixed",
        "entity_node_limit": min(max(1_200, total_nodes // 10), 2_200),
        "focus_entity_limit": min(total_nodes, 480),
        "t0_label_limit": min(total_nodes, 120),
        "t1_reveal_batch_size": 160,
        "t1_reveal_delay_ms": 220,
        "t1_batch_gap_ms": 28,
        "show_edge_labels": False,
        "show_detail_edge_labels": False,
        "edge_mode": "batched",
        "edge_batch_threshold": 600,
        "edge_batch_size": 220,
        "edge_refresh_batch_size": 360,
        "animation_mode": "minimal",
        "layout_switch_animate_max_nodes": 0,
        "layout_switch_animate_max_edges": 0,
        "prefer_fast_first_paint": True,
        "projection_auto_expand": True,
        "projection_auto_expand_delay_ms": 280,
        "projection_auto_expand_cooldown_ms": 1_200,
        "projection_auto_expand_min_zoom": 0.18,
        "projection_auto_expand_max_clusters": 24,
        "projection_auto_expand_max_nodes": 240,
        "projection_viewport_materialize_limit": 7_500,
    }


def _attach_graph_render_metadata(result: dict[str, Any]) -> dict[str, Any]:
    payload = result if isinstance(result, dict) else {}
    runtime_graph = payload.get("runtime_graph") if isinstance(payload.get("runtime_graph"), dict) else {}
    runtime_nodes = runtime_graph.get("nodes") if isinstance(runtime_graph.get("nodes"), list) else []
    runtime_edges = runtime_graph.get("edges") if isinstance(runtime_graph.get("edges"), list) else []
    node_count = len(runtime_nodes) or len(payload.get("nodes") or [])
    edge_count = len(runtime_edges) or len(payload.get("edges") or [])
    stats = dict(payload.get("stats") or {})
    tier = str(payload.get("graph_tier") or stats.get("graph_tier") or _resolve_graph_tier(node_count)).strip() or "small"
    render_hints = payload.get("render_hints")
    if not isinstance(render_hints, dict):
        render_hints = _build_graph_render_hints(
            node_count=node_count,
            edge_count=edge_count,
            view_mode=stats.get("view_mode") or payload.get("view_mode") or "",
        )
    else:
        render_hints = {**render_hints}
        render_hints.setdefault("tier", tier)
        render_hints.setdefault("all_nodes_visible", True)
    stats["graph_tier"] = tier
    payload["stats"] = stats
    payload["graph_tier"] = tier
    payload["render_hints"] = render_hints
    return payload


def _is_missing_account_key(value: Any) -> bool:
    return classify_counterparty_placeholder(value) is not None


def _is_placeholder_node_id(value: Any) -> bool:
    return bool(placeholder_kind_from_token(value))


def _placeholder_node_id(kind: Any, name: Any = "") -> str:
    base = placeholder_key_token(kind)
    if not base:
        return ""
    node_name = str(name or "").strip()
    if not node_name:
        return base
    return f"{base}::name::{node_name}"


def _placeholder_kind_from_node_id(value: Any) -> str:
    return placeholder_kind_from_token(value)


def _normalize_flow_source(value: Any) -> str:
    source = str(value or "").strip().lower()
    if source == "stats-react":
        return "stats"
    return source


def _to_text_list(values: Any) -> list[str]:
    if not isinstance(values, (list, tuple)):
        return []
    out: list[str] = []
    seen: set[str] = set()
    for item in values:
        text = str(item or "").strip()
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
    return out


def _to_optional_float(value: Any) -> Optional[float]:
    if isinstance(value, bool):
        return None
    try:
        out = float(value)
    except Exception:
        return None
    if not math.isfinite(out):
        return None
    return out


def _required_amount_float(value: Any) -> float:
    if isinstance(value, bool):
        raise TxnAmountCoverageIncompleteError()
    normalized = _to_optional_float(value)
    if normalized is None or not math.isfinite(normalized):
        raise TxnAmountCoverageIncompleteError()
    return normalized


def _required_nonnegative_min_amount(value: Any) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError("flow_min_amount_invalid")
    normalized = float(value)
    if not math.isfinite(normalized) or normalized < 0:
        raise ValueError("flow_min_amount_invalid")
    return normalized


def _optional_expected_amount(value: Any) -> Optional[float]:
    if value is None:
        return None
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError("flow_expected_total_amount_invalid")
    normalized = float(value)
    if not math.isfinite(normalized) or normalized < 0:
        raise ValueError("flow_expected_total_amount_invalid")
    return normalized


def _project_node_total_amount(
    node_amounts: dict[str, float],
    node_id: str,
) -> Optional[float]:
    if node_id not in node_amounts:
        return None
    return _round2(node_amounts[node_id])


def _required_transaction_count(value: Any) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        raise TxnAmountCoverageIncompleteError()
    return value


def _to_optional_int(value: Any) -> Optional[int]:
    try:
        return int(value)
    except Exception:
        return None


def _clone_json(value: Any, fallback: Any) -> Any:
    try:
        text = dumps_canonical_json(
            value,
            max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
            max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
        )
        cloned = loads_strict_json(
            text,
            max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
            max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
        )
        if isinstance(fallback, dict) and not isinstance(cloned, dict):
            return dict(fallback)
        if isinstance(fallback, list) and not isinstance(cloned, list):
            return list(fallback)
        if isinstance(fallback, str) and not isinstance(cloned, str):
            return fallback
        return cloned
    except (StrictJSONError, UnicodeError):
        return fallback


def _normalize_graph_entities(values: Any) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    if not isinstance(values, list):
        return out
    for item in values:
        out.append(item if isinstance(item, dict) else {})
    return _clone_json(out, [])


def _normalize_projection_payload(value: Any) -> dict[str, Any]:
    source = value if isinstance(value, dict) else {}
    return _clone_json(source, {})


def _normalize_projection_request(value: Any) -> dict[str, Any]:
    source = value if isinstance(value, dict) else {}
    graph = source.get("graph") if isinstance(source.get("graph"), dict) else {}
    drill = source.get("drill") if isinstance(source.get("drill"), dict) else {}
    merged = {
        **_clone_json(graph, {}),
        **_clone_json(drill, {}),
        **_clone_json(source, {}),
    }
    render_mode = str(merged.get("render_mode") or merged.get("renderMode") or "").strip().lower()
    if render_mode not in {"", "auto", "full", "skeleton"}:
        render_mode = ""
    return {
        "render_mode": render_mode or "full",
        "cluster_ids": _to_text_list(merged.get("cluster_ids") or merged.get("clusterIds")),
        "tile_ids": _to_text_list(merged.get("tile_ids") or merged.get("tileIds")),
        "node_ids": _to_text_list(merged.get("node_ids") or merged.get("nodeIds")),
        "search_query": str(
            merged.get("search_query")
            or merged.get("searchQuery")
            or merged.get("query")
            or merged.get("q")
            or ""
        ).strip(),
        "search_limit": max(
            1,
            min(256, int(merged.get("search_limit") or merged.get("searchLimit") or merged.get("match_limit") or 24)),
        ),
        "path_node_ids": _to_text_list(
            merged.get("path_node_ids")
            or merged.get("pathNodeIds")
            or (
                merged.get("path", {}).get("node_ids")
                if isinstance(merged.get("path"), dict)
                else merged.get("path", {}).get("nodeIds")
                if isinstance(merged.get("path"), dict)
                else []
            )
        ),
        "viewport": _clone_json(merged.get("viewport"), {}) if isinstance(merged.get("viewport"), dict) else {},
        "include_neighbors": bool(merged.get("include_neighbors", merged.get("includeNeighbors", True))),
        "neighbor_depth": max(0, min(3, int(merged.get("neighbor_depth") or merged.get("neighborDepth") or 1))),
        "materialize_limit": max(
            1,
            min(50_000, int(merged.get("materialize_limit") or merged.get("materializeLimit") or 4_000)),
        ),
        "reset": bool(merged.get("reset")),
    }


def _should_use_skeleton_projection(request_context: Any) -> bool:
    request = _normalize_projection_request(request_context)
    return str(request.get("render_mode") or "").strip().lower() == "skeleton"


def _resolve_projection_render_mode(
    request_context: Any,
    result_payload: Optional[dict[str, Any]] = None,
) -> str:
    request = _normalize_projection_request(request_context)
    mode = str(request.get("render_mode") or "").strip().lower()
    if mode == "skeleton":
        return "skeleton"
    if mode != "auto":
        return "full"
    payload = result_payload if isinstance(result_payload, dict) else {}
    runtime_graph = payload.get("runtime_graph") if isinstance(payload.get("runtime_graph"), dict) else {}
    runtime_nodes = runtime_graph.get("nodes") if isinstance(runtime_graph.get("nodes"), list) else []
    runtime_edges = runtime_graph.get("edges") if isinstance(runtime_graph.get("edges"), list) else []
    node_count = len(runtime_nodes) or len(payload.get("nodes") or [])
    edge_count = len(runtime_edges) or len(payload.get("edges") or [])
    raw_context = request_context if isinstance(request_context, dict) else {}
    layout_mode = str(
        raw_context.get("layout")
        or raw_context.get("layoutPreset")
        or raw_context.get("layout_preset")
        or ""
    ).strip().lower()
    if (
        layout_mode == "network"
        and (
            node_count > _NETWORK_LAYOUT_LARGE_MIN_NODES
            or edge_count > _NETWORK_LAYOUT_LARGE_MIN_EDGES
        )
    ):
        return "skeleton"
    tier = str(payload.get("graph_tier") or _resolve_graph_tier(node_count)).strip().lower() or _resolve_graph_tier(node_count)
    return "skeleton" if tier in {"large", "xlarge"} else "full"


def _runtime_graph_to_entities(
    runtime_nodes: Sequence[dict[str, Any]],
    runtime_edges: Sequence[dict[str, Any]],
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    modern_nodes: list[dict[str, Any]] = []
    modern_edges: list[dict[str, Any]] = []

    for row in runtime_edges or []:
        item = row if isinstance(row, dict) else {}
        _required_amount_float(item.get("amount"))

    for row in runtime_nodes or []:
        item = row if isinstance(row, dict) else {}
        node_id = str(item.get("id") or "").strip()
        if not node_id:
            continue
        label = (
            str(item.get("name") or "").strip()
            or str(item.get("title") or "").strip()
            or str(item.get("display_id") or "").strip()
            or node_id
        )
        ntype = str(item.get("ntype") or "").strip().lower()
        display_id = str(item.get("display_id") or "").strip()
        if node_id.startswith("__unknown_cp__name::") or ntype == "unknown":
            node_type = "unknown"
        elif ntype in {"seed", "account"} or _looks_like_account(display_id or node_id):
            node_type = "account"
        else:
            node_type = "holder"
        modern_nodes.append(
            {
                "node_id": node_id,
                "label": label,
                "node_type": node_type,
                "risk_score": None,
            }
        )

    for row in runtime_edges or []:
        item = row if isinstance(row, dict) else {}
        edge_id = str(item.get("id") or "").strip()
        source = str(item.get("source") or "").strip()
        target = str(item.get("target") or "").strip()
        if not source or not target:
            continue
        modern_edges.append(
            {
                "edge_id": edge_id or f"{source}=={target}",
                "from_node_id": source,
                "to_node_id": target,
                "tx_count": _required_transaction_count(item.get("count")),
                "amount_total": _round2(item.get("amount")),
            }
        )
    return modern_nodes, modern_edges


def _normalize_view_graph_blob(value: Any) -> dict[str, Any]:
    source = value if isinstance(value, dict) else {}
    payload = {
        "nodes": _normalize_graph_entities(source.get("nodes")),
        "edges": _normalize_graph_entities(source.get("edges")),
    }
    raw_runtime_revision = (
        source.get("runtime_revision")
        if "runtime_revision" in source
        else source.get("runtimeRevision")
    )
    runtime_revision = _to_optional_int(raw_runtime_revision)
    if runtime_revision is not None and runtime_revision >= 0:
        payload["runtime_revision"] = runtime_revision
    return payload


def _graph_has_payload(graph: Any) -> bool:
    source = graph if isinstance(graph, dict) else {}
    return bool(source.get("nodes")) or bool(source.get("edges"))


def _normalize_snapshot_ref(value: Any) -> Optional[dict[str, Any]]:
    source = value if isinstance(value, dict) else {}
    snapshot_id = str(source.get("snapshot_id") or source.get("snapshotId") or source.get("graph_hash") or "").strip()
    graph_hash = str(source.get("graph_hash") or source.get("graphHash") or snapshot_id).strip()
    if not snapshot_id and not graph_hash:
        return None
    ref_id = snapshot_id or graph_hash
    out = {
        "snapshot_id": ref_id,
        "graph_hash": graph_hash or ref_id,
        "node_count": max(0, int(source.get("node_count") or source.get("nodeCount") or 0)),
        "edge_count": max(0, int(source.get("edge_count") or source.get("edgeCount") or 0)),
        "stored_at": str(source.get("stored_at") or source.get("storedAt") or "").strip(),
    }
    graph_storage_mode = str(
        source.get("graph_storage_mode") or source.get("graphStorageMode") or ""
    ).strip().lower()
    if graph_storage_mode in {"full", "delta"}:
        out["graph_storage_mode"] = graph_storage_mode
    return out


def _normalize_projection_viewport(value: Any) -> dict[str, Any]:
    source = value if isinstance(value, dict) else {}
    center = source.get("center") if isinstance(source.get("center"), dict) else {}
    size = source.get("size") if isinstance(source.get("size"), dict) else {}
    zoom = _to_optional_float(source.get("zoom"))
    center_x = _to_optional_float(center.get("x"))
    center_y = _to_optional_float(center.get("y"))
    width = _to_optional_float(size.get("width") or source.get("width"))
    height = _to_optional_float(size.get("height") or source.get("height"))
    out: dict[str, Any] = {}
    if zoom is not None and zoom > 0:
        out["zoom"] = float(zoom)
    if center_x is not None and center_y is not None:
        out["center"] = {"x": float(center_x), "y": float(center_y)}
    if width is not None and width > 0 and height is not None and height > 0:
        out["size"] = {"width": float(width), "height": float(height)}
    return out


def _select_projection_targets_from_viewport(
    runtime_graph: Any,
    viewport: Any,
    *,
    max_clusters: int = _PROJECTION_VIEWPORT_MAX_TARGET_CLUSTERS,
    max_nodes: int = _PROJECTION_VIEWPORT_MAX_TARGET_NODES,
) -> dict[str, list[str]]:
    graph = runtime_graph if isinstance(runtime_graph, dict) else {}
    normalized = _normalize_projection_viewport(viewport)
    center = normalized.get("center") if isinstance(normalized.get("center"), dict) else {}
    size = normalized.get("size") if isinstance(normalized.get("size"), dict) else {}
    zoom = _to_optional_float(normalized.get("zoom"))
    width = _to_optional_float(size.get("width"))
    height = _to_optional_float(size.get("height"))
    if (
        zoom is None
        or zoom <= 0
        or width is None
        or width <= 0
        or height is None
        or height <= 0
        or _to_optional_float(center.get("x")) is None
        or _to_optional_float(center.get("y")) is None
    ):
        return {"cluster_ids": [], "node_ids": []}

    targets = project_flow_graph_render_plan(
        render_plans=[],
        viewport_expand_targets={
            "runtimeGraph": {
                "nodes": graph.get("nodes") if isinstance(graph.get("nodes"), list) else [],
            },
            "viewport": normalized,
            "paddingX": float(width) * _PROJECTION_VIEWPORT_PADDING_RATIO * 0.5,
            "paddingY": float(height) * _PROJECTION_VIEWPORT_PADDING_RATIO * 0.5,
            "maxClusters": max(1, int(max_clusters or 1)),
            "maxNodes": max(1, int(max_nodes or 1)),
            "preferTiles": False,
            "projectDotClusterTargets": True,
        },
    ).get("viewportExpandTargets")
    target_map = targets if isinstance(targets, dict) else {}
    return {
        "cluster_ids": _to_text_list(target_map.get("clusterIds"))[: max(1, int(max_clusters or 1))],
        "node_ids": _to_text_list(target_map.get("nodeIds"))[: max(1, int(max_nodes or 1))],
    }


def _stable_json_text(value: Any) -> str:
    return dumps_canonical_json(
        value if value is not None else None,
        max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
        max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
    )


def _result_snapshot_hashed_payload(payload: dict[str, Any]) -> dict[str, Any]:
    hashed_payload: dict[str, Any] = {
        "version": payload.get("version"),
        "case_lifecycle_generation": payload.get("case_lifecycle_generation"),
        "case_lifecycle_binding_digest": payload.get("case_lifecycle_binding_digest"),
        "node_count": payload.get("node_count"),
        "edge_count": payload.get("edge_count"),
        "graph": payload.get("graph"),
        "result": payload.get("result"),
    }
    if "graph_patch" in payload:
        hashed_payload["graph_patch"] = payload.get("graph_patch")
    if "graph_base_snapshot_ref" in payload:
        hashed_payload["graph_base_snapshot_ref"] = payload.get("graph_base_snapshot_ref")
    hashed_payload["graph_storage_mode"] = payload.get("graph_storage_mode")
    hashed_payload["graph_storage_chain_depth"] = payload.get("graph_storage_chain_depth")
    return hashed_payload


def _result_snapshot_digest(payload: dict[str, Any]) -> str:
    canonical = dumps_canonical_json(
        _result_snapshot_hashed_payload(payload),
        max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
        max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
    )
    return hashlib.sha256(_FLOW_RESULT_SNAPSHOT_CONTENT_DOMAIN + canonical.encode("utf-8")).hexdigest()


def _result_snapshot_envelope_digest(payload: dict[str, Any]) -> str:
    envelope = {key: value for key, value in payload.items() if key != "envelope_sha256"}
    canonical = dumps_canonical_json(
        envelope,
        max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
        max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
    )
    return hashlib.sha256(_FLOW_RESULT_SNAPSHOT_ENVELOPE_DOMAIN + canonical.encode("utf-8")).hexdigest()


def _is_strict_nonnegative_int(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool) and value >= 0


def _is_sha256_hex(value: Any) -> bool:
    return (
        isinstance(value, str)
        and len(value) == 64
        and all(character in "0123456789abcdef" for character in value)
    )


def _is_owner_private_regular_file(item: os.stat_result) -> bool:
    return bool(
        stat.S_ISREG(item.st_mode)
        and int(item.st_nlink) == 1
        and int(item.st_mode) & 0o077 == 0
        and (not hasattr(os, "geteuid") or int(item.st_uid) == int(os.geteuid()))
    )


def _is_owner_private_directory(item: os.stat_result) -> bool:
    return bool(
        stat.S_ISDIR(item.st_mode)
        and int(item.st_mode) & 0o077 == 0
        and (not hasattr(os, "geteuid") or int(item.st_uid) == int(os.geteuid()))
    )


def _valid_result_snapshot_shape(payload: Any) -> bool:
    if not isinstance(payload, dict):
        return False
    common_keys = {
        "version",
        "case_lifecycle_generation",
        "case_lifecycle_binding_digest",
        "snapshot_id",
        "graph_hash",
        "node_count",
        "edge_count",
        "stored_at",
        "graph",
        "result",
        "graph_storage_mode",
        "graph_storage_chain_depth",
        "envelope_sha256",
    }
    storage_mode = payload.get("graph_storage_mode")
    expected_keys = common_keys if storage_mode == "full" else common_keys | {
        "graph_patch",
        "graph_base_snapshot_ref",
    }
    if set(payload) != expected_keys:
        return False
    if (
        type(payload.get("version")) is not int
        or payload.get("version") != _FLOW_RESULT_SNAPSHOT_VERSION
        or not _is_strict_nonnegative_int(payload.get("case_lifecycle_generation"))
        or payload.get("case_lifecycle_generation") < 1
        or not _is_sha256_hex(payload.get("case_lifecycle_binding_digest"))
        or not _is_sha256_hex(payload.get("snapshot_id"))
        or payload.get("graph_hash") != payload.get("snapshot_id")
        or not _is_sha256_hex(payload.get("envelope_sha256"))
        or not _is_strict_nonnegative_int(payload.get("node_count"))
        or not _is_strict_nonnegative_int(payload.get("edge_count"))
        or not isinstance(payload.get("stored_at"), str)
        or not payload["stored_at"]
        or payload["stored_at"].strip() != payload["stored_at"]
        or storage_mode not in {"full", "delta"}
        or not _is_strict_nonnegative_int(payload.get("graph_storage_chain_depth"))
    ):
        return False
    try:
        stored_at = datetime.fromisoformat(payload["stored_at"].replace("Z", "+00:00"))
    except ValueError:
        return False
    if stored_at.tzinfo is None:
        return False

    graph = payload.get("graph")
    graph_keys = {"nodes", "edges"}
    if not isinstance(graph, dict):
        return False
    observed_graph_keys = set(graph)
    if observed_graph_keys != graph_keys and observed_graph_keys != graph_keys | {"runtime_revision"}:
        return False
    if (
        not isinstance(graph.get("nodes"), list)
        or not isinstance(graph.get("edges"), list)
        or any(not isinstance(item, dict) for item in graph["nodes"])
        or any(not isinstance(item, dict) for item in graph["edges"])
        or (
            "runtime_revision" in graph
            and not _is_strict_nonnegative_int(graph.get("runtime_revision"))
        )
    ):
        return False

    result = payload.get("result")
    if not isinstance(result, dict) or set(result) != {
        "nodes",
        "edges",
        "stats",
        "publication_status",
        "fact_answer_allowed",
        "graph_tier",
        "render_hints",
        "projection",
    }:
        return False
    if (
        not isinstance(result.get("nodes"), list)
        or not isinstance(result.get("edges"), list)
        or any(not isinstance(item, dict) for item in result["nodes"])
        or any(not isinstance(item, dict) for item in result["edges"])
        or not isinstance(result.get("stats"), dict)
        or result.get("publication_status") != "blocked"
        or result.get("fact_answer_allowed") is not False
        or not isinstance(result.get("graph_tier"), str)
        or not isinstance(result.get("render_hints"), dict)
        or not isinstance(result.get("projection"), dict)
    ):
        return False

    node_count = payload["node_count"]
    edge_count = payload["edge_count"]
    chain_depth = payload["graph_storage_chain_depth"]
    if storage_mode == "full":
        return (
            chain_depth == 0
            and node_count == len(graph["nodes"])
            and edge_count == len(graph["edges"])
        )
    if (
        chain_depth <= 0
        or chain_depth > _RESULT_SNAPSHOT_DELTA_MAX_CHAIN_DEPTH
        or graph["nodes"]
        or graph["edges"]
    ):
        return False
    base_ref = payload.get("graph_base_snapshot_ref")
    if (
        not isinstance(base_ref, dict)
        or set(base_ref) != {"snapshot_id", "graph_hash"}
        or not _is_sha256_hex(base_ref.get("snapshot_id"))
        or base_ref.get("graph_hash") != base_ref.get("snapshot_id")
    ):
        return False
    patch = payload.get("graph_patch")
    if not isinstance(patch, dict) or set(patch) != {
        "remove_node_ids",
        "upsert_nodes",
        "remove_edge_ids",
        "upsert_edges",
        "summary",
    }:
        return False
    remove_node_ids = patch.get("remove_node_ids")
    remove_edge_ids = patch.get("remove_edge_ids")
    upsert_nodes = patch.get("upsert_nodes")
    upsert_edges = patch.get("upsert_edges")
    if (
        not isinstance(remove_node_ids, list)
        or not isinstance(remove_edge_ids, list)
        or not isinstance(upsert_nodes, list)
        or not isinstance(upsert_edges, list)
        or any(not isinstance(item, str) or not item for item in remove_node_ids + remove_edge_ids)
        or len(set(remove_node_ids)) != len(remove_node_ids)
        or len(set(remove_edge_ids)) != len(remove_edge_ids)
        or any(not isinstance(item, dict) for item in upsert_nodes + upsert_edges)
    ):
        return False
    summary = patch.get("summary")
    if not isinstance(summary, dict) or set(summary) != {
        "base_nodes",
        "base_edges",
        "target_nodes",
        "target_edges",
        "op_count",
        "target_entity_count",
    }:
        return False
    if any(not _is_strict_nonnegative_int(summary.get(key)) for key in summary):
        return False
    return (
        summary["target_nodes"] == node_count
        and summary["target_edges"] == edge_count
        and summary["target_entity_count"] == node_count + edge_count
        and summary["op_count"]
        == len(remove_node_ids) + len(remove_edge_ids) + len(upsert_nodes) + len(upsert_edges)
    )


def _runtime_node_identity(row: dict[str, Any], index: int) -> str:
    for key in ("id", "node_id", "display_id", "label", "title", "name"):
        value = str(row.get(key) or "").strip()
        if value:
            return value
    return f"node:{index}"


def _runtime_node_has_stable_identity(row: dict[str, Any]) -> bool:
    return any(str(row.get(key) or "").strip() for key in ("id", "node_id", "display_id", "label", "title", "name"))


def _runtime_edge_identity(row: dict[str, Any], index: int) -> str:
    for key in ("id", "edge_id"):
        value = str(row.get(key) or "").strip()
        if value:
            return value
    source = str(row.get("source") or row.get("from_node_id") or "").strip()
    target = str(row.get("target") or row.get("to_node_id") or "").strip()
    if source and target:
        mode = str(row.get("mode") or "").strip()
        arrow = str(row.get("edgeArrow") or row.get("arrow") or "").strip()
        label = str(row.get("label") or "").strip()
        amount = row.get("amount_total") if row.get("amount_total") is not None else row.get("amount")
        count = row.get("tx_count") if row.get("tx_count") is not None else row.get("count")
        return f"{source}->{target}|m:{mode}|a:{arrow}|l:{label}|amt:{amount}|cnt:{count}"
    return f"edge:{index}"


def _runtime_edge_has_stable_identity(row: dict[str, Any]) -> bool:
    if any(str(row.get(key) or "").strip() for key in ("id", "edge_id")):
        return True
    source = str(row.get("source") or row.get("from_node_id") or "").strip()
    target = str(row.get("target") or row.get("to_node_id") or "").strip()
    return bool(source and target)


def _runtime_graph_has_unique_identities(graph: dict[str, Any]) -> bool:
    normalized = _normalize_view_graph_blob(graph)
    if any(not _runtime_node_has_stable_identity(row) for row in normalized["nodes"]):
        return False
    if any(not _runtime_edge_has_stable_identity(row) for row in normalized["edges"]):
        return False
    node_ids = [
        _runtime_node_identity(row, index)
        for index, row in enumerate(normalized["nodes"])
    ]
    edge_ids = [
        _runtime_edge_identity(row, index)
        for index, row in enumerate(normalized["edges"])
    ]
    return len(node_ids) == len(set(node_ids)) and len(edge_ids) == len(set(edge_ids))


def _build_runtime_graph_patch(
    base_graph: dict[str, list[dict[str, Any]]], target_graph: dict[str, list[dict[str, Any]]]
) -> dict[str, Any]:
    normalized_base = _normalize_view_graph_blob(base_graph)
    normalized_target = _normalize_view_graph_blob(target_graph)
    if not _runtime_graph_has_unique_identities(normalized_base) or not _runtime_graph_has_unique_identities(
        normalized_target
    ):
        raise ValueError("flow_result_snapshot_non_unique_graph_identity")

    def _build_index(
        values: list[dict[str, Any]],
        identity_builder,
    ) -> dict[str, dict[str, Any]]:
        out: dict[str, dict[str, Any]] = {}
        for index, row in enumerate(values):
            item = row if isinstance(row, dict) else {}
            out[_clone_json(identity_builder(item, index), "")] = _clone_json(item, {})
        return out

    base_nodes = _build_index(normalized_base["nodes"], _runtime_node_identity)
    target_nodes = _build_index(normalized_target["nodes"], _runtime_node_identity)
    base_edges = _build_index(normalized_base["edges"], _runtime_edge_identity)
    target_edges = _build_index(normalized_target["edges"], _runtime_edge_identity)

    remove_node_ids = [node_id for node_id in base_nodes.keys() if node_id not in target_nodes]
    upsert_nodes = [
        row
        for node_id, row in target_nodes.items()
        if node_id not in base_nodes or _stable_json_text(base_nodes[node_id]) != _stable_json_text(row)
    ]
    remove_edge_ids = [edge_id for edge_id in base_edges.keys() if edge_id not in target_edges]
    upsert_edges = [
        row
        for edge_id, row in target_edges.items()
        if edge_id not in base_edges or _stable_json_text(base_edges[edge_id]) != _stable_json_text(row)
    ]

    op_count = len(remove_node_ids) + len(upsert_nodes) + len(remove_edge_ids) + len(upsert_edges)
    target_entity_count = len(normalized_target["nodes"]) + len(normalized_target["edges"])

    return {
        "remove_node_ids": remove_node_ids,
        "upsert_nodes": upsert_nodes,
        "remove_edge_ids": remove_edge_ids,
        "upsert_edges": upsert_edges,
        "summary": {
            "base_nodes": len(normalized_base["nodes"]),
            "base_edges": len(normalized_base["edges"]),
            "target_nodes": len(normalized_target["nodes"]),
            "target_edges": len(normalized_target["edges"]),
            "op_count": op_count,
            "target_entity_count": target_entity_count,
        },
    }


def _apply_runtime_graph_patch(
    base_graph: dict[str, Any],
    patch: dict[str, Any],
    *,
    runtime_revision: Optional[int] = None,
    inherit_runtime_revision: bool = True,
) -> dict[str, Any]:
    base = _normalize_view_graph_blob(base_graph)
    payload = patch if isinstance(patch, dict) else {}

    node_index: dict[str, dict[str, Any]] = {}
    for index, row in enumerate(_normalize_graph_entities(base.get("nodes"))):
        node_index[_runtime_node_identity(row, index)] = _clone_json(row, {})
    for raw_node_id in payload.get("remove_node_ids") or []:
        node_index.pop(str(raw_node_id or "").strip(), None)
    for index, row in enumerate(_normalize_graph_entities(payload.get("upsert_nodes"))):
        node_index[_runtime_node_identity(row, index)] = _clone_json(row, {})

    edge_index: dict[str, dict[str, Any]] = {}
    for index, row in enumerate(_normalize_graph_entities(base.get("edges"))):
        edge_index[_runtime_edge_identity(row, index)] = _clone_json(row, {})
    for raw_edge_id in payload.get("remove_edge_ids") or []:
        edge_index.pop(str(raw_edge_id or "").strip(), None)
    for index, row in enumerate(_normalize_graph_entities(payload.get("upsert_edges"))):
        edge_index[_runtime_edge_identity(row, index)] = _clone_json(row, {})

    out = {
        "nodes": list(node_index.values()),
        "edges": list(edge_index.values()),
    }
    next_runtime_revision = runtime_revision
    if next_runtime_revision is None and inherit_runtime_revision:
        next_runtime_revision = _to_optional_int(payload.get("runtime_revision"))
    if next_runtime_revision is None and inherit_runtime_revision:
        next_runtime_revision = _to_optional_int(base.get("runtime_revision") or base.get("runtimeRevision"))
    if next_runtime_revision is not None and next_runtime_revision >= 0:
        out["runtime_revision"] = int(next_runtime_revision)
    return out


def _validated_runtime_graph_patch(
    base_graph: dict[str, Any],
    target_graph: dict[str, Any],
    candidate_patch: Optional[dict[str, Any]] = None,
) -> Optional[dict[str, Any]]:
    base = _normalize_view_graph_blob(base_graph)
    target = _normalize_view_graph_blob(target_graph)
    if not _runtime_graph_has_unique_identities(base) or not _runtime_graph_has_unique_identities(target):
        return None
    try:
        patch = candidate_patch if isinstance(candidate_patch, dict) else _build_runtime_graph_patch(base, target)
        dumps_canonical_json(
            patch,
            max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
            max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
        )
    except (StrictJSONError, ValueError):
        return None
    if set(patch) != {
        "remove_node_ids",
        "upsert_nodes",
        "remove_edge_ids",
        "upsert_edges",
        "summary",
    }:
        return None
    remove_node_ids = patch.get("remove_node_ids")
    remove_edge_ids = patch.get("remove_edge_ids")
    upsert_nodes = patch.get("upsert_nodes")
    upsert_edges = patch.get("upsert_edges")
    if (
        not isinstance(remove_node_ids, list)
        or not isinstance(remove_edge_ids, list)
        or not isinstance(upsert_nodes, list)
        or not isinstance(upsert_edges, list)
        or any(not isinstance(item, str) or not item for item in remove_node_ids + remove_edge_ids)
        or len(remove_node_ids) != len(set(remove_node_ids))
        or len(remove_edge_ids) != len(set(remove_edge_ids))
        or any(not isinstance(item, dict) for item in upsert_nodes + upsert_edges)
    ):
        return None
    summary = patch.get("summary")
    if not isinstance(summary, dict) or set(summary) != {
        "base_nodes",
        "base_edges",
        "target_nodes",
        "target_edges",
        "op_count",
        "target_entity_count",
    }:
        return None
    if any(not _is_strict_nonnegative_int(summary.get(key)) for key in summary):
        return None
    expected_op_count = len(remove_node_ids) + len(remove_edge_ids) + len(upsert_nodes) + len(upsert_edges)
    if (
        summary["base_nodes"] != len(base["nodes"])
        or summary["base_edges"] != len(base["edges"])
        or summary["target_nodes"] != len(target["nodes"])
        or summary["target_edges"] != len(target["edges"])
        or summary["target_entity_count"] != len(target["nodes"]) + len(target["edges"])
        or summary["op_count"] != expected_op_count
    ):
        return None
    target_revision = _to_optional_int(target.get("runtime_revision"))
    reconstructed = _apply_runtime_graph_patch(
        base,
        patch,
        runtime_revision=target_revision,
        inherit_runtime_revision=False,
    )
    try:
        if _stable_json_text(reconstructed) != _stable_json_text(target):
            return None
    except StrictJSONError:
        return None
    if len(reconstructed["nodes"]) != len(target["nodes"]) or len(reconstructed["edges"]) != len(target["edges"]):
        return None
    return _clone_json(patch, None)


def _build_result_snapshot_patch_payload(
    *,
    base_ref: dict[str, Any],
    target_ref: dict[str, Any],
    base_result: dict[str, Any],
    target_result: dict[str, Any],
    precomputed_patch: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    target_graph = _normalize_view_graph_blob(target_result.get("runtime_graph"))
    base_graph = _normalize_view_graph_blob(base_result.get("runtime_graph"))
    patch = _validated_runtime_graph_patch(base_graph, target_graph, precomputed_patch)
    summary = patch.get("summary") if isinstance(patch, dict) else {}
    op_count = int(summary.get("op_count") or 0)
    target_entity_count = int(summary.get("target_entity_count") or 0)
    use_delta = patch is not None and target_entity_count > 0 and op_count < target_entity_count
    trace_id = (
        f"flow-snapshot-patch:{str(base_ref.get('snapshot_id') or base_ref.get('graph_hash') or '')[:12]}"
        f":{str(target_ref.get('snapshot_id') or target_ref.get('graph_hash') or '')[:12]}"
    )
    return _attach_graph_render_metadata({
        "base_snapshot_ref": base_ref,
        "result_snapshot_ref": target_ref,
        "nodes": _clone_json(target_result.get("nodes"), []),
        "edges": _clone_json(target_result.get("edges"), []),
        "stats": _clone_json(target_result.get("stats"), {}),
        "runtime_graph": {"nodes": [], "edges": []} if use_delta else target_graph,
        "runtime_graph_patch": patch if use_delta else None,
        "patch_kind": "delta" if use_delta else "full",
        "base_runtime_revision": _to_optional_int(base_result.get("runtime_graph", {}).get("runtime_revision")),
        "runtime_revision": _to_optional_int(target_graph.get("runtime_revision")),
        "trace_id": trace_id,
        "graph_tier": target_result.get("graph_tier") or "",
        "render_hints": target_result.get("render_hints") or {},
        "projection": _clone_json(target_result.get("projection"), {}),
    })


def _looks_like_account(value: str) -> bool:
    s = str(value or "").strip()
    if not s:
        return False
    if _is_missing_account_key(s):
        return False
    if s.startswith("__unknown__"):
        return False
    if s.isdigit() and len(s) >= 8:
        return True
    if any(ch.isdigit() for ch in s) and len(s) >= 10:
        return True
    return False


def _parse_ymd(value: Any) -> Optional[datetime]:
    text = str(value or "").strip()
    if not text:
        return None
    try:
        return datetime.strptime(text[:10], "%Y-%m-%d")
    except Exception:
        return None


def _dt_to_str(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    try:
        return value.strftime("%Y-%m-%d %H:%M:%S")
    except Exception:
        return str(value)


@dataclass
class _EdgeAgg:
    a: str
    b: str
    amount_total: float = 0.0
    tx_count: int = 0
    out_amount: float = 0.0  # a -> b
    in_amount: float = 0.0  # b -> a


class FlowViewNotFoundError(KeyError):
    pass


class FlowJobResultManifestError(RuntimeError):
    pass


class FlowResultSnapshotError(RuntimeError):
    pass


class FlowViewMetadataError(RuntimeError):
    pass


def _merge_projection_expand_request(
    base_projection: Optional[dict[str, Any]],
    expand_request: Optional[dict[str, Any]],
) -> dict[str, Any]:
    base = base_projection if isinstance(base_projection, dict) else {}
    req = _normalize_projection_request(expand_request)
    raw = expand_request if isinstance(expand_request, dict) else {}
    raw_graph = raw.get("graph") if isinstance(raw.get("graph"), dict) else {}
    raw_drill = raw.get("drill") if isinstance(raw.get("drill"), dict) else {}
    explicit_mode = str(
        raw.get("render_mode")
        or raw.get("renderMode")
        or raw_drill.get("render_mode")
        or raw_drill.get("renderMode")
        or raw_graph.get("render_mode")
        or raw_graph.get("renderMode")
        or ""
    ).strip().lower()
    if explicit_mode != "full":
        req["render_mode"] = "skeleton"
    if req.get("reset"):
        return req
    merged_cluster_ids = _to_text_list((base.get("expanded_cluster_ids") or [])) + _to_text_list(req.get("cluster_ids"))
    merged_tile_ids = _to_text_list((base.get("expanded_tile_ids") or [])) + _to_text_list(req.get("tile_ids"))
    merged_node_ids = _to_text_list((base.get("expanded_node_ids") or [])) + _to_text_list(req.get("node_ids"))
    merged_path_node_ids = _to_text_list((base.get("path_node_ids") or [])) + _to_text_list(req.get("path_node_ids"))
    return {
        **req,
        "cluster_ids": _to_text_list(merged_cluster_ids),
        "tile_ids": _to_text_list(merged_tile_ids),
        "node_ids": _to_text_list(merged_node_ids),
        "path_node_ids": _to_text_list(merged_path_node_ids),
        "search_query": str(req.get("search_query") or "").strip(),
        "search_limit": max(1, int(req.get("search_limit") or 24)),
    }


def _build_projected_result(
    *,
    full_result: dict[str, Any],
    source_snapshot_ref: Optional[dict[str, Any]],
    request_context: Optional[dict[str, Any]] = None,
    base_projection: Optional[dict[str, Any]] = None,
) -> Optional[dict[str, Any]]:
    full_runtime_graph = full_result.get("runtime_graph") if isinstance(full_result.get("runtime_graph"), dict) else {}
    full_nodes = [node for node in (full_runtime_graph.get("nodes") or []) if isinstance(node, dict)]
    full_edges = [edge for edge in (full_runtime_graph.get("edges") or []) if isinstance(edge, dict)]
    if not full_nodes:
        return None

    projection_request = _normalize_projection_request(request_context)
    request_mode = str(projection_request.get("render_mode") or "").strip().lower()
    if request_mode == "full":
        copied = {
            "nodes": _normalize_graph_entities(full_result.get("nodes")),
            "edges": _normalize_graph_entities(full_result.get("edges")),
            "stats": _clone_json(full_result.get("stats"), {}),
            "runtime_graph": full_runtime_graph,
            "graph_tier": str(full_result.get("graph_tier") or ""),
            "render_hints": _clone_json(full_result.get("render_hints"), {}),
            "projection": {
                "mode": "full",
                "source_result_snapshot_ref": _clone_json(source_snapshot_ref, None),
            },
        }
        return _attach_graph_render_metadata(copied)

    full_graph_tier = str(full_result.get("graph_tier") or _resolve_graph_tier(len(full_nodes))).strip().lower() or "small"
    use_point_layer = full_graph_tier == "xlarge"
    tile_size = _PROJECTION_TILE_SIZE_XLARGE if use_point_layer else _PROJECTION_TILE_SIZE_LARGE
    projection_model_request = {
        **(_clone_json(request_context, {}) if isinstance(request_context, dict) else {}),
        **projection_request,
        "use_point_layer": use_point_layer,
        "view_mode": str((full_result.get("stats") or {}).get("view_mode") or "relation").strip().lower()
        or "relation",
    }
    cluster_state = project_flow_skeleton_clusters(
        nodes=full_nodes,
        edges=full_edges,
        request_context=projection_model_request,
        base_projection=base_projection,
        tile_size=tile_size,
    )
    anchor_ids = _to_text_list(cluster_state.get("anchor_ids"))

    if (
        "base_projection_node_ids" not in cluster_state
        or "explicit_entity_ids" not in cluster_state
        or "requested_cluster_materializations" not in cluster_state
        or "cluster_visibility_plan" not in cluster_state
        or "projected_node_plan" not in cluster_state
        or "projected_edge_plan" not in cluster_state
    ):
        raise RuntimeError("Rust analysis compute project-flow-skeleton-clusters returned no projection entity materialization")
    search_match_node_ids = _to_text_list(cluster_state.get("search_match_node_ids"))

    visibility_plan = cluster_state.get("cluster_visibility_plan")
    if not isinstance(visibility_plan, dict):
        raise RuntimeError("Rust analysis compute project-flow-skeleton-clusters returned no cluster visibility plan")
    projected_node_plan = cluster_state.get("projected_node_plan")
    if (
        not isinstance(projected_node_plan, dict)
        or not isinstance(projected_node_plan.get("nodes"), list)
        or not isinstance(projected_node_plan.get("clusters"), list)
    ):
        raise RuntimeError("Rust analysis compute project-flow-skeleton-clusters returned no projected node plan")
    projected_edge_plan = cluster_state.get("projected_edge_plan")
    if not isinstance(projected_edge_plan, dict) or not isinstance(projected_edge_plan.get("edges"), list):
        raise RuntimeError("Rust analysis compute project-flow-skeleton-clusters returned no projected edge plan")

    projected_nodes = _normalize_graph_entities(projected_node_plan.get("nodes"))
    cluster_rows = [
        dict(item) for item in projected_node_plan.get("clusters") or [] if isinstance(item, dict)
    ]
    dot_count = _required_transaction_count(projected_node_plan.get("dot_count"))
    entity_count = _required_transaction_count(projected_node_plan.get("entity_count"))
    expanded_cluster_ids = _to_text_list(projected_node_plan.get("expanded_cluster_ids"))
    expanded_tile_ids = _to_text_list(projected_node_plan.get("expanded_tile_ids"))
    expanded_node_ids = _to_text_list(projected_node_plan.get("expanded_node_ids"))
    point_layer_buckets = [
        dict(item) for item in projected_node_plan.get("point_layer_buckets") or [] if isinstance(item, dict)
    ]
    point_layer_total = _required_transaction_count(projected_node_plan.get("point_layer_total"))
    projected_edges = _normalize_graph_entities(projected_edge_plan.get("edges"))
    total_amount_from_edges = _round2(_required_amount_float(projected_edge_plan.get("total_amount")))
    total_count_from_edges = _required_transaction_count(projected_edge_plan.get("total_count"))

    modern_nodes, modern_edges = _runtime_graph_to_entities(projected_nodes, projected_edges)
    stats = _clone_json(full_result.get("stats"), {})
    stats["node_count"] = len(modern_nodes)
    stats["edge_count"] = len(modern_edges)
    stats["full_node_count"] = len(full_nodes)
    stats["full_edge_count"] = len(full_edges)
    stats["visible_leaf_node_count"] = len(full_nodes)
    stats["projected_dot_node_count"] = dot_count
    stats["projected_entity_node_count"] = entity_count
    stats["projected_cluster_count"] = len([row for row in cluster_rows if not row.get("expanded")])
    stats["projected_partial_cluster_count"] = len([row for row in cluster_rows if row.get("partially_expanded")])
    stats["projected_materialized_member_count"] = sum(
        _required_transaction_count(row.get("materialized_member_count")) for row in cluster_rows
    )
    stats["projection_mode"] = "skeleton"
    stats["total_amount"] = _round2(total_amount_from_edges)
    stats["total_count"] = int(total_count_from_edges)
    if use_point_layer:
        stats["projected_point_node_count"] = point_layer_total

    render_hints = _clone_json(full_result.get("render_hints"), {})
    render_hints["tier"] = str(full_result.get("graph_tier") or render_hints.get("tier") or _resolve_graph_tier(len(projected_nodes)))
    render_hints["all_nodes_visible"] = True
    render_hints["default_node_mode"] = "mixed"
    render_hints["edge_mode"] = "skeleton"
    render_hints["show_edge_labels"] = False
    render_hints["show_detail_edge_labels"] = False
    render_hints["prefer_fast_first_paint"] = True
    render_hints["point_layer_mode"] = "clustered" if use_point_layer and point_layer_total > 0 else "none"

    projection_payload = {
        "mode": "skeleton",
        "source_result_snapshot_ref": _clone_json(source_snapshot_ref, None),
        "anchor_node_ids": anchor_ids,
        "clusters": cluster_rows,
        "expanded_cluster_ids": sorted(expanded_cluster_ids),
        "expanded_tile_ids": sorted(expanded_tile_ids),
        "expanded_node_ids": expanded_node_ids,
        "path_node_ids": _to_text_list(projection_request.get("path_node_ids")),
        "visible_leaf_node_count": len(full_nodes),
        "cluster_node_count": len([row for row in cluster_rows if not row.get("expanded")]),
        "entity_node_count": entity_count,
        "dot_node_count": dot_count,
        "full_node_count": len(full_nodes),
        "full_edge_count": len(full_edges),
        "partially_expanded_cluster_ids": sorted(
            str(row.get("cluster_id") or "").strip()
            for row in cluster_rows
            if row.get("partially_expanded") and str(row.get("cluster_id") or "").strip()
        ),
        "search_query": str(projection_request.get("search_query") or "").strip(),
        "search_match_node_ids": search_match_node_ids,
    }
    if use_point_layer and point_layer_total > 0:
        projection_payload["point_layer"] = {
            "mode": "clustered",
            "total_point_count": point_layer_total,
            "bucket_count": len(point_layer_buckets),
            "buckets": point_layer_buckets,
        }

    return _attach_graph_render_metadata(
        {
            "nodes": modern_nodes,
            "edges": modern_edges,
            "stats": stats,
            "runtime_graph": {
                "nodes": projected_nodes,
                "edges": projected_edges,
            },
            "graph_tier": str(full_result.get("graph_tier") or ""),
            "render_hints": render_hints,
            "projection": projection_payload,
            "_partial_expand_fast_path": True,
        }
    )


def _edge_endpoint_ids(edge: dict[str, Any]) -> tuple[str, str]:
    source = str(edge.get("source") or edge.get("from_node_id") or "").strip()
    target = str(edge.get("target") or edge.get("to_node_id") or "").strip()
    return source, target


def _runtime_edge_key(edge: dict[str, Any], index: int = 0) -> str:
    edge_id = str(edge.get("id") or edge.get("edge_id") or "").strip()
    if edge_id:
        return edge_id
    source, target = _edge_endpoint_ids(edge)
    return f"{source}=={target}::{index}" if source and target else f"edge::{index}"


def _try_build_partial_projected_result(
    *,
    full_result: dict[str, Any],
    source_snapshot_ref: Optional[dict[str, Any]],
    request_context: Optional[dict[str, Any]],
    base_result: dict[str, Any],
    base_projection: dict[str, Any],
) -> Optional[dict[str, Any]]:
    projection_request = _normalize_projection_request(request_context)
    if (
        str(projection_request.get("render_mode") or "").strip().lower() != "skeleton"
        or projection_request.get("reset")
        or str(projection_request.get("search_query") or "").strip()
        or projection_request.get("viewport")
    ):
        return None
    cluster_ids = _to_text_list(projection_request.get("cluster_ids"))
    direct_node_ids = _to_text_list(projection_request.get("node_ids")) + _to_text_list(
        projection_request.get("path_node_ids")
    )
    if not cluster_ids and not direct_node_ids:
        return None
    full_runtime_graph = _normalize_view_graph_blob(full_result.get("runtime_graph"))
    full_nodes = _normalize_graph_entities(full_runtime_graph.get("nodes"))
    full_edges = _normalize_graph_entities(full_runtime_graph.get("edges"))
    if len(full_nodes) <= 5000 or str(full_result.get("graph_tier") or "").strip().lower() != "xlarge":
        return None
    base_runtime_graph = base_result.get("runtime_graph") if isinstance(base_result.get("runtime_graph"), dict) else {}
    base_nodes = [node for node in (base_runtime_graph.get("nodes") or []) if isinstance(node, dict)]
    base_edges = [edge for edge in (base_runtime_graph.get("edges") or []) if isinstance(edge, dict)]
    if not base_nodes:
        return None

    full_node_by_id = {
        str(node.get("id") or "").strip(): node
        for node in full_nodes
        if str(node.get("id") or "").strip()
    }
    base_node_by_id = {
        str(node.get("id") or "").strip(): node
        for node in base_nodes
        if str(node.get("id") or "").strip()
    }
    visible_ids = set(base_node_by_id.keys())
    cluster_rows = [
        dict(row)
        for row in (base_projection.get("clusters") or [])
        if isinstance(row, dict) and str(row.get("cluster_id") or row.get("clusterId") or "").strip()
    ]
    cluster_by_id = {
        str(row.get("cluster_id") or row.get("clusterId") or "").strip(): row
        for row in cluster_rows
    }
    materialize_limit = max(1, min(512, int(projection_request.get("materialize_limit") or 64)))
    selected_ids: list[str] = []
    selected_set: set[str] = set()

    def remember(node_id: str) -> None:
        node_id = str(node_id or "").strip()
        if (
            not node_id
            or node_id in selected_set
            or node_id in visible_ids
            or node_id.startswith(_CLUSTER_NODE_PREFIX)
            or node_id not in full_node_by_id
            or len(selected_ids) >= materialize_limit
        ):
            return
        selected_set.add(node_id)
        selected_ids.append(node_id)

    for node_id in direct_node_ids:
        remember(node_id)

    def candidate_score(node_id: str) -> tuple[int, float, int, str]:
        node = full_node_by_id.get(node_id, {})
        raw_amount = node.get("total_amount")
        if raw_amount is None:
            raw_amount = node.get("amount")
        amount = _to_optional_float(raw_amount)
        degree = max(0, int(node.get("degree") or 0))
        if amount is None:
            return (1, 0.0, -degree, node_id)
        return (0, -abs(amount), -degree, node_id)

    for cluster_id in cluster_ids:
        if len(selected_ids) >= materialize_limit:
            break
        row = cluster_by_id.get(cluster_id)
        if not row:
            continue
        cluster_node = base_node_by_id.get(cluster_id, {})
        anchor_ids = _to_text_list(row.get("anchor_ids") or row.get("anchorIds"))
        anchor_id = str(row.get("anchor_id") or row.get("anchorId") or "").strip()
        if anchor_id:
            anchor_ids = [anchor_id] + [item for item in anchor_ids if item != anchor_id]
        candidates: list[str] = []
        candidates.extend(_to_text_list(row.get("remaining_member_ids") or row.get("remainingMemberIds")))
        candidates.extend(_to_text_list(row.get("member_ids") or row.get("memberIds")))
        candidates.extend(_to_text_list(cluster_node.get("display_ids") or cluster_node.get("displayIds")))
        anchor_set = set(anchor_ids)
        if anchor_set:
            for edge in full_edges:
                source, target = _edge_endpoint_ids(edge)
                if source in anchor_set:
                    candidates.append(target)
                elif target in anchor_set:
                    candidates.append(source)
        ranked = sorted(dict.fromkeys(candidates), key=candidate_score)
        for node_id in ranked:
            remember(node_id)
            if len(selected_ids) >= materialize_limit:
                break

    if not selected_ids:
        return None

    next_nodes = [dict(node) for node in base_nodes]
    partial_upsert_nodes: list[dict[str, Any]] = []
    selected_cluster_by_node: dict[str, str] = {}
    for cluster_id in cluster_ids:
        for node_id in selected_ids:
            selected_cluster_by_node.setdefault(node_id, cluster_id)
    for node_id in selected_ids:
        node = dict(full_node_by_id[node_id])
        cluster_id = selected_cluster_by_node.get(node_id, "")
        if cluster_id:
            node["projection_cluster_id"] = cluster_id
        node["projection_visible"] = True
        node["projection_collapsed"] = False
        node.setdefault("nodeRenderMode", "entity")
        next_nodes.append(node)
        partial_upsert_nodes.append(dict(node))
        visible_ids.add(node_id)

    edge_keys = {_runtime_edge_key(edge, index) for index, edge in enumerate(base_edges)}
    next_edges = [dict(edge) for edge in base_edges]
    partial_upsert_edges: list[dict[str, Any]] = []
    for index, edge in enumerate(full_edges):
        source, target = _edge_endpoint_ids(edge)
        if not source or not target or source not in visible_ids or target not in visible_ids:
            continue
        if source not in selected_set and target not in selected_set:
            continue
        edge_key = _runtime_edge_key(edge, index)
        if edge_key in edge_keys:
            continue
        edge_keys.add(edge_key)
        edge_row = dict(edge)
        next_edges.append(edge_row)
        partial_upsert_edges.append(dict(edge_row))

    projection_payload = _clone_json(base_projection, {})
    projection_payload["mode"] = "skeleton"
    if source_snapshot_ref:
        projection_payload["source_result_snapshot_ref"] = _clone_json(source_snapshot_ref, None)
    projection_payload["expanded_node_ids"] = _to_text_list(
        (projection_payload.get("expanded_node_ids") or [])
    ) + [node_id for node_id in selected_ids if node_id not in _to_text_list(projection_payload.get("expanded_node_ids"))]
    projection_payload["path_node_ids"] = _to_text_list(projection_payload.get("path_node_ids")) + [
        node_id
        for node_id in _to_text_list(projection_request.get("path_node_ids"))
        if node_id not in _to_text_list(projection_payload.get("path_node_ids"))
    ]
    expanded_cluster_ids = set(_to_text_list(projection_payload.get("expanded_cluster_ids")))
    expanded_cluster_ids.update(cluster_ids)
    projection_payload["expanded_cluster_ids"] = sorted(expanded_cluster_ids)
    partial_cluster_ids = set(_to_text_list(projection_payload.get("partially_expanded_cluster_ids")))
    partial_cluster_ids.update(cluster_ids)
    projection_payload["partially_expanded_cluster_ids"] = sorted(partial_cluster_ids)
    updated_clusters = []
    for row in cluster_rows:
        cluster_id = str(row.get("cluster_id") or row.get("clusterId") or "").strip()
        next_row = dict(row)
        if cluster_id in cluster_ids:
            added = len([node_id for node_id in selected_ids if selected_cluster_by_node.get(node_id) == cluster_id])
            next_row["partially_expanded"] = True
            next_row["materialized_member_count"] = _required_transaction_count(
                next_row.get("materialized_member_count")
            ) + added
            remaining = max(0, _required_transaction_count(next_row.get("remaining_member_count")) - added)
            next_row["remaining_member_count"] = remaining
            next_row["expanded"] = remaining == 0 and added > 0
        updated_clusters.append(next_row)
    if updated_clusters:
        projection_payload["clusters"] = updated_clusters

    modern_nodes, modern_edges = _runtime_graph_to_entities(next_nodes, next_edges)
    stats = _clone_json(base_result.get("stats"), {})
    stats["node_count"] = len(modern_nodes)
    stats["edge_count"] = len(modern_edges)
    stats["full_node_count"] = len(full_nodes)
    stats["full_edge_count"] = len(full_edges)
    stats["projection_mode"] = "skeleton"
    stats["projected_entity_node_count"] = len(
        [node for node in next_nodes if str(node.get("nodeRenderMode") or "").strip().lower() != "dot"]
    )
    stats["projected_materialized_member_count"] = _required_transaction_count(
        stats.get("projected_materialized_member_count")
    ) + len(selected_ids)

    return _attach_graph_render_metadata(
        {
            "nodes": modern_nodes,
            "edges": modern_edges,
            "stats": stats,
            "runtime_graph": {
                "nodes": next_nodes,
                "edges": next_edges,
            },
            "graph_tier": str(full_result.get("graph_tier") or base_result.get("graph_tier") or ""),
            "render_hints": _clone_json(base_result.get("render_hints") or full_result.get("render_hints"), {}),
            "projection": projection_payload,
            "_partial_expand_fast_path": True,
            "_partial_graph_patch": {
                "remove_node_ids": [],
                "upsert_nodes": partial_upsert_nodes,
                "remove_edge_ids": [],
                "upsert_edges": partial_upsert_edges,
                "summary": {
                    "base_nodes": len(base_nodes),
                    "base_edges": len(base_edges),
                    "target_nodes": len(next_nodes),
                    "target_edges": len(next_edges),
                    "op_count": len(partial_upsert_nodes) + len(partial_upsert_edges),
                    "target_entity_count": len(next_nodes) + len(next_edges),
                },
            },
        }
    )


class FlowRepository:
    """Repository for flow graph queries and flow view persistence."""

    def __init__(self) -> None:
        self._storage = CaseStorage()
        self._daily_agg = TxnDailyAggregateStore(self._storage)
        self._purge_legacy_flow_view_artifacts()
        self._purge_all_legacy_stats_focus_fact_caches()
        self._purge_legacy_result_snapshot_aliases()

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def case_exists(self, case_id: str) -> bool:
        return self._storage.get_case(case_id) is not None

    def _can_use_materialized_graph_source(
        self,
        *,
        min_amount: float,
        request_context: Optional[dict[str, Any]] = None,
    ) -> bool:
        normalized_min_amount = _required_nonnegative_min_amount(min_amount)
        context_obj = request_context if isinstance(request_context, dict) else {}
        if bool(context_obj.get("__use_materialized_source") or context_obj.get("_use_materialized_source")):
            return True
        if normalized_min_amount > 0:
            return False
        projection_request = _normalize_projection_request(context_obj)
        return str(projection_request.get("render_mode") or "").strip().lower() in {"auto", "skeleton"}

    def _apply_account_display_metadata(
        self,
        *,
        con: DuckDBEngine,
        case_id: str,
        account_ids: Iterable[str],
        node_display: dict[str, str],
        node_display_list: dict[str, list[str]],
        node_name: dict[str, str],
        node_name_source: dict[str, str],
    ) -> None:
        keys = sorted({str(item or "").strip() for item in account_ids if str(item or "").strip()})
        if not keys:
            return
        rows = self._query_account_display_rows(con=con, case_id=case_id, account_ids=keys)
        for row in rows:
            account_key = str(row.get("account_key") or "").strip()
            if not account_key:
                continue
            card_display = str(row.get("card_display") or "").strip()
            acct_display = str(row.get("acct_display") or "").strip()
            open_name = str(row.get("open_name") or "").strip()
            display_id = card_display or acct_display or account_key
            if display_id:
                node_display[account_key] = display_id
            display_ids = []
            for value in str(row.get("card_aliases") or "").split("|"):
                value = value.strip()
                if value and value not in display_ids:
                    display_ids.append(value)
            for value in (card_display, acct_display):
                if value and value not in display_ids:
                    display_ids.append(value)
            if display_ids:
                node_display_list[account_key] = display_ids
            if open_name and node_name_source.get(account_key) != "open":
                node_name[account_key] = open_name
                node_name_source[account_key] = "open"

    def _query_account_display_rows(
        self,
        *,
        con: DuckDBEngine,
        case_id: str,
        account_ids: Sequence[str],
    ) -> list[dict[str, Any]]:
        keys = [str(item or "").strip() for item in account_ids if str(item or "").strip()]
        if not keys:
            return []
        placeholders = ",".join(["?"] * len(keys))
        if self._table_exists(con, "analysis_account_dim"):
            try:
                rows = con.query(
                    "SELECT account_key, acct_display, card_display, open_name, '' AS card_aliases "
                    f"FROM analysis_account_dim WHERE account_key IN ({placeholders})",
                    tuple(keys),
                )
                if rows:
                    return [
                        {
                            "account_key": row[0],
                            "acct_display": row[1],
                            "card_display": row[2],
                            "open_name": row[3],
                            "card_aliases": row[4],
                        }
                        for row in rows
                    ]
            except Exception:
                pass
        if not self._table_exists(con, "fc_transaction_norm"):
            return []
        cols = self._table_columns(con, "fc_transaction_norm")
        acct_key_expr = _account_key_expr("t", cols, candidates=_ACCOUNT_KEY_CANDIDATES)
        if acct_key_expr == "NULL":
            return []
        card_expr = _coalesce_expr(
            [_text_col_expr("t", cols, column, clean_invalid=True) for column in ("clean_card_no", "card_no_norm", "card_no")]
        )
        acct_expr = _coalesce_expr(
            [_text_col_expr("t", cols, column, clean_invalid=True) for column in ("clean_acct_no", "acct_no_norm", "acct_no")]
        )
        open_name_expr = _text_col_expr("t", cols, "account_open_name", clean_invalid=True) or "NULL"
        ts_expr = _txn_ts_expr("t", cols)
        if card_expr == "NULL" and acct_expr == "NULL":
            return []
        try:
            rows = con.query(
                "WITH base AS ("
                "  SELECT "
                f"    {acct_key_expr} AS account_key, "
                f"    {acct_expr} AS acct_display, "
                f"    {card_expr} AS card_display, "
                f"    {open_name_expr} AS open_name, "
                f"    {ts_expr} AS txn_ts "
                "  FROM fc_transaction_norm t "
                "  WHERE t.case_id=?"
                "), filtered AS ("
                f"  SELECT * FROM base WHERE account_key IN ({placeholders})"
                "), aliases AS ("
                "  SELECT account_key, STRING_AGG(DISTINCT card_display, '|') AS card_aliases "
                "  FROM filtered "
                "  WHERE card_display IS NOT NULL AND TRIM(card_display) <> '' "
                "  GROUP BY account_key"
                "), picked AS ("
                "  SELECT account_key, acct_display, card_display, open_name, txn_ts, "
                "         ROW_NUMBER() OVER ("
                "           PARTITION BY account_key "
                "           ORDER BY (txn_ts IS NOT NULL) DESC, txn_ts DESC, card_display DESC"
                "         ) AS rn "
                "  FROM filtered"
                ") "
                "SELECT picked.account_key, picked.acct_display, picked.card_display, picked.open_name, aliases.card_aliases "
                "FROM picked LEFT JOIN aliases ON aliases.account_key=picked.account_key "
                "WHERE picked.rn=1",
                tuple([case_id] + keys),
            )
        except Exception:
            return []
        return [
            {
                "account_key": row[0],
                "acct_display": row[1],
                "card_display": row[2],
                "open_name": row[3],
                "card_aliases": row[4],
            }
            for row in rows
        ]

    def _build_projection_source_query(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict[str, Any]] = None,
        prefer_materialized_source: bool = False,
    ) -> dict[str, Any]:
        context_obj = _clone_json(request_context if isinstance(request_context, dict) else {}, {})
        graph_ctx = context_obj.get("graph") if isinstance(context_obj.get("graph"), dict) else {}
        graph_ctx = {**_clone_json(graph_ctx, {}), "render_mode": "full"}
        context_obj["graph"] = graph_ctx
        if prefer_materialized_source:
            context_obj["__use_materialized_source"] = True
        else:
            context_obj.pop("__use_materialized_source", None)
        return {
            "case_id": str(case_id or "").strip(),
            "seeds": [str(item or "").strip() for item in seeds if str(item or "").strip()],
            "depth": max(1, int(depth or 1)),
            "direction": direction if direction in {"in", "out", "both"} else "both",
            "min_amount": _required_nonnegative_min_amount(min_amount),
            "request_context": context_obj,
        }

    def _build_graph_result_from_runtime_graph(self, runtime_graph: dict[str, Any]) -> dict[str, Any]:
        runtime_nodes = list(runtime_graph.get("nodes") or [])
        runtime_edges = list(runtime_graph.get("edges") or [])
        modern_nodes, modern_edges = _runtime_graph_to_entities(runtime_nodes, runtime_edges)
        stats = dict(runtime_graph.get("stats") or {})
        stats["node_count"] = len(modern_nodes)
        stats["edge_count"] = len(modern_edges)
        return _attach_graph_render_metadata(
            {
                "nodes": modern_nodes,
                "edges": modern_edges,
                "stats": stats,
                "runtime_graph": {
                    "nodes": runtime_nodes,
                    "edges": runtime_edges,
                },
            }
        )

    def _build_materialized_graph_result_bundle(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict[str, Any]] = None,
        expected_amount_coverage: TxnAmountCoverageV1,
    ) -> Optional[dict[str, Any]]:
        if not self._can_use_materialized_graph_source(min_amount=min_amount, request_context=request_context):
            return None
        context_obj = _clone_json(request_context if isinstance(request_context, dict) else {}, {})
        context_obj["__use_materialized_source"] = True
        runtime_graph = self._build_runtime_graph(
            case_id=case_id,
            seeds=seeds,
            depth=depth,
            direction=direction,
            min_amount=min_amount,
            request_context=context_obj,
            expected_amount_coverage=expected_amount_coverage,
        )
        full_result = self._build_graph_result_from_runtime_graph(runtime_graph)
        direct_mode = _resolve_projection_render_mode(request_context, full_result)
        projected_result = None
        if direct_mode == "skeleton":
            projected_result = _build_projected_result(
                full_result=full_result,
                source_snapshot_ref=None,
                request_context=request_context or {},
            )
            if projected_result is not None:
                projection = _normalize_projection_payload(projected_result.get("projection"))
                projection["source_query"] = self._build_projection_source_query(
                    case_id=case_id,
                    seeds=seeds,
                    depth=depth,
                    direction=direction,
                    min_amount=min_amount,
                    request_context=context_obj,
                    prefer_materialized_source=True,
                )
                projected_result["projection"] = projection
                projected_result = _attach_graph_render_metadata(projected_result)
        return {
            "full_result": full_result,
            "projected_result": projected_result,
        }

    def _materialize_projection_source_result(
        self,
        *,
        case_id: str,
        base_snapshot_ref: Optional[dict[str, Any]],
        base_projection: Optional[dict[str, Any]],
    ) -> tuple[Optional[dict[str, Any]], dict[str, Any]]:
        projection = _normalize_projection_payload(base_projection)
        source_snapshot_ref = _normalize_snapshot_ref(projection.get("source_result_snapshot_ref"))
        if source_snapshot_ref:
            full_result = self._load_result_snapshot(case_id, source_snapshot_ref)
            if _graph_has_payload(full_result.get("runtime_graph")):
                return source_snapshot_ref, full_result

        source_query = projection.get("source_query") if isinstance(projection.get("source_query"), dict) else {}
        if source_query:
            query_case_id = str(source_query.get("case_id") or case_id or "").strip() or case_id
            query_seeds = [str(item or "").strip() for item in source_query.get("seeds") or [] if str(item or "").strip()]
            query_context = _clone_json(source_query.get("request_context"), {})
            graph_ctx = query_context.get("graph") if isinstance(query_context.get("graph"), dict) else {}
            graph_ctx = {**_clone_json(graph_ctx, {}), "render_mode": "full"}
            query_context["graph"] = graph_ctx
            result = self.build_graph(
                case_id=query_case_id,
                seeds=query_seeds,
                depth=max(1, int(source_query.get("depth") or 1)),
                direction=str(source_query.get("direction") or "both"),
                min_amount=_required_nonnegative_min_amount(source_query.get("min_amount")),
                request_context=query_context,
            )
            result_ref = _normalize_snapshot_ref(result.get("result_snapshot_ref")) or self._persist_result_snapshot(query_case_id, result)
            return result_ref, result

        fallback_ref = _normalize_snapshot_ref(base_snapshot_ref)
        fallback_result = self._load_result_snapshot(case_id, fallback_ref)
        return fallback_ref, fallback_result

    def build_graph(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict] = None,
        case_lifecycle_binding: Optional[FlowCaseLifecycleBindingV1] = None,
    ) -> dict:
        normalized_min_amount = _required_nonnegative_min_amount(min_amount)
        binding = (
            self.freeze_case_lifecycle_binding(case_id)
            if case_lifecycle_binding is None
            else self.require_case_lifecycle_binding_current(
                case_lifecycle_binding,
                case_id=case_id,
            )
        )
        amount_coverage = self._daily_agg.require_complete_amount_coverage(case_id=case_id)
        requested_projection = _normalize_projection_request(request_context)
        materialized_bundle = None
        if str(requested_projection.get("render_mode") or "").strip().lower() in {"auto", "skeleton"}:
            materialized_bundle = self._build_materialized_graph_result_bundle(
                case_id=case_id,
                seeds=seeds,
                depth=depth,
                direction=direction,
                min_amount=normalized_min_amount,
                request_context=request_context,
                expected_amount_coverage=amount_coverage,
            )
        if materialized_bundle is not None:
            full_result = materialized_bundle.get("full_result") if isinstance(materialized_bundle.get("full_result"), dict) else {}
            projected_result = (
                materialized_bundle.get("projected_result") if isinstance(materialized_bundle.get("projected_result"), dict) else None
            )
            _attach_candidate_amount_coverage(full_result, amount_coverage)
            if projected_result is not None:
                _attach_candidate_amount_coverage(projected_result, amount_coverage)
            result = projected_result or full_result
            if projected_result is not None:
                projected_snapshot_ref = self._persist_result_snapshot(
                    case_id=case_id,
                    result=projected_result,
                    base_snapshot_ref=_normalize_snapshot_ref(
                        _normalize_projection_payload(projected_result.get("projection")).get("source_result_snapshot_ref")
                    ),
                    case_lifecycle_binding=binding,
                )
                if projected_snapshot_ref:
                    result["result_snapshot_ref"] = projected_snapshot_ref
            self.require_case_lifecycle_binding_current(binding, case_id=case_id)
            return _attach_graph_render_metadata(result)

        runtime_graph = self._build_runtime_graph(
            case_id=case_id,
            seeds=seeds,
            depth=depth,
            direction=direction,
            min_amount=normalized_min_amount,
            request_context=request_context,
            expected_amount_coverage=amount_coverage,
        )
        result = _attach_candidate_amount_coverage(
            self._build_graph_result_from_runtime_graph(runtime_graph),
            amount_coverage,
        )
        if _resolve_projection_render_mode(request_context, result) == "skeleton":
            full_snapshot_ref = self._persist_result_snapshot(
                case_id=case_id,
                result=result,
                case_lifecycle_binding=binding,
            )
            projected_result = _build_projected_result(
                full_result=result,
                source_snapshot_ref=full_snapshot_ref,
                request_context=request_context or {},
            )
            if projected_result is not None:
                result = _attach_candidate_amount_coverage(projected_result, amount_coverage)
                if full_snapshot_ref:
                    projection = _normalize_projection_payload(result.get("projection"))
                    projection["source_result_snapshot_ref"] = full_snapshot_ref
                    result["projection"] = projection
                    projected_snapshot_ref = self._persist_result_snapshot(
                        case_id=case_id,
                        result=result,
                        base_snapshot_ref=full_snapshot_ref,
                        case_lifecycle_binding=binding,
                    )
                    if projected_snapshot_ref:
                        result["result_snapshot_ref"] = projected_snapshot_ref
        self.require_case_lifecycle_binding_current(binding, case_id=case_id)
        return result

    def _build_stats_focus_account_fast_graph_native(
        self,
        *,
        case_id: str,
        depth: int,
        direction: str,
        min_amount: float,
        view_mode: str,
        seed_ids: Sequence[str],
        left_seed_ids: Sequence[str],
        focus_id_raw: str,
        focus_label: str,
        focus_ids: Sequence[str],
        focus_placeholder_kinds: Sequence[str],
        request_id: str,
        source: str,
        focus_only: bool,
        focus_key_type: str,
        payload_focus_account_strict: bool,
        payload_include_missing: bool,
        expected_total_amount: Optional[float],
        expected_row_count: Optional[int],
        date_start: Optional[datetime],
        date_end_excl: Optional[datetime],
    ) -> Optional[dict]:
        if source != "stats" or not focus_only or focus_key_type != "account":
            return None
        if max(1, int(depth or 1)) != 1:
            return None
        if not payload_focus_account_strict:
            return None

        query_seed_ids = []
        seen_query_seed: set[str] = set()
        for raw_seed in left_seed_ids or seed_ids:
            seed_key = str(raw_seed or "").strip()
            if (
                not seed_key
                or seed_key in seen_query_seed
                or seed_key.startswith("__unknown_cp__name::")
                or _is_placeholder_node_id(seed_key)
                or _is_missing_account_key(seed_key)
                or seed_key == "0"
            ):
                continue
            seen_query_seed.add(seed_key)
            query_seed_ids.append(seed_key)
        if not query_seed_ids:
            return None

        selected_focus_ids: list[str] = []
        seen_focus_ids: set[str] = set()
        for raw_focus in focus_ids:
            focus_key = str(raw_focus or "").strip()
            if (
                not focus_key
                or focus_key in seen_focus_ids
                or _is_placeholder_node_id(focus_key)
                or _is_missing_account_key(focus_key)
            ):
                continue
            seen_focus_ids.add(focus_key)
            selected_focus_ids.append(focus_key)

        selected_placeholder_kinds = normalize_placeholder_kinds(focus_placeholder_kinds)
        include_missing_counterparty = bool(payload_include_missing or selected_placeholder_kinds)
        if not selected_focus_ids and not selected_placeholder_kinds:
            return None

        if not self._daily_agg.ensure_materialized(case_id, engine=None):
            return None
        with tempfile.TemporaryDirectory(prefix="analytix-flow-focus-graph-") as temp_dir:
            output_json = Path(temp_dir) / "graph.json"
            try:
                query_flow_focus_graph_to_file(
                    case_id=case_id,
                    db_path=self._storage.case_db(case_id),
                    query_seed_ids=query_seed_ids,
                    seed_ids=seed_ids,
                    selected_focus_ids=selected_focus_ids,
                    selected_placeholder_kinds=selected_placeholder_kinds,
                    include_missing_counterparty=include_missing_counterparty,
                    direction=direction,
                    min_amount=min_amount,
                    date_start=date_start.strftime("%Y-%m-%d") if date_start is not None else "",
                    date_end_excl=date_end_excl.strftime("%Y-%m-%d") if date_end_excl is not None else "",
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
            except TxnAmountCoverageIncompleteError:
                raise
            except Exception:
                return None
            native_result = json.loads(output_json.read_text(encoding="utf-8"))
        graph = native_result.get("graph") if isinstance(native_result, dict) else None
        if not isinstance(graph, dict):
            raise RuntimeError("Rust analysis compute query-flow-focus-graph returned no graph")
        return graph

    def _build_stats_focus_account_fast_graph(
        self,
        *,
        con,
        case_id: str,
        depth: int,
        direction: str,
        min_amount: float,
        view_mode: str,
        seed_ids: Sequence[str],
        left_seed_ids: Sequence[str],
        focus_id_raw: str,
        focus_label: str,
        focus_ids: Sequence[str],
        focus_placeholder_kinds: Sequence[str],
        request_id: str,
        source: str,
        focus_only: bool,
        focus_key_type: str,
        payload_focus_account_strict: bool,
        payload_include_missing: bool,
        expected_total_amount: Optional[float],
        expected_row_count: Optional[int],
        date_start: Optional[datetime],
        date_end_excl: Optional[datetime],
        cols: set[str],
    ) -> Optional[dict]:
        if source != "stats" or not focus_only or focus_key_type != "account":
            return None
        if max(1, int(depth or 1)) != 1:
            return None
        if not payload_focus_account_strict:
            return None

        query_seed_ids = []
        seen_query_seed: set[str] = set()
        for raw_seed in left_seed_ids or seed_ids:
            seed_key = str(raw_seed or "").strip()
            if (
                not seed_key
                or seed_key in seen_query_seed
                or seed_key.startswith("__unknown_cp__name::")
                or _is_placeholder_node_id(seed_key)
                or _is_missing_account_key(seed_key)
                or seed_key == "0"
            ):
                continue
            seen_query_seed.add(seed_key)
            query_seed_ids.append(seed_key)
        if not query_seed_ids:
            return None

        selected_focus_ids: list[str] = []
        seen_focus_ids: set[str] = set()
        for raw_focus in focus_ids:
            focus_key = str(raw_focus or "").strip()
            if (
                not focus_key
                or focus_key in seen_focus_ids
                or _is_placeholder_node_id(focus_key)
                or _is_missing_account_key(focus_key)
            ):
                continue
            seen_focus_ids.add(focus_key)
            selected_focus_ids.append(focus_key)

        selected_placeholder_kinds = normalize_placeholder_kinds(focus_placeholder_kinds)
        include_missing_counterparty = bool(payload_include_missing or selected_placeholder_kinds)
        if not selected_focus_ids and not selected_placeholder_kinds:
            return None
        if not self._table_exists(con, "analysis_txn_detail_idx"):
            return None

        rows = None
        acct_key_expr = _account_key_expr("t", cols, candidates=_ACCOUNT_KEY_CANDIDATES)
        cp_key_expr = _counterparty_key_expr("t", cols)
        cp_raw_expr = _counterparty_raw_expr("t", cols)
        cp_placeholder_kind_expr = placeholder_kind_sql(cp_raw_expr) if cp_raw_expr != "NULL" else "NULL"
        dc_expr = _dc_value_expr("t", cols)
        amt_expr = _amount_value_expr("t", cols)
        ts_expr = _txn_ts_expr("t", cols)
        open_name_expr = _text_col_expr("t", cols, "account_open_name", clean_invalid=True) or "NULL"
        cp_name_expr = _text_col_expr("t", cols, "counterparty_name", clean_invalid=True) or "NULL"
        if rows is None and (acct_key_expr == "NULL" or dc_expr == "NULL" or amt_expr == "NULL"):
            return None

        base_sql = (
            "WITH base AS ("
            "  SELECT "
            f"    {acct_key_expr} AS acct_key, "
            f"    {cp_key_expr} AS cp_key, "
            f"    {cp_raw_expr} AS cp_key_raw, "
            f"    {cp_placeholder_kind_expr} AS cp_placeholder_kind, "
            f"    {dc_expr} AS dc_val, "
            f"    ABS({amt_expr}) AS amt_abs, "
            f"    {ts_expr} AS txn_ts, "
            f"    {open_name_expr} AS open_name, "
            f"    {cp_name_expr} AS cp_name "
            "  FROM analysis_txn_detail_idx t "
            "  WHERE amount_source_present=1 "
            "    AND amount_parse_failed=0 "
            "    AND amount IS NOT NULL "
            "    AND dc_val IN ('进','出')"
            ") "
        )

        cp_name_select_expr = "MAX(cp_name)"
        from_clause = "FROM base"
        if cp_name_expr != "NULL":
            base_sql += (
                ", name_pick AS ("
                "  SELECT cp_key AS pick_key, cp_name AS pick_name "
                "  FROM ("
                "    SELECT cp_key, cp_name, "
                "      COUNT(*) AS name_cnt, "
                "      MAX(txn_ts) AS name_last_ts, "
                "      ROW_NUMBER() OVER ("
                "        PARTITION BY cp_key "
                "        ORDER BY name_cnt DESC, name_last_ts DESC, cp_name DESC"
                "      ) AS rn "
                "    FROM base "
                "    WHERE cp_name IS NOT NULL AND TRIM(cp_name) <> '' "
                "    GROUP BY cp_key, cp_name"
                "  ) t "
                "  WHERE rn=1"
                ") "
            )
            from_clause = "FROM base LEFT JOIN name_pick ON name_pick.pick_key = base.cp_key"
            cp_name_select_expr = "COALESCE(MAX(name_pick.pick_name), MAX(cp_name))"

        unknown_name_label = "未知户名"
        unknown_card_label = "未知账号"
        unknown_prefix = "__unknown_cp__name::"
        placeholder_prefix = PLACEHOLDER_TOKEN_PREFIX
        unknown_cp_id_empty = f"{unknown_prefix}__empty__"
        if include_missing_counterparty:
            cp_key_norm_expr = (
                "CASE "
                "WHEN cp_placeholder_kind IS NOT NULL AND cp_name IS NOT NULL THEN "
                f"'{placeholder_prefix}' || cp_placeholder_kind || '::name::' || cp_name "
                "WHEN cp_placeholder_kind IS NOT NULL THEN "
                f"'{placeholder_prefix}' || cp_placeholder_kind "
                "WHEN cp_key IS NOT NULL THEN cp_key "
                "WHEN cp_name IS NOT NULL THEN "
                f"'{unknown_prefix}' || cp_name "
                "ELSE "
                f"'{unknown_cp_id_empty}' END"
            )
        else:
            cp_key_norm_expr = "cp_key"

        filters = []
        params: list[Any] = []
        seed_placeholders = ",".join(["?"] * len(query_seed_ids))
        filters.append(f"acct_key IN ({seed_placeholders})")
        params.extend(query_seed_ids)
        filters.append("acct_key IS NOT NULL")
        filters.append("dc_val IN ('进','出')")

        focus_match_parts: list[str] = []
        if selected_focus_ids:
            focus_placeholders = ",".join(["?"] * len(selected_focus_ids))
            focus_match_parts.append(f"(cp_placeholder_kind IS NULL AND cp_key IN ({focus_placeholders}))")
            params.extend(selected_focus_ids)
        if selected_placeholder_kinds:
            kind_placeholders = ",".join(["?"] * len(selected_placeholder_kinds))
            focus_match_parts.append(f"(cp_placeholder_kind IN ({kind_placeholders}))")
            params.extend(selected_placeholder_kinds)
        if not focus_match_parts:
            return None
        filters.append("(" + " OR ".join(focus_match_parts) + ")")

        dir_mode = direction if direction in {"in", "out"} else "all"
        if dir_mode == "out":
            filters.append("dc_val='出'")
        elif dir_mode == "in":
            filters.append("dc_val='进'")

        if date_start is not None:
            filters.append("txn_ts >= ?")
            params.append(date_start)
        if date_end_excl is not None:
            filters.append("txn_ts < ?")
            params.append(date_end_excl)
        if min_amount > 0:
            filters.append("amt_abs >= ?")
            params.append(float(min_amount))

        sql = (
            base_sql
            + "SELECT "
            f"  acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw AS cp_key_raw, dc_val, "
            "  COUNT(1) AS txn_count, "
            "  SUM(amt_abs) AS amt_sum, "
            "  MIN(txn_ts) AS first_ts, "
            "  MAX(txn_ts) AS last_ts, "
            "  MAX(open_name) AS open_name, "
            f"  {cp_name_select_expr} AS cp_name "
            f"{from_clause} "
            f"WHERE {' AND '.join(filters)} "
            f"GROUP BY acct_key, {cp_key_norm_expr}, cp_key, cp_key_raw, dc_val "
            "ORDER BY amt_sum DESC"
        )

        @dataclass
        class _EdgeAggregate:
            a: str
            b: str
            amount: float = 0.0
            count: int = 0
            out_amount: float = 0.0
            in_amount: float = 0.0
            first_ts: Optional[object] = None
            last_ts: Optional[object] = None

        def _norm_focus_key(value: str) -> str:
            text = str(value or "").strip()
            if not text:
                return ""
            text = "".join(text.split())
            idx_dash = text.find("-")
            idx_under = text.find("_")
            if idx_dash > 0 or idx_under > 0:
                if idx_dash <= 0:
                    idx = idx_under
                elif idx_under <= 0:
                    idx = idx_dash
                else:
                    idx = min(idx_dash, idx_under)
                text = text[:idx]
            return text

        def _merge_first_ts(ts1: Optional[object], ts2: Optional[object]) -> Optional[object]:
            if ts1 is None:
                return ts2
            if ts2 is None:
                return ts1
            return ts1 if ts1 <= ts2 else ts2

        def _merge_last_ts(ts1: Optional[object], ts2: Optional[object]) -> Optional[object]:
            if ts1 is None:
                return ts2
            if ts2 is None:
                return ts1
            return ts1 if ts1 >= ts2 else ts2

        def _ts_within_mirror_tolerance(ts1: Optional[object], ts2: Optional[object]) -> bool:
            if ts1 is None or ts2 is None:
                return False
            try:
                return abs((ts1 - ts2).total_seconds()) <= _STATS_TXN_MIRROR_TOLERANCE_SECONDS
            except Exception:
                return False

        def _aggregate_detail_rows_with_mirror_dedupe(detail_rows: Sequence[Sequence[Any]]) -> list[tuple[Any, ...]]:
            aggregate_rows: dict[tuple[str, str, str, str], dict[str, Any]] = {}
            pending_mirrors: dict[tuple[str, str, float], dict[str, list[dict[str, Any]]]] = defaultdict(
                lambda: {"src": [], "tgt": []}
            )
            query_seed_id_set = set(query_seed_ids)
            selected_focus_id_set = set(selected_focus_ids)
            selected_placeholder_kind_set = set(selected_placeholder_kinds)

            def add_row(item: dict[str, Any], amount: float, count: int = 1) -> None:
                row_key = (
                    str(item.get("acct_key") or "").strip(),
                    str(item.get("cp_key") or "").strip(),
                    str(item.get("cp_key_raw") or "").strip(),
                    str(item.get("dc_val") or "").strip(),
                )
                current = aggregate_rows.get(row_key)
                if current is None:
                    current = {
                        "acct_key": row_key[0],
                        "cp_key": row_key[1],
                        "cp_key_raw": row_key[2],
                        "dc_val": row_key[3],
                        "txn_count": 0,
                        "amt_sum": 0.0,
                        "first_ts": None,
                        "last_ts": None,
                        "open_name": str(item.get("open_name") or "").strip(),
                        "cp_name": str(item.get("cp_name") or "").strip(),
                    }
                    aggregate_rows[row_key] = current
                current["txn_count"] = int(current.get("txn_count") or 0) + int(count or 0)
                current["amt_sum"] = float(current.get("amt_sum", 0.0)) + float(amount)
                current["first_ts"] = _merge_first_ts(current.get("first_ts"), item.get("txn_ts"))
                current["last_ts"] = _merge_last_ts(current.get("last_ts"), item.get("txn_ts"))
                if not str(current.get("open_name") or "").strip() and str(item.get("open_name") or "").strip():
                    current["open_name"] = str(item.get("open_name") or "").strip()
                if not str(current.get("cp_name") or "").strip() and str(item.get("cp_name") or "").strip():
                    current["cp_name"] = str(item.get("cp_name") or "").strip()

            for detail_row in detail_rows:
                if not detail_row:
                    continue
                acct_key, cp_key, cp_key_raw, dc_val, amt_abs, txn_ts, open_name, cp_name, cp_placeholder_kind, raw_cp_key = detail_row
                if amt_abs is None:
                    raise TxnAmountCoverageIncompleteError()
                dc_val = str(dc_val or "").strip()
                if dc_val not in {"进", "出"}:
                    continue
                cp_placeholder_kind_text = str(cp_placeholder_kind or "").strip()
                raw_cp_key_text = str(raw_cp_key or "").strip()
                item = {
                    "acct_key": str(acct_key or "").strip(),
                    "cp_key": str(cp_key or "").strip(),
                    "cp_key_raw": str(cp_key_raw or "").strip(),
                    "dc_val": dc_val,
                    "amt_abs": abs(float(amt_abs)),
                    "txn_ts": txn_ts,
                    "open_name": str(open_name or "").strip(),
                    "cp_name": str(cp_name or "").strip(),
                    "primary": (
                        str(acct_key or "").strip() in query_seed_id_set
                        and (
                            (not cp_placeholder_kind_text and raw_cp_key_text in selected_focus_id_set)
                            or (cp_placeholder_kind_text in selected_placeholder_kind_set)
                        )
                    ),
                }
                if not item["acct_key"] or not item["cp_key"]:
                    continue
                if dc_val == "出":
                    flow_src, flow_tgt, mirror_side = item["acct_key"], item["cp_key"], "src"
                else:
                    flow_src, flow_tgt, mirror_side = item["cp_key"], item["acct_key"], "tgt"
                amount = float(item["amt_abs"])
                if flow_src != flow_tgt:
                    mirror_key = (flow_src, flow_tgt, _round2(amount))
                    opposite_side = "tgt" if mirror_side == "src" else "src"
                    opposite_rows = pending_mirrors[mirror_key][opposite_side]
                    matched_index = None
                    for index, candidate in enumerate(opposite_rows):
                        if _ts_within_mirror_tolerance(candidate.get("txn_ts"), txn_ts):
                            matched_index = index
                            break
                    if matched_index is not None:
                        matched_row = opposite_rows.pop(matched_index)
                        if not item.get("primary") and not matched_row.get("primary"):
                            continue
                        representative = item if item.get("primary") else matched_row if matched_row.get("primary") else item
                        representative = dict(representative)
                        representative["txn_ts"] = _merge_first_ts(matched_row.get("txn_ts"), txn_ts)
                        add_row(representative, amount, 1)
                        continue
                    pending_mirrors[mirror_key][mirror_side].append(item)
                    continue
                add_row(item, amount, 1)

            for mirror_state in pending_mirrors.values():
                for side in ("src", "tgt"):
                    for pending_row in mirror_state[side]:
                        if not pending_row.get("primary"):
                            continue
                        pending_amount = pending_row.get("amt_abs")
                        if pending_amount is None:
                            raise TxnAmountCoverageIncompleteError()
                        add_row(pending_row, float(pending_amount), 1)

            rows_out = [
                (
                    item.get("acct_key"),
                    item.get("cp_key"),
                    item.get("cp_key_raw"),
                    item.get("dc_val"),
                    int(item.get("txn_count") or 0),
                    float(item.get("amt_sum", 0.0)),
                    item.get("first_ts"),
                    item.get("last_ts"),
                    item.get("open_name"),
                    item.get("cp_name"),
                )
                for item in aggregate_rows.values()
            ]
            rows_out.sort(key=lambda item: float(item[5]), reverse=True)
            return rows_out

        if rows is None and self._table_exists(con, "analysis_txn_detail_idx"):
            detail_filters = []
            detail_params: list[Any] = []
            detail_acct_ids = []
            seen_detail_acct_ids = set()
            for raw_acct in [*query_seed_ids, *selected_focus_ids]:
                acct_text = str(raw_acct or "").strip()
                if not acct_text or acct_text in seen_detail_acct_ids:
                    continue
                seen_detail_acct_ids.add(acct_text)
                detail_acct_ids.append(acct_text)
            detail_acct_placeholders = ",".join(["?"] * len(detail_acct_ids))
            detail_filters.append(f"acct_key IN ({detail_acct_placeholders})")
            detail_params.extend(detail_acct_ids)
            detail_filters.append("acct_key IS NOT NULL")
            detail_filters.append("dc_val IN ('进','出')")
            detail_primary_parts: list[str] = []
            detail_primary_params: list[Any] = []
            if selected_focus_ids:
                focus_placeholders_detail = ",".join(["?"] * len(selected_focus_ids))
                detail_primary_parts.append(f"(cp_placeholder_kind IS NULL AND cp_key IN ({focus_placeholders_detail}))")
                detail_primary_params.extend(selected_focus_ids)
            if selected_placeholder_kinds:
                kind_placeholders_detail = ",".join(["?"] * len(selected_placeholder_kinds))
                detail_primary_parts.append(f"(cp_placeholder_kind IN ({kind_placeholders_detail}))")
                detail_primary_params.extend(selected_placeholder_kinds)
            mirror_focus_parts = []
            if detail_primary_parts:
                seed_primary_placeholders = ",".join(["?"] * len(query_seed_ids))
                mirror_focus_parts.append(
                    f"(acct_key IN ({seed_primary_placeholders}) AND ({' OR '.join(detail_primary_parts)}))"
                )
                detail_params.extend(query_seed_ids)
                detail_params.extend(detail_primary_params)
            if query_seed_ids:
                seed_cp_placeholders_detail = ",".join(["?"] * len(query_seed_ids))
                mirror_focus_parts.append(f"(cp_placeholder_kind IS NULL AND cp_key IN ({seed_cp_placeholders_detail}))")
                detail_params.extend(query_seed_ids)
            if not mirror_focus_parts:
                return None
            detail_filters.append("(" + " OR ".join(mirror_focus_parts) + ")")
            if dir_mode == "out":
                detail_filters.append("dc_val='出'")
            elif dir_mode == "in":
                detail_filters.append("dc_val='进'")
            if date_start is not None:
                detail_filters.append("txn_ts >= ?")
                detail_params.append(date_start)
            if date_end_excl is not None:
                detail_filters.append("txn_ts < ?")
                detail_params.append(date_end_excl)
            if min_amount > 0:
                detail_filters.append("ABS(amount) >= ?")
                detail_params.append(float(min_amount))
            detail_cp_key_expr = (
                "CASE "
                "WHEN cp_placeholder_kind IS NOT NULL AND counterparty_name IS NOT NULL AND TRIM(counterparty_name) <> '' THEN "
                f"'{placeholder_prefix}' || cp_placeholder_kind || '::name::' || counterparty_name "
                "WHEN cp_placeholder_kind IS NOT NULL THEN "
                f"'{placeholder_prefix}' || cp_placeholder_kind "
                "WHEN cp_key IS NOT NULL THEN cp_key "
                "WHEN counterparty_name IS NOT NULL AND TRIM(counterparty_name) <> '' THEN "
                f"'{unknown_prefix}' || counterparty_name "
                "ELSE "
                f"'{unknown_cp_id_empty}' END"
                if include_missing_counterparty
                else "cp_key"
            )
            detail_sql = (
                "SELECT "
                f"  acct_key, {detail_cp_key_expr} AS cp_key, cp_raw AS cp_key_raw, dc_val, "
                "  ABS(amount) AS amt_abs, txn_ts, "
                "  account_open_name AS open_name, COALESCE(cp_name_pick, counterparty_name) AS cp_name, "
                "  cp_placeholder_kind, cp_key AS raw_cp_key "
                "FROM analysis_txn_detail_idx "
                f"WHERE {' AND '.join(detail_filters)} "
                "ORDER BY txn_ts ASC, amt_abs DESC, acct_key ASC, cp_key ASC, dc_val ASC"
            )
            try:
                detail_rows = con.query(detail_sql, tuple(detail_params))
                normalized_detail_rows = []
                for detail_row in detail_rows:
                    normalized_detail_rows.append(
                        (
                            detail_row[0],
                            detail_row[1],
                            detail_row[2],
                            detail_row[3],
                            detail_row[4],
                            detail_row[5],
                            detail_row[6],
                            detail_row[7],
                            detail_row[8],
                            detail_row[9],
                        )
                    )
                rows = _aggregate_detail_rows_with_mirror_dedupe(normalized_detail_rows)
            except TxnAmountCoverageIncompleteError:
                raise
            except Exception:
                return None
        if rows is None:
            return None

        edges: dict[tuple[str, str], _EdgeAggregate] = {}
        node_amount: dict[str, float] = {}
        node_count: dict[str, int] = {}
        node_name: dict[str, str] = {}
        node_name_source: dict[str, str] = {}
        node_display: dict[str, str] = {}
        cp_display_ids: dict[str, set[str]] = {}
        node_display_list: dict[str, list[str]] = {}
        acct_seen: set[str] = set()
        cp_name_missing: set[str] = set()
        seen_seed = set(seed_ids)

        def _apply_flow_agg(
            *,
            flow_src: str,
            flow_tgt: str,
            amount: float,
            count: int,
            first_ts: Optional[object],
            last_ts: Optional[object],
        ) -> None:
            if not flow_src or not flow_tgt:
                return
            a, b = sorted([flow_src, flow_tgt])
            edge_key = (a, b)
            agg = edges.get(edge_key)
            if agg is None:
                agg = _EdgeAggregate(a=a, b=b)
                edges[edge_key] = agg

            agg.amount += amount
            agg.count += count
            if flow_src == a:
                agg.out_amount += amount
            else:
                agg.in_amount += amount
            agg.first_ts = _merge_first_ts(agg.first_ts, first_ts)
            agg.last_ts = _merge_last_ts(agg.last_ts, last_ts)

            if flow_src == flow_tgt:
                node_amount[flow_src] = node_amount.get(flow_src, 0.0) + amount
                node_count[flow_src] = node_count.get(flow_src, 0) + count
                return

            for node_id in (flow_src, flow_tgt):
                node_amount[node_id] = node_amount.get(node_id, 0.0) + amount
                node_count[node_id] = node_count.get(node_id, 0) + count

        def _apply_row_context(
            acct_value: Any,
            cp_value: Any,
            cp_raw_value: Any,
            cp_name_value: Any,
            open_name_value: Any,
        ) -> Optional[tuple[str, str]]:
            acct_key = str(acct_value or "").strip()
            cp_key = str(cp_value or "").strip()
            cp_key_raw = str(cp_raw_value or "").strip()
            cp_name = str(cp_name_value or "").strip()
            open_name = str(open_name_value or "").strip()
            if not acct_key or not cp_key:
                return None

            is_placeholder_cp = _is_placeholder_node_id(cp_key)
            acct_seen.add(acct_key)
            if is_placeholder_cp:
                placeholder_label = placeholder_kind_label(_placeholder_kind_from_node_id(cp_key))
                if placeholder_label:
                    node_display[cp_key] = placeholder_label
            elif cp_key_raw and cp_key.startswith(unknown_prefix) and not _is_missing_account_key(cp_key_raw):
                cp_display_ids.setdefault(cp_key, set()).add(cp_key_raw)

            if open_name and node_name_source.get(acct_key) != "open":
                node_name[acct_key] = open_name
                node_name_source[acct_key] = "open"

            if not cp_name and not cp_key.startswith(unknown_prefix) and not is_placeholder_cp:
                cp_name_missing.add(cp_key)
                if node_name_source.get(cp_key) == "cp":
                    node_name[cp_key] = unknown_name_label
                    node_name_source[cp_key] = "unknown"

            if cp_name:
                if (is_placeholder_cp or cp_key not in cp_name_missing) and node_name_source.get(cp_key) != "open":
                    if node_name.get(cp_key) in {None, "", unknown_name_label}:
                        node_name[cp_key] = cp_name
                        node_name_source[cp_key] = "cp"

            return acct_key, cp_key

        for row in rows:
            if not row:
                continue
            acct_key, cp_key, cp_key_raw, dc_val, txn_count, amt_sum, first_ts, last_ts, open_name, cp_name = row
            dc_val = str(dc_val or "").strip()
            if dc_val not in {"进", "出"}:
                continue
            row_ctx = _apply_row_context(
                acct_key,
                cp_key,
                cp_key_raw,
                cp_name,
                open_name,
            )
            if row_ctx is None:
                continue
            acct_key_norm, cp_key_norm = row_ctx
            if dc_val == "出":
                flow_src, flow_tgt = acct_key_norm, cp_key_norm
            else:
                flow_src, flow_tgt = cp_key_norm, acct_key_norm
            if amt_sum is None:
                raise TxnAmountCoverageIncompleteError()
            _apply_flow_agg(
                flow_src=flow_src,
                flow_tgt=flow_tgt,
                amount=float(amt_sum),
                count=_required_transaction_count(txn_count),
                first_ts=first_ts,
                last_ts=last_ts,
            )

        for node_id, ids in cp_display_ids.items():
            if not ids:
                continue
            show_ids = sorted(ids)
            node_display_list[node_id] = show_ids
            node_display[node_id] = " / ".join(show_ids[:6]) + (f" 等{len(show_ids)}个" if len(show_ids) > 6 else "")

        edge_list = sorted(edges.values(), key=lambda edge: edge.amount, reverse=True)
        self_loop_edges = sum(1 for edge in edge_list if edge.a == edge.b)
        node_ids: set[str] = set()
        for edge in edge_list:
            node_ids.add(edge.a)
            node_ids.add(edge.b)
        if focus_only and not node_ids and focus_id_raw:
            node_ids.add(focus_id_raw)
        if include_missing_counterparty and node_ids:
            for node_id in node_ids:
                if node_id.startswith(unknown_prefix) and node_id not in node_display:
                    node_display[node_id] = unknown_card_label
                if _is_placeholder_node_id(node_id) and node_id not in node_display:
                    node_display[node_id] = placeholder_kind_label(_placeholder_kind_from_node_id(node_id)) or node_id
        self._apply_account_display_metadata(
            con=con,
            case_id=case_id,
            account_ids=node_ids.intersection(acct_seen),
            node_display=node_display,
            node_display_list=node_display_list,
            node_name=node_name,
            node_name_source=node_name_source,
        )

        nodes_out = []
        for node_id in sorted(node_ids):
            name = str(node_name.get(node_id) or "").strip()
            if not name and focus_label and focus_id_raw:
                try:
                    if _norm_focus_key(node_id) == _norm_focus_key(focus_id_raw):
                        name = focus_label
                except Exception:
                    pass
            display_id = node_display.get(node_id) or node_id
            ntype = "seed" if node_id in seen_seed else ("account" if node_id in acct_seen else "node")
            if _is_placeholder_node_id(node_id):
                title = f"{display_id} | {name}" if name else display_id
            elif ntype in {"seed", "account"}:
                title = f"{display_id} | {name}" if name and name != display_id else display_id
            else:
                title = name if name else node_id
            display_ids = node_display_list.get(node_id)
            nodes_out.append(
                {
                    "id": node_id,
                    "title": title,
                    "name": name,
                    "display_id": display_id,
                    "display_ids": display_ids,
                    "ntype": ntype,
                    "total_amount": _project_node_total_amount(node_amount, node_id),
                    "total_count": int(node_count.get(node_id, 0)),
                }
            )

        edges_out = []
        for edge in edge_list:
            if view_mode == "net":
                net = edge.out_amount - edge.in_amount
                if abs(net) < 1e-9:
                    continue
                if net > 0:
                    source_id, target_id = edge.a, edge.b
                else:
                    source_id, target_id = edge.b, edge.a
                amount = abs(net)
                edges_out.append(
                    {
                        "id": f"{edge.a}=={edge.b}",
                        "source": source_id,
                        "target": target_id,
                        "amount": _round2(amount),
                        "count": int(edge.count),
                        "out_amount": _round2(edge.out_amount),
                        "in_amount": _round2(edge.in_amount),
                        "forward_amount": _round2(amount),
                        "reverse_amount": 0.0,
                        "mode": "single",
                        "label": f"￥{amount:.2f}",
                        "first_time": _dt_to_str(edge.first_ts),
                        "last_time": _dt_to_str(edge.last_ts),
                    }
                )
                continue

            if edge.out_amount > 0 and edge.in_amount == 0:
                source_id, target_id = edge.a, edge.b
            elif edge.in_amount > 0 and edge.out_amount == 0:
                source_id, target_id = edge.b, edge.a
            elif edge.out_amount >= edge.in_amount:
                source_id, target_id = edge.a, edge.b
            else:
                source_id, target_id = edge.b, edge.a

            if source_id == edge.a and target_id == edge.b:
                forward_amount = edge.out_amount
                reverse_amount = edge.in_amount
            else:
                forward_amount = edge.in_amount
                reverse_amount = edge.out_amount

            edges_out.append(
                {
                    "id": f"{edge.a}=={edge.b}",
                    "source": source_id,
                    "target": target_id,
                    "amount": _round2(edge.amount),
                    "count": int(edge.count),
                    "out_amount": _round2(edge.out_amount),
                    "in_amount": _round2(edge.in_amount),
                    "forward_amount": _round2(forward_amount),
                    "reverse_amount": _round2(reverse_amount),
                    "mode": "double" if edge.out_amount > 0 and edge.in_amount > 0 else "single",
                    "first_time": _dt_to_str(edge.first_ts),
                    "last_time": _dt_to_str(edge.last_ts),
                }
            )

        total_amount = _round2(sum(_required_amount_float(item.get("amount")) for item in edges_out))

        stats = {
            "requested_depth": int(depth),
            "direction": direction,
            "seed_count": len(seed_ids),
            "node_count": len(nodes_out),
            "edge_count": len(edges_out),
            "self_loop_edge_count": self_loop_edges,
            "total_amount": total_amount,
            "view_mode": view_mode,
            "context_applied": {
                "source": source,
                "request_id": request_id,
                "date_start": date_start.strftime("%Y-%m-%d") if date_start else "",
                "date_end": (date_end_excl - timedelta(days=1)).strftime("%Y-%m-%d") if date_end_excl else "",
                "focus_only": focus_only,
                "focus_counterparty_strict": True,
                "focus_id_count": len(selected_focus_ids),
                "focus_name_count": 0,
                "focus_unknown_name": False,
                "include_missing_counterparty": include_missing_counterparty,
                "focus_key_type": focus_key_type,
                "build_mode": "stats_focus_account_fast",
            },
        }
        if expected_total_amount is not None:
            stats["expected_total_amount"] = _round2(expected_total_amount)
            stats["expected_total_amount_delta"] = _round2(total_amount - expected_total_amount)
        if expected_row_count is not None:
            stats["expected_row_count"] = int(expected_row_count)
            stats["expected_row_count_delta"] = int(len(edges_out) - int(expected_row_count))

        return {
            "nodes": nodes_out,
            "edges": edges_out,
            "stats": stats,
        }

    def _build_runtime_graph(
        self,
        *,
        case_id: str,
        seeds: Sequence[str],
        depth: int,
        direction: str,
        min_amount: float,
        request_context: Optional[dict] = None,
        expected_amount_coverage: TxnAmountCoverageV1,
    ) -> dict:
        context_obj = request_context if isinstance(request_context, dict) else {}

        seed_ids: list[str] = []
        seen_seed: set[str] = set()
        for item in seeds:
            key = str(item or "").strip()
            if not key or key in seen_seed:
                continue
            seen_seed.add(key)
            seed_ids.append(key)

        focus_self_only = bool(
            context_obj.get("focus_self_only") or context_obj.get("focusSelfOnly") or context_obj.get("onlySelf")
        )
        dir_mode = direction if direction in {"in", "out"} else "all"
        view_mode = str(context_obj.get("view") or context_obj.get("mode") or "relation").strip().lower()
        if view_mode not in {"relation", "net"}:
            view_mode = "relation"

        focus_id_raw = str(context_obj.get("focus_id") or context_obj.get("focusId") or "").strip()
        focus_name = str(context_obj.get("focus_name") or context_obj.get("focusName") or "").strip()
        focus_label = str(context_obj.get("focus_label") or context_obj.get("focusLabel") or "").strip()
        focus_ids = _to_text_list(context_obj.get("focus_ids") or context_obj.get("focusIds"))
        focus_names = _to_text_list(context_obj.get("focus_names") or context_obj.get("focusNames"))
        focus_placeholder_kinds = normalize_placeholder_kinds(
            context_obj.get("focus_placeholder_kinds") or context_obj.get("focusPlaceholderKinds") or []
        )
        left_seed_ids = _to_text_list(context_obj.get("left_seeds") or context_obj.get("leftSeeds"))
        request_id = str(context_obj.get("request_id") or context_obj.get("requestId") or "").strip()
        source = _normalize_flow_source(context_obj.get("source"))
        focus_key_type = str(context_obj.get("focus_key_type") or context_obj.get("focusKeyType") or "").strip().lower()
        no_limit_stats = source == "stats"

        if focus_id_raw and focus_id_raw not in focus_ids:
            focus_ids.insert(0, focus_id_raw)
        if focus_name and focus_name not in focus_names:
            focus_names.insert(0, focus_name)
        if focus_ids and not focus_id_raw:
            focus_id_raw = focus_ids[0]
        if focus_names and not focus_name:
            focus_name = focus_names[0]

        focus_only = bool(
            context_obj.get("focus_only")
            or context_obj.get("focusOnly")
            or focus_ids
            or focus_names
            or focus_placeholder_kinds
        )
        if not focus_label:
            focus_label = focus_name
        if source == "stats" and focus_key_type == "account":
            focus_name = ""
            focus_names = []
            focus_ids = [item for item in focus_ids if not placeholder_kind_from_token(item)]
            if placeholder_kind_from_token(focus_id_raw):
                focus_id_raw = ""
        if source == "stats" and focus_key_type == "name":
            focus_id_raw = ""
            focus_ids = []

        unknown_name_label = "未知户名"
        unknown_card_label = "未知账号"
        payload_focus_unknown = bool(context_obj.get("focus_unknown_name") or context_obj.get("focusUnknownName"))
        payload_include_missing = bool(
            context_obj.get("include_missing_counterparty")
            or context_obj.get("includeMissingCounterparty")
            or context_obj.get("include_unknown_account")
            or context_obj.get("includeUnknownAccount")
            or focus_placeholder_kinds
        )
        payload_focus_account_strict = bool(
            context_obj.get("focus_counterparty_strict") or context_obj.get("focusCounterpartyStrict")
        )
        focus_unknown_name = bool(
            source == "stats"
            and focus_only
            and focus_key_type == "name"
            and (payload_focus_unknown or not focus_name)
        )
        if focus_unknown_name and not focus_label:
            focus_label = unknown_name_label

        hop = max(1, int(depth or 1))
        max_edges = max(50, min(5000, int(context_obj.get("maxEdges") or context_obj.get("max_edges") or 800)))
        date_start = _parse_ymd(context_obj.get("date_start") or context_obj.get("dateStart"))
        date_end = _parse_ymd(context_obj.get("date_end") or context_obj.get("dateEnd"))
        date_end_excl = date_end + timedelta(days=1) if date_end else None

        expected_total_amount = _optional_expected_amount(
            context_obj.get("expected_total_amount", context_obj.get("expectedTotalAmount"))
        )
        expected_row_count = _to_optional_int(
            context_obj.get("expected_row_count", context_obj.get("expectedRowCount"))
        )
        stats_focus_account_txn_level = bool(source == "stats" and focus_only and focus_key_type == "account")

        def empty_graph(seed_count: int) -> dict:
            stats = {
                "requested_depth": int(depth),
                "direction": direction,
                "seed_count": seed_count,
                "node_count": 0,
                "edge_count": 0,
                "total_amount": None,
                "view_mode": view_mode,
                "context_applied": {
                    "source": source,
                    "request_id": request_id,
                    "date_start": date_start.strftime("%Y-%m-%d") if date_start else "",
                    "date_end": date_end.strftime("%Y-%m-%d") if date_end else "",
                    "focus_only": focus_only,
                    "focus_counterparty_strict": payload_focus_account_strict,
                    "focus_id_count": len(focus_ids),
                    "focus_name_count": len(focus_names),
                    "focus_unknown_name": focus_unknown_name,
                    "include_missing_counterparty": payload_include_missing,
                },
            }
            if expected_total_amount is not None:
                stats["expected_total_amount"] = _round2(expected_total_amount)
                stats["expected_total_amount_delta"] = None
            if expected_row_count is not None:
                stats["expected_row_count"] = int(expected_row_count)
                stats["expected_row_count_delta"] = int(0 - int(expected_row_count))
            return _attach_graph_render_metadata({"nodes": [], "edges": [], "stats": stats, "runtime_graph": {"nodes": [], "edges": []}})

        if not seed_ids and not focus_self_only:
            return empty_graph(0)

        if focus_self_only and (focus_id_raw or focus_label or focus_name):
            node_id = focus_id_raw or focus_label or focus_name
            node_name = focus_label or focus_name
            display_id = focus_id_raw if focus_id_raw else ""
            node = {
                "id": node_id,
                "title": node_name or node_id,
                "name": node_name or "",
                "display_id": display_id,
                "display_ids": [display_id] if display_id else None,
                "ntype": "seed",
                "total_amount": None,
                "total_count": 0,
            }
            stats = empty_graph(len(seed_ids))
            stats["nodes"] = [node]
            stats["stats"]["node_count"] = 1
            if isinstance(stats.get("runtime_graph"), dict):
                stats["runtime_graph"]["nodes"] = [dict(node)]
            return _attach_graph_render_metadata(stats)

        con = self._storage.open_case_engine(case_id, read_only=False)
        try:
            con.execute("BEGIN TRANSACTION")
            transaction_coverage = self._daily_agg.require_complete_amount_coverage(
                case_id=case_id,
                engine=con,
                ensure_ready=False,
            )
            if transaction_coverage.digest != expected_amount_coverage.digest:
                raise TxnAmountCoverageIncompleteError()
            detail_table_exists = self._table_exists(con, "analysis_txn_detail_idx")
            raw_table_exists = detail_table_exists
            account_dim_exists = self._table_exists(con, "analysis_account_dim")
            use_materialized_source = False
            use_stats_name_replacement_dedupe = False
            if (
                not stats_focus_account_txn_level
                and self._can_use_materialized_graph_source(min_amount=min_amount, request_context=context_obj)
            ):
                use_materialized_source = self._table_exists(con, "analysis_txn_daily_agg")
            if source == "stats" and focus_only and focus_key_type == "name" and detail_table_exists and account_dim_exists:
                use_materialized_source = False
                use_stats_name_replacement_dedupe = True
            if not raw_table_exists and not use_materialized_source and not use_stats_name_replacement_dedupe:
                return empty_graph(len(seed_ids))

            cols = self._table_columns(con, "analysis_txn_detail_idx") if raw_table_exists else set()
            acct_key_expr = _account_key_expr("t", cols, candidates=_ACCOUNT_KEY_CANDIDATES) if raw_table_exists else "NULL"
            cp_key_expr = _counterparty_key_expr("t", cols) if raw_table_exists else "NULL"
            cp_raw_expr = _counterparty_raw_expr("t", cols) if raw_table_exists else "NULL"
            cp_placeholder_kind_expr = placeholder_kind_sql(cp_raw_expr) if cp_raw_expr != "NULL" else "NULL"
            dc_expr = _dc_value_expr("t", cols) if raw_table_exists else "NULL"
            amt_expr = _amount_value_expr("t", cols) if raw_table_exists else "0"
            ts_expr = _txn_ts_expr("t", cols) if raw_table_exists else "NULL"
            open_name_expr = _text_col_expr("t", cols, "account_open_name", clean_invalid=True) if raw_table_exists else None
            cp_name_expr = _text_col_expr("t", cols, "counterparty_name", clean_invalid=True) if raw_table_exists else None
            open_name_expr = open_name_expr or "NULL"
            cp_name_expr = cp_name_expr or "NULL"

            if (
                not use_materialized_source
                and not use_stats_name_replacement_dedupe
                and (acct_key_expr == "NULL" or (cp_key_expr == "NULL" and cp_raw_expr == "NULL"))
            ):
                return empty_graph(len(seed_ids))

            if raw_table_exists:
                fast_stats_graph = self._build_stats_focus_account_fast_graph(
                    con=con,
                    case_id=case_id,
                    depth=hop,
                    direction=direction,
                    min_amount=min_amount,
                    view_mode=view_mode,
                    seed_ids=seed_ids,
                    left_seed_ids=left_seed_ids,
                    focus_id_raw=focus_id_raw,
                    focus_label=focus_label,
                    focus_ids=focus_ids,
                    focus_placeholder_kinds=focus_placeholder_kinds,
                    request_id=request_id,
                    source=source,
                    focus_only=focus_only,
                    focus_key_type=focus_key_type,
                    payload_focus_account_strict=payload_focus_account_strict,
                    payload_include_missing=payload_include_missing,
                    expected_total_amount=expected_total_amount,
                    expected_row_count=expected_row_count,
                    date_start=date_start,
                    date_end_excl=date_end_excl,
                    cols=cols,
                )
                if fast_stats_graph is not None:
                    return _attach_graph_render_metadata(fast_stats_graph)

            if use_stats_name_replacement_dedupe:
                base_sql = _stats_name_replacement_dedupe_base_sql()
                txn_count_select_expr = "COUNT(1)"
                first_ts_select_expr = "MIN(txn_ts)"
                last_ts_select_expr = "MAX(txn_ts)"
                time_filter_field = "txn_ts"
                date_start_param = date_start
                date_end_param = date_end_excl
            elif use_materialized_source:
                base_sql = (
                    "WITH base AS ("
                    "  SELECT "
                    "    acct_key AS acct_key, "
                    "    cp_key AS cp_key, "
                    "    cp_raw AS cp_key_raw, "
                    "    cp_placeholder_kind AS cp_placeholder_kind, "
                    "    dc_val AS dc_val, "
                    "    txn_count AS txn_count, "
                    "    ABS(amt_sum) AS amt_abs, "
                    "    first_ts AS first_ts, "
                    "    last_ts AS last_ts, "
                    "    txn_day AS txn_day, "
                    "    open_name AS open_name, "
                    "    cp_name AS cp_name "
                    "  FROM analysis_txn_daily_agg t "
                    ") "
                )
                txn_count_select_expr = "SUM(txn_count)"
                first_ts_select_expr = "MIN(first_ts)"
                last_ts_select_expr = "MAX(last_ts)"
                time_filter_field = "txn_day"
                date_start_param = date_start.strftime("%Y-%m-%d") if date_start else None
                date_end_param = date_end_excl.strftime("%Y-%m-%d") if date_end_excl else None
            else:
                base_sql = (
                    "WITH base AS ("
                    "  SELECT "
                    f"    {acct_key_expr} AS acct_key, "
                    f"    {cp_key_expr} AS cp_key, "
                    f"    {cp_raw_expr} AS cp_key_raw, "
                    f"    {cp_placeholder_kind_expr} AS cp_placeholder_kind, "
                    f"    {dc_expr} AS dc_val, "
                    f"    ABS({amt_expr}) AS amt_abs, "
                    f"    {ts_expr} AS txn_ts, "
                    f"    {open_name_expr} AS open_name, "
                    f"    {cp_name_expr} AS cp_name "
                    "  FROM analysis_txn_detail_idx t "
                    "  WHERE amount_source_present=1 "
                    "    AND amount_parse_failed=0 "
                    "    AND amount IS NOT NULL "
                    "    AND dc_val IN ('进','出')"
                    ") "
                )
                txn_count_select_expr = "COUNT(1)"
                first_ts_select_expr = "MIN(txn_ts)"
                last_ts_select_expr = "MAX(txn_ts)"
                time_filter_field = "txn_ts"
                date_start_param = date_start
                date_end_param = date_end_excl

            cp_name_select_expr = "MAX(cp_name)"
            from_clause = "FROM base"
            if cp_name_expr != "NULL" or use_materialized_source or use_stats_name_replacement_dedupe:
                name_count_expr = "SUM(txn_count)" if use_materialized_source else "COUNT(*)"
                name_last_ts_expr = "MAX(last_ts)" if use_materialized_source else "MAX(txn_ts)"
                base_sql += (
                    ", name_pick AS ("
                    "  SELECT cp_key AS pick_key, cp_name AS pick_name "
                    "  FROM ("
                    "    SELECT cp_key, cp_name, "
                    f"      {name_count_expr} AS name_cnt, "
                    f"      {name_last_ts_expr} AS name_last_ts, "
                    "      ROW_NUMBER() OVER ("
                    "        PARTITION BY cp_key "
                    "        ORDER BY name_cnt DESC, name_last_ts DESC, cp_name DESC"
                    "      ) AS rn "
                    "    FROM base "
                    "    WHERE cp_name IS NOT NULL AND TRIM(cp_name) <> '' "
                    "    GROUP BY cp_key, cp_name"
                    "  ) t "
                    "  WHERE rn=1"
                    ") "
                )
                from_clause = "FROM base LEFT JOIN name_pick ON name_pick.pick_key = base.cp_key"
                cp_name_select_expr = "COALESCE(MAX(name_pick.pick_name), MAX(cp_name))"

            @dataclass
            class _EdgeAggregate:
                a: str
                b: str
                amount: float = 0.0
                count: int = 0
                out_amount: float = 0.0
                in_amount: float = 0.0
                first_ts: Optional[object] = None
                last_ts: Optional[object] = None

            edges: dict[tuple[str, str], _EdgeAggregate] = {}
            node_amount: dict[str, float] = {}
            node_count: dict[str, int] = {}
            node_name: dict[str, str] = {}
            node_name_source: dict[str, str] = {}
            node_display: dict[str, str] = {}
            cp_display_ids: dict[str, set[str]] = {}
            node_display_list: dict[str, list[str]] = {}
            acct_seen: set[str] = set()
            cp_name_missing: set[str] = set()

            def _norm_focus_key(value: str) -> str:
                text = str(value or "").strip()
                if not text:
                    return ""
                text = "".join(text.split())
                idx_dash = text.find("-")
                idx_under = text.find("_")
                if idx_dash > 0 or idx_under > 0:
                    if idx_dash <= 0:
                        idx = idx_under
                    elif idx_under <= 0:
                        idx = idx_dash
                    else:
                        idx = min(idx_dash, idx_under)
                    text = text[:idx]
                return text

            def _looks_like_id(value: str) -> bool:
                text = str(value or "").strip()
                return bool(text and text.isdigit() and len(text) >= 8)

            def _usable_account_key(value: str) -> bool:
                text = str(value or "").strip()
                if not text or text.startswith("__unknown_cp__name::") or _is_placeholder_node_id(text):
                    return False
                if _is_missing_account_key(text):
                    return False
                return text != "0"

            focus_name_list = list(focus_names)
            focus_name_set = set(focus_name_list)
            selected_placeholder_kind_list = list(focus_placeholder_kinds)
            if payload_include_missing and focus_key_type == "account" and not selected_placeholder_kind_list:
                selected_placeholder_kind_list = list(PLACEHOLDER_KIND_LABELS.keys())
            selected_placeholder_kind_set = set(selected_placeholder_kind_list)
            focus_id_set: set[str] = set()
            for focus_id in focus_ids:
                focus_id_set.add(focus_id)
                normalized = _norm_focus_key(focus_id)
                if normalized:
                    focus_id_set.add(normalized)
            for maybe_id in [name for name in focus_name_list if _looks_like_id(name)]:
                focus_id_set.add(maybe_id)
                normalized = _norm_focus_key(maybe_id)
                if normalized:
                    focus_id_set.add(normalized)

            auto_focus_account_strict = bool(
                source == "stats" and focus_only and focus_key_type == "account" and len(focus_id_set) >= 180
            )
            focus_account_counterparty_strict = bool(payload_focus_account_strict or auto_focus_account_strict)
            focus_match_id_set: set[str] = set(focus_id_set)
            if source == "stats" and focus_only and focus_key_type == "account" and not focus_account_counterparty_strict:
                for seed_id in seed_ids:
                    seed_text = str(seed_id or "").strip()
                    if not seed_text:
                        continue
                    focus_match_id_set.add(seed_text)
                    normalized = _norm_focus_key(seed_text)
                    if normalized:
                        focus_match_id_set.add(normalized)

            query_seed_ids = list(seed_ids)
            if source == "stats" and focus_only and focus_key_type == "account" and focus_account_counterparty_strict:
                strict_left = [seed_id for seed_id in left_seed_ids if _usable_account_key(seed_id)]
                if strict_left:
                    query_seed_ids = strict_left
            if source == "stats" and focus_only and focus_key_type == "account":
                seed_seen = set(query_seed_ids)
                added_focus_frontier = 0
                for focus_id in [focus_id_raw] + list(focus_ids):
                    key = str(focus_id or "").strip()
                    if (
                        focus_account_counterparty_strict
                        or not _usable_account_key(key)
                        or key in seed_seen
                        or added_focus_frontier >= 220
                    ):
                        continue
                    seed_seen.add(key)
                    query_seed_ids.append(key)
                    added_focus_frontier += 1

            visited: set[str] = set(query_seed_ids)
            frontier: set[str] = set(query_seed_ids)
            max_nodes = max(200, max_edges * 3)

            include_missing_counterparty = bool(
                payload_include_missing
                or (source == "stats" and focus_only and focus_key_type == "name" and (focus_name_set or focus_unknown_name))
            )
            include_unknown_empty = bool((focus_key_type == "name" and payload_include_missing) or focus_unknown_name)
            include_missing_by_account = bool(focus_key_type == "account" and selected_placeholder_kind_set)
            unknown_prefix = "__unknown_cp__name::"
            placeholder_prefix = PLACEHOLDER_TOKEN_PREFIX
            unknown_cp_id_empty = ""
            if include_missing_counterparty and include_unknown_empty:
                unknown_cp_id_empty = f"{unknown_prefix}__empty__"
                node_display[unknown_cp_id_empty] = unknown_card_label
                node_name.setdefault(unknown_cp_id_empty, unknown_name_label)

            if include_missing_counterparty:
                if focus_key_type == "name":
                    if include_unknown_empty:
                        cp_key_norm_expr = (
                            "CASE WHEN cp_key IS NOT NULL THEN cp_key "
                            "WHEN cp_name IS NOT NULL THEN "
                            f"'{unknown_prefix}' || cp_name "
                            "WHEN cp_key IS NULL THEN "
                            f"'{unknown_cp_id_empty}' "
                            "ELSE cp_key END"
                        )
                    else:
                        cp_key_norm_expr = (
                            "CASE WHEN cp_key IS NOT NULL THEN cp_key "
                            "WHEN cp_name IS NOT NULL THEN "
                            f"'{unknown_prefix}' || cp_name "
                            "WHEN cp_key IS NULL THEN NULL "
                            "ELSE cp_key END"
                        )
                else:
                    cp_key_norm_expr = (
                        "CASE "
                        "WHEN cp_placeholder_kind IS NOT NULL AND cp_name IS NOT NULL THEN "
                        f"'{placeholder_prefix}' || cp_placeholder_kind || '::name::' || cp_name "
                        "WHEN cp_placeholder_kind IS NOT NULL THEN "
                        f"'{placeholder_prefix}' || cp_placeholder_kind "
                        "WHEN cp_key IS NOT NULL THEN cp_key "
                        "WHEN cp_name IS NOT NULL THEN "
                        f"'{unknown_prefix}' || cp_name "
                        "ELSE "
                        f"'{unknown_cp_id_empty}' END"
                    )
            else:
                cp_key_norm_expr = "cp_key"

            stats_seed_scope = {
                str(item or "").strip()
                for item in (left_seed_ids or seed_ids or query_seed_ids)
                if _usable_account_key(str(item or "").strip())
            }
            stats_mirror_scope = set(stats_seed_scope)
            stats_mirror_scope.update(
                str(item or "").strip()
                for item in focus_id_set
                if _usable_account_key(str(item or "").strip())
            )
            query_seed_scope = {str(item or "").strip() for item in query_seed_ids}
            use_stats_txn_dedupe = bool(stats_focus_account_txn_level and not use_materialized_source)
            pending_stats_mirrors: dict[tuple[str, str, float], dict[str, list[dict[str, Any]]]] = defaultdict(
                lambda: {"src": [], "tgt": []}
            )

            def _apply_row_context(
                acct_value: Any,
                cp_value: Any,
                cp_raw_value: Any,
                cp_name_value: Any,
                open_name_value: Any,
                next_frontier_ref: set[str],
            ) -> Optional[tuple[str, str, bool]]:
                acct_key = str(acct_value or "").strip()
                cp_key = str(cp_value or "").strip()
                cp_key_raw = str(cp_raw_value or "").strip()
                cp_name = str(cp_name_value or "").strip()
                open_name = str(open_name_value or "").strip()
                if not acct_key or not cp_key:
                    return None

                is_placeholder_cp = _is_placeholder_node_id(cp_key)
                is_unknown_cp = bool(include_missing_counterparty and cp_key.startswith(unknown_prefix))
                acct_seen.add(acct_key)
                if is_placeholder_cp:
                    placeholder_label = placeholder_kind_label(_placeholder_kind_from_node_id(cp_key))
                    if placeholder_label:
                        node_display[cp_key] = placeholder_label
                elif cp_key_raw and cp_key.startswith(unknown_prefix) and not _is_missing_account_key(cp_key_raw):
                    cp_display_ids.setdefault(cp_key, set()).add(cp_key_raw)
                if open_name and node_name_source.get(acct_key) != "open":
                    node_name[acct_key] = open_name
                    node_name_source[acct_key] = "open"

                if not cp_name and not cp_key.startswith(unknown_prefix) and not is_placeholder_cp:
                    cp_name_missing.add(cp_key)
                    if node_name_source.get(cp_key) == "cp":
                        node_name[cp_key] = unknown_name_label
                        node_name_source[cp_key] = "unknown"

                if cp_name:
                    if (is_placeholder_cp or cp_key not in cp_name_missing) and node_name_source.get(cp_key) != "open":
                        if node_name.get(cp_key) in {None, "", unknown_name_label}:
                            node_name[cp_key] = cp_name
                            node_name_source[cp_key] = "cp"
                elif focus_key_type == "name" and cp_key and cp_key in focus_name_set:
                    if node_name.get(cp_key) in {None, ""}:
                        node_name[cp_key] = unknown_name_label
                        node_name_source.setdefault(cp_key, "unknown")
                elif focus_unknown_name and node_name.get(cp_key) in {None, ""}:
                    node_name[cp_key] = unknown_name_label
                    node_name_source.setdefault(cp_key, "unknown")

                if not is_unknown_cp and not is_placeholder_cp and cp_key not in visited and len(visited) < max_nodes:
                    visited.add(cp_key)
                    next_frontier_ref.add(cp_key)
                return acct_key, cp_key, bool(is_unknown_cp or is_placeholder_cp)

            def _merge_first_ts(ts1: Optional[object], ts2: Optional[object]) -> Optional[object]:
                if ts1 is None:
                    return ts2
                if ts2 is None:
                    return ts1
                return ts1 if ts1 <= ts2 else ts2

            def _merge_last_ts(ts1: Optional[object], ts2: Optional[object]) -> Optional[object]:
                if ts1 is None:
                    return ts2
                if ts2 is None:
                    return ts1
                return ts1 if ts1 >= ts2 else ts2

            def _apply_flow_agg(
                *,
                flow_src: str,
                flow_tgt: str,
                amount: float,
                count: int,
                first_ts: Optional[object],
                last_ts: Optional[object],
            ) -> None:
                if not flow_src or not flow_tgt:
                    return
                a, b = sorted([flow_src, flow_tgt])
                edge_key = (a, b)
                agg = edges.get(edge_key)
                if agg is None:
                    agg = _EdgeAggregate(a=a, b=b)
                    edges[edge_key] = agg

                agg.amount += amount
                agg.count += count
                if flow_src == a:
                    agg.out_amount += amount
                else:
                    agg.in_amount += amount
                agg.first_ts = _merge_first_ts(agg.first_ts, first_ts)
                agg.last_ts = _merge_last_ts(agg.last_ts, last_ts)

                if flow_src == flow_tgt:
                    node_amount[flow_src] = node_amount.get(flow_src, 0.0) + amount
                    node_count[flow_src] = node_count.get(flow_src, 0) + count
                    return

                for node_id in (flow_src, flow_tgt):
                    node_amount[node_id] = node_amount.get(node_id, 0.0) + amount
                    node_count[node_id] = node_count.get(node_id, 0) + count

            for _ in range(hop):
                if not frontier:
                    break

                frontier_list = sorted(frontier)
                placeholders = ",".join(["?"] * len(frontier_list))
                step_where = [
                    f"acct_key IN ({placeholders})",
                    "acct_key IS NOT NULL",
                    "dc_val IN ('进','出')",
                ]
                missing_step_where: Optional[list[str]] = None
                if not include_missing_counterparty:
                    step_where.append("cp_placeholder_kind IS NULL")
                    step_where.append("cp_key IS NOT NULL")
                params: list[Any] = []
                params.extend(frontier_list)
                missing_params: Optional[list[Any]] = None

                if source == "stats" and focus_only:
                    if focus_key_type == "account" and focus_account_counterparty_strict:
                        if include_missing_by_account and selected_placeholder_kind_set:
                            missing_step_where = list(step_where)
                            missing_params = list(params)
                            placeholders_missing = ",".join(["?"] * len(selected_placeholder_kind_set))
                            missing_step_where.append(f"cp_placeholder_kind IN ({placeholders_missing})")
                            missing_params.extend(sorted(selected_placeholder_kind_set))
                        if focus_id_set:
                            placeholders_focus = ",".join(["?"] * len(focus_id_set))
                            if use_stats_txn_dedupe and stats_seed_scope:
                                placeholders_seed_cp = ",".join(["?"] * len(stats_seed_scope))
                                step_where.append(
                                    f"((cp_key IN ({placeholders_focus})) "
                                    f"OR (cp_placeholder_kind IS NULL AND cp_key IN ({placeholders_seed_cp})))"
                                )
                                params.extend(sorted(focus_id_set))
                                params.extend(sorted(stats_seed_scope))
                            else:
                                step_where.append(f"cp_key IN ({placeholders_focus})")
                                params.extend(sorted(focus_id_set))
                        elif include_missing_by_account:
                            step_where.append("1=0")
                    elif focus_key_type == "name" and focus_name_list:
                        placeholders_focus = ",".join(["?"] * len(focus_name_list))
                        if focus_unknown_name:
                            step_where.append(f"(cp_name IN ({placeholders_focus}) OR cp_name IS NULL)")
                            params.extend(focus_name_list)
                        else:
                            step_where.append(
                                f"(cp_name IN ({placeholders_focus}) OR (cp_name IS NULL AND cp_key IN ({placeholders_focus})))"
                            )
                            params.extend(focus_name_list)
                            params.extend(focus_name_list)
                    elif focus_key_type == "name" and focus_unknown_name:
                        step_where.append("cp_name IS NULL")

                if dir_mode == "out":
                    step_where.append("dc_val='出'")
                    if missing_step_where is not None:
                        missing_step_where.append("dc_val='出'")
                elif dir_mode == "in":
                    step_where.append("dc_val='进'")
                    if missing_step_where is not None:
                        missing_step_where.append("dc_val='进'")

                if date_start_param is not None:
                    step_where.append(f"{time_filter_field} >= ?")
                    params.append(date_start_param)
                    if missing_step_where is not None and missing_params is not None:
                        missing_step_where.append(f"{time_filter_field} >= ?")
                        missing_params.append(date_start_param)
                if date_end_param is not None:
                    step_where.append(f"{time_filter_field} < ?")
                    params.append(date_end_param)
                    if missing_step_where is not None and missing_params is not None:
                        missing_step_where.append(f"{time_filter_field} < ?")
                        missing_params.append(date_end_param)
                if min_amount > 0:
                    step_where.append("amt_abs >= ?")
                    params.append(float(min_amount))
                    if missing_step_where is not None and missing_params is not None:
                        missing_step_where.append("amt_abs >= ?")
                        missing_params.append(float(min_amount))

                next_frontier: set[str] = set()
                step_where_sql = " AND ".join(step_where)
                if use_stats_txn_dedupe:
                    raw_cp_name_expr = "COALESCE(name_pick.pick_name, cp_name)" if cp_name_expr != "NULL" else "cp_name"
                    raw_sql = (
                        base_sql
                        + "SELECT "
                        f"  acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw AS cp_key_raw, dc_val, "
                        "  amt_abs, txn_ts, "
                        "  open_name AS open_name, "
                        f"  {raw_cp_name_expr} AS cp_name "
                        f"{from_clause} "
                        f"WHERE {step_where_sql} "
                        "ORDER BY txn_ts ASC, amt_abs DESC, acct_key ASC, cp_key ASC, dc_val ASC"
                    )
                    try:
                        raw_rows = con.query(raw_sql, tuple(params))
                    except Exception:
                        raw_rows = []

                    for row in raw_rows:
                        if not row:
                            continue
                        acct_key, cp_key, cp_key_raw, dc_val, amt_abs, txn_ts, open_name, cp_name = row
                        dc_val = str(dc_val or "").strip()
                        if dc_val not in {"进", "出"}:
                            continue
                        row_ctx = _apply_row_context(
                            acct_key,
                            cp_key,
                            cp_key_raw,
                            cp_name,
                            open_name,
                            next_frontier,
                        )
                        if row_ctx is None:
                            continue
                        acct_key_norm, cp_key_norm, _ = row_ctx
                        if dc_val == "出":
                            flow_src, flow_tgt = acct_key_norm, cp_key_norm
                            mirror_side = "src"
                        else:
                            flow_src, flow_tgt = cp_key_norm, acct_key_norm
                            mirror_side = "tgt"

                        amount = _required_amount_float(amt_abs)
                        count = 1
                        row_primary = bool(
                            acct_key_norm in query_seed_scope
                            and (
                                cp_key_norm in focus_match_id_set
                                or _norm_focus_key(cp_key_norm) in focus_match_id_set
                                or _placeholder_kind_from_node_id(cp_key_norm) in selected_placeholder_kind_set
                            )
                        )
                        if (
                            txn_ts is not None
                            and flow_src != flow_tgt
                            and flow_src in stats_mirror_scope
                            and flow_tgt in stats_mirror_scope
                        ):
                            mirror_key = (flow_src, flow_tgt, _round2(amount))
                            opposite_side = "tgt" if mirror_side == "src" else "src"
                            opposite_rows = pending_stats_mirrors[mirror_key][opposite_side]
                            matched_index: Optional[int] = None
                            for index, candidate in enumerate(opposite_rows):
                                candidate_ts = candidate.get("txn_ts")
                                if candidate_ts is None:
                                    continue
                                if (
                                    abs((txn_ts - candidate_ts).total_seconds())
                                    <= _STATS_TXN_MIRROR_TOLERANCE_SECONDS
                                ):
                                    matched_index = index
                                    break
                            if matched_index is not None:
                                matched_row = opposite_rows.pop(matched_index)
                                if not row_primary and not bool(matched_row.get("primary")):
                                    continue
                                _apply_flow_agg(
                                    flow_src=flow_src,
                                    flow_tgt=flow_tgt,
                                    amount=amount,
                                    count=count,
                                    first_ts=_merge_first_ts(matched_row.get("txn_ts"), txn_ts),
                                    last_ts=_merge_last_ts(matched_row.get("txn_ts"), txn_ts),
                                )
                                continue
                            pending_stats_mirrors[mirror_key][mirror_side].append(
                                {
                                    "flow_src": flow_src,
                                    "flow_tgt": flow_tgt,
                                    "amount": amount,
                                    "txn_ts": txn_ts,
                                    "primary": row_primary,
                                }
                            )
                            continue

                        _apply_flow_agg(
                            flow_src=flow_src,
                            flow_tgt=flow_tgt,
                            amount=amount,
                            count=count,
                            first_ts=txn_ts,
                            last_ts=txn_ts,
                        )

                    frontier = next_frontier
                    continue

                limit_sql = "" if no_limit_stats else " LIMIT ?"
                sql = (
                    base_sql
                    + "SELECT "
                    f"  acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw AS cp_key_raw, dc_val, "
                    f"  {txn_count_select_expr} AS txn_count, "
                    "  SUM(amt_abs) AS amt_sum, "
                    f"  {first_ts_select_expr} AS first_ts, "
                    f"  {last_ts_select_expr} AS last_ts, "
                    "  MAX(open_name) AS open_name, "
                    f"  {cp_name_select_expr} AS cp_name "
                    f"{from_clause} "
                    f"WHERE {step_where_sql} "
                    f"GROUP BY acct_key, {cp_key_norm_expr}, cp_key, cp_key_raw, dc_val "
                    "ORDER BY amt_sum DESC "
                    f"{limit_sql}"
                )
                if not no_limit_stats:
                    params.append(max_edges)

                try:
                    rows = con.query(sql, tuple(params))
                except Exception:
                    rows = []

                if missing_step_where is not None and missing_params is not None:
                    missing_sql = (
                        base_sql
                        + "SELECT "
                        f"  acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw AS cp_key_raw, dc_val, "
                        f"  {txn_count_select_expr} AS txn_count, "
                        "  SUM(amt_abs) AS amt_sum, "
                        f"  {first_ts_select_expr} AS first_ts, "
                        f"  {last_ts_select_expr} AS last_ts, "
                        "  MAX(open_name) AS open_name, "
                        f"  {cp_name_select_expr} AS cp_name "
                        f"{from_clause} "
                        f"WHERE {' AND '.join(missing_step_where)} "
                        f"GROUP BY acct_key, {cp_key_norm_expr}, cp_key, cp_key_raw, dc_val "
                        "ORDER BY amt_sum DESC "
                    )
                    try:
                        missing_rows = con.query(missing_sql, tuple(missing_params))
                    except Exception:
                        missing_rows = []
                    if missing_rows:
                        rows = list(rows) + list(missing_rows)

                for row in rows:
                    if not row:
                        continue
                    acct_key, cp_key, cp_key_raw, dc_val, txn_count, amt_sum, first_ts, last_ts, open_name, cp_name = row
                    dc_val = str(dc_val or "").strip()
                    if dc_val not in {"进", "出"}:
                        continue
                    row_ctx = _apply_row_context(
                        acct_key,
                        cp_key,
                        cp_key_raw,
                        cp_name,
                        open_name,
                        next_frontier,
                    )
                    if row_ctx is None:
                        continue
                    acct_key_norm, cp_key_norm, _ = row_ctx

                    if dc_val == "出":
                        flow_src, flow_tgt = acct_key_norm, cp_key_norm
                    else:
                        flow_src, flow_tgt = cp_key_norm, acct_key_norm
                    if amt_sum is None:
                        raise TxnAmountCoverageIncompleteError()
                    _apply_flow_agg(
                        flow_src=flow_src,
                        flow_tgt=flow_tgt,
                        amount=float(amt_sum),
                        count=_required_transaction_count(txn_count),
                        first_ts=first_ts,
                        last_ts=last_ts,
                    )

                frontier = next_frontier

            if use_stats_txn_dedupe:
                for mirror_state in pending_stats_mirrors.values():
                    for side in ("src", "tgt"):
                        for pending_row in mirror_state[side]:
                            if not bool(pending_row.get("primary")):
                                continue
                            _apply_flow_agg(
                                flow_src=str(pending_row.get("flow_src") or "").strip(),
                                flow_tgt=str(pending_row.get("flow_tgt") or "").strip(),
                                amount=_required_amount_float(pending_row.get("amount")),
                                count=1,
                                first_ts=pending_row.get("txn_ts"),
                                last_ts=pending_row.get("txn_ts"),
                            )

            for node_id, ids in cp_display_ids.items():
                if not ids:
                    continue
                show_ids = sorted(ids)
                node_display_list[node_id] = show_ids
                node_display[node_id] = " / ".join(show_ids[:6]) + (f" 等{len(show_ids)}个" if len(show_ids) > 6 else "")

            def _edge_match_id(edge: _EdgeAggregate) -> bool:
                if not focus_match_id_set:
                    return False
                return (
                    edge.a in focus_match_id_set
                    or edge.b in focus_match_id_set
                    or _norm_focus_key(edge.a) in focus_match_id_set
                    or _norm_focus_key(edge.b) in focus_match_id_set
                )

            def _edge_match_name(edge: _EdgeAggregate) -> bool:
                if not focus_name_list:
                    return False
                for node_id in (edge.a, edge.b):
                    name = str(node_name.get(node_id) or "").strip()
                    if not name:
                        continue
                    for focus_name_item in focus_name_list:
                        if focus_name_item and focus_name_item in name:
                            return True
                return False

            def _edge_match_placeholder(edge: _EdgeAggregate) -> bool:
                if not selected_placeholder_kind_set:
                    return False
                return (
                    _placeholder_kind_from_node_id(edge.a) in selected_placeholder_kind_set
                    or _placeholder_kind_from_node_id(edge.b) in selected_placeholder_kind_set
                )

            def _edge_match_unknown_name(edge: _EdgeAggregate) -> bool:
                if not focus_unknown_name or not unknown_cp_id_empty:
                    return False
                return edge.a == unknown_cp_id_empty or edge.b == unknown_cp_id_empty

            if not focus_only:
                filtered_edges = list(edges.values())
            else:
                want_placeholder = bool(selected_placeholder_kind_set)
                want_unknown = bool(focus_unknown_name)
                want_name = bool(focus_name_list)
                want_id = bool(focus_match_id_set)
                filtered_edges = [
                    edge
                    for edge in edges.values()
                    if (
                        (want_id and _edge_match_id(edge))
                        or (want_name and _edge_match_name(edge))
                        or (want_placeholder and _edge_match_placeholder(edge))
                        or (want_unknown and _edge_match_unknown_name(edge))
                    )
                ]

            edge_list = sorted(filtered_edges, key=lambda edge: edge.amount, reverse=True)
            if not no_limit_stats:
                edge_list = edge_list[:max_edges]
            self_loop_edges = sum(1 for edge in edge_list if edge.a == edge.b)

            node_ids: set[str] = set()
            if not focus_only:
                node_ids.update(seed_ids)
            for edge in edge_list:
                node_ids.add(edge.a)
                node_ids.add(edge.b)
            if focus_only and not node_ids and focus_id_raw:
                node_ids.add(focus_id_raw)
            if include_missing_counterparty and node_ids:
                for node_id in node_ids:
                    if node_id.startswith(unknown_prefix) and node_id not in node_display:
                        node_display[node_id] = unknown_card_label
                    if _is_placeholder_node_id(node_id) and node_id not in node_display:
                        node_display[node_id] = placeholder_kind_label(_placeholder_kind_from_node_id(node_id)) or node_id
            self._apply_account_display_metadata(
                con=con,
                case_id=case_id,
                account_ids=node_ids.intersection(acct_seen),
                node_display=node_display,
                node_display_list=node_display_list,
                node_name=node_name,
                node_name_source=node_name_source,
            )

            nodes_out = []
            for node_id in sorted(node_ids):
                name = str(node_name.get(node_id) or "").strip()
                if not name and focus_label and focus_id_raw:
                    try:
                        if _norm_focus_key(node_id) == _norm_focus_key(focus_id_raw):
                            name = focus_label
                    except Exception:
                        pass
                display_id = node_display.get(node_id) or node_id
                ntype = "seed" if node_id in seen_seed else ("account" if node_id in acct_seen else "node")
                if _is_placeholder_node_id(node_id):
                    title = f"{display_id} | {name}" if name else display_id
                elif ntype in {"seed", "account"}:
                    title = f"{display_id} | {name}" if name and name != display_id else display_id
                else:
                    title = name if name else node_id
                display_ids = node_display_list.get(node_id)
                nodes_out.append(
                    {
                        "id": node_id,
                        "title": title,
                        "name": name,
                        "display_id": display_id,
                        "display_ids": display_ids,
                        "ntype": ntype,
                        "total_amount": _project_node_total_amount(node_amount, node_id),
                        "total_count": int(node_count.get(node_id, 0)),
                    }
                )

            edges_out = []
            for edge in edge_list:
                if view_mode == "net":
                    net = edge.out_amount - edge.in_amount
                    if abs(net) < 1e-9:
                        continue
                    if net > 0:
                        source_id, target_id = edge.a, edge.b
                    else:
                        source_id, target_id = edge.b, edge.a
                    amount = abs(net)
                    edges_out.append(
                        {
                            "id": f"{edge.a}=={edge.b}",
                            "source": source_id,
                            "target": target_id,
                            "amount": _round2(amount),
                            "count": int(edge.count),
                            "out_amount": _round2(edge.out_amount),
                            "in_amount": _round2(edge.in_amount),
                            "forward_amount": _round2(amount),
                            "reverse_amount": 0.0,
                            "mode": "single",
                            "label": f"￥{amount:.2f}",
                            "first_time": _dt_to_str(edge.first_ts),
                            "last_time": _dt_to_str(edge.last_ts),
                        }
                    )
                    continue

                if edge.out_amount > 0 and edge.in_amount == 0:
                    source_id, target_id = edge.a, edge.b
                elif edge.in_amount > 0 and edge.out_amount == 0:
                    source_id, target_id = edge.b, edge.a
                elif edge.out_amount >= edge.in_amount:
                    source_id, target_id = edge.a, edge.b
                else:
                    source_id, target_id = edge.b, edge.a

                if source_id == edge.a and target_id == edge.b:
                    forward_amount = edge.out_amount
                    reverse_amount = edge.in_amount
                else:
                    forward_amount = edge.in_amount
                    reverse_amount = edge.out_amount

                edges_out.append(
                    {
                        "id": f"{edge.a}=={edge.b}",
                        "source": source_id,
                        "target": target_id,
                        "amount": _round2(edge.amount),
                        "count": int(edge.count),
                        "out_amount": _round2(edge.out_amount),
                        "in_amount": _round2(edge.in_amount),
                        "forward_amount": _round2(forward_amount),
                        "reverse_amount": _round2(reverse_amount),
                        "mode": "double" if edge.out_amount > 0 and edge.in_amount > 0 else "single",
                        "first_time": _dt_to_str(edge.first_ts),
                        "last_time": _dt_to_str(edge.last_ts),
                    }
                )

            total_amount = _round2(sum(_required_amount_float(item.get("amount")) for item in edges_out))
            stats = {
                "requested_depth": int(depth),
                "direction": direction,
                "seed_count": len(seed_ids),
                "node_count": len(nodes_out),
                "edge_count": len(edges_out),
                "self_loop_edge_count": self_loop_edges,
                "total_amount": total_amount,
                "view_mode": view_mode,
                "context_applied": {
                    "source": source,
                    "request_id": request_id,
                    "date_start": date_start.strftime("%Y-%m-%d") if date_start else "",
                    "date_end": date_end.strftime("%Y-%m-%d") if date_end else "",
                    "focus_only": focus_only,
                    "focus_counterparty_strict": focus_account_counterparty_strict,
                    "focus_id_count": len(focus_ids),
                    "focus_name_count": len(focus_names),
                    "focus_unknown_name": focus_unknown_name,
                    "include_missing_counterparty": include_missing_counterparty,
                    "focus_key_type": focus_key_type,
                },
            }
            if expected_total_amount is not None:
                stats["expected_total_amount"] = _round2(expected_total_amount)
                stats["expected_total_amount_delta"] = _round2(total_amount - expected_total_amount)
            if expected_row_count is not None:
                stats["expected_row_count"] = int(expected_row_count)
                stats["expected_row_count_delta"] = int(len(edges_out) - int(expected_row_count))

            return {
                "nodes": nodes_out,
                "edges": edges_out,
                "stats": stats,
            }
        finally:
            try:
                con.execute("ROLLBACK")
            except Exception:
                pass
            try:
                con.close()
            except Exception:
                pass

    def create_view(self, *, case_id: str) -> dict:
        data = self._load_case_views(case_id)
        views = list(data.get("views") or [])
        item = self._public_view_item(case_id=case_id, view_id=str(uuid.uuid4()))
        views.append(item)
        data["case_id"] = case_id
        data["version"] = _FLOW_VIEW_METADATA_VERSION
        data["views"] = views
        self._save_case_views(case_id, data)
        return item

    def list_views(self, *, case_id: str, page: int, page_size: int) -> Tuple[List[dict], int]:
        data = self._load_case_views(case_id)
        views = list(data.get("views") or [])
        total = len(views)
        start = (int(page) - 1) * int(page_size)
        end = start + int(page_size)
        return views[start:end], total

    def get_view(self, *, case_id: str, view_id: str) -> dict:
        _, _, item = self._find_case_view(case_id=case_id, view_id=view_id)
        return item

    def update_view(self, *, case_id: str, view_id: str) -> dict:
        _, _, item = self._find_case_view(case_id=case_id, view_id=view_id)
        return item

    def delete_view(self, *, case_id: str, view_id: str) -> None:
        case_id, data, item = self._find_case_view(case_id=case_id, view_id=view_id)
        views = [view for view in list(data.get("views") or []) if str(view.get("view_id") or "") != item["view_id"]]
        data["views"] = views
        self._save_case_views(case_id, data)

    def reorder_views(self, *, case_id: str, order: Sequence[str]) -> dict:
        data = self._load_case_views(case_id)
        views = list(data.get("views") or [])
        by_id = {str(view.get("view_id") or ""): view for view in views if str(view.get("view_id") or "").strip()}

        used: set[str] = set()
        reordered: list[dict] = []
        for raw_view_id in order or []:
            view_id = str(raw_view_id or "").strip()
            if not view_id or view_id in used:
                continue
            item = by_id.get(view_id)
            if item is None:
                continue
            reordered.append(item)
            used.add(view_id)

        for item in views:
            view_id = str(item.get("view_id") or "").strip()
            if view_id and view_id not in used:
                reordered.append(item)

        data["views"] = reordered
        self._save_case_views(case_id, data)
        return {"ok": True, "count": len(reordered)}

    def _views_dir(self) -> Path:
        return Path(self._storage.app_dir) / "flow_view_metadata_v2"

    def _case_views_path(self, case_id: str) -> Path:
        try:
            component = case_bound_storage_name(case_id)
        except (UnicodeError, ValueError):
            raise FlowViewMetadataError("flow_view_case_invalid")
        return self._views_dir() / f"{component}.json"

    def _result_snapshots_dir(self, case_id: str) -> Path:
        try:
            component = case_bound_storage_name(case_id)
        except (UnicodeError, ValueError):
            raise FlowResultSnapshotError("flow_result_snapshot_case_invalid") from None
        return Path(self._storage.app_dir) / "flow_result_snapshots" / component

    def _result_snapshot_path(self, case_id: str, snapshot_id: str) -> Path:
        safe_id = safe_fs_name(snapshot_id or "snapshot", "snapshot")
        return self._result_snapshots_dir(case_id) / f"{safe_id}.json"

    def _job_result_manifests_dir(self, case_id: str) -> Path:
        try:
            component = case_bound_storage_name(case_id)
        except (UnicodeError, ValueError):
            raise FlowJobResultManifestError("flow_job_result_case_invalid")
        return Path(self._storage.app_dir) / "flow_job_results" / component

    def _result_snapshot_case_lock_path(self, case_id: str) -> Path:
        try:
            component = case_bound_storage_name(case_id)
        except (UnicodeError, ValueError):
            raise FlowResultSnapshotError("flow_result_snapshot_case_invalid") from None
        return Path(self._storage.app_dir) / "flow_result_snapshot_locks" / f"{component}.lock"

    def _result_snapshot_inventory_lock_path(self) -> Path:
        return Path(self._storage.app_dir) / "flow_result_snapshot_inventory.lock"

    def _case_binding_generation(self, case_id: str) -> Optional[int]:
        state_getter = getattr(self._storage, "get_case_binding_state", None)
        if not callable(state_getter):
            return None
        try:
            state = state_getter(case_id)
        except Exception:
            return None
        if not isinstance(state, dict):
            return None
        generation = state.get("lifecycle_generation")
        if (
            str(state.get("case_id") or "") != case_id
            or str(state.get("deleted_at") or "").strip()
            or type(generation) is not int
            or generation < 1
        ):
            return None
        return generation

    @staticmethod
    def _normalize_case_lifecycle_binding(
        value: Any,
    ) -> Optional[FlowCaseLifecycleBindingV1]:
        if isinstance(value, FlowCaseLifecycleBindingV1):
            binding = value
        elif isinstance(value, dict) and set(value) == {
            "version",
            "case_id",
            "lifecycle_generation",
            "binding_digest",
        }:
            if value.get("version") != 1:
                return None
            generation = value.get("lifecycle_generation")
            if type(generation) is not int or generation < 1:
                return None
            binding = FlowCaseLifecycleBindingV1(
                case_id=str(value.get("case_id") or "").strip(),
                generation=generation,
                binding_digest=str(value.get("binding_digest") or "").strip(),
            )
        else:
            return None
        if (
            not binding.case_id
            or binding.generation < 1
            or binding.binding_digest
            != _flow_case_lifecycle_binding_digest(binding.case_id, binding.generation)
        ):
            return None
        return binding

    def freeze_case_lifecycle_binding(self, case_id: str) -> FlowCaseLifecycleBindingV1:
        normalized_case_id = str(case_id or "").strip()
        generation = self._case_binding_generation(normalized_case_id)
        if generation is None:
            raise FlowResultSnapshotError("flow_case_lifecycle_binding_unavailable")
        return FlowCaseLifecycleBindingV1(
            case_id=normalized_case_id,
            generation=generation,
            binding_digest=_flow_case_lifecycle_binding_digest(normalized_case_id, generation),
        )

    def require_case_lifecycle_binding_current(
        self,
        value: Any,
        *,
        case_id: Optional[str] = None,
    ) -> FlowCaseLifecycleBindingV1:
        binding = self._normalize_case_lifecycle_binding(value)
        expected_case_id = str(
            case_id if case_id is not None else (binding.case_id if binding is not None else "")
        ).strip()
        if (
            binding is None
            or binding.case_id != expected_case_id
            or not self._case_binding_is_publishable(
                expected_case_id,
                expected_generation=binding.generation,
            )
        ):
            raise FlowResultSnapshotError("flow_case_lifecycle_binding_stale")
        return binding

    @contextmanager
    def case_lifecycle_publication_lease(
        self,
        value: Any,
        *,
        case_id: Optional[str] = None,
    ) -> Iterator[FlowCaseLifecycleBindingV1]:
        """Linearize one terminal publication against case lifecycle changes.

        Case deletion, restore, and purge use this same per-case lock. A caller
        that publishes a terminal job state and its public event inside this
        lease therefore either finishes entirely before the lifecycle change,
        or observes the new generation and fails closed before publication.
        """

        binding = self._normalize_case_lifecycle_binding(value)
        expected_case_id = str(
            case_id if case_id is not None else (binding.case_id if binding is not None else "")
        ).strip()
        if binding is None or binding.case_id != expected_case_id:
            raise FlowResultSnapshotError("flow_case_lifecycle_binding_stale")
        with private_exclusive_file_lock(self._result_snapshot_case_lock_path(expected_case_id)):
            current = self.require_case_lifecycle_binding_current(
                binding,
                case_id=expected_case_id,
            )
            yield current
            self.require_case_lifecycle_binding_current(
                current,
                case_id=expected_case_id,
            )

    def _case_binding_is_publishable(
        self,
        case_id: str,
        *,
        expected_generation: Optional[int] = None,
    ) -> bool:
        generation = self._case_binding_generation(case_id)
        return generation is not None and (
            expected_generation is None or generation == expected_generation
        )

    def _result_snapshot_ref_is_reconstructable(
        self,
        case_id: str,
        snapshot_ref: Optional[dict[str, Any]],
    ) -> bool:
        ref = _normalize_snapshot_ref(snapshot_ref)
        if not ref:
            return False
        loaded = self._load_result_snapshot(case_id, ref)
        return _graph_has_payload(_normalize_view_graph_blob(loaded.get("runtime_graph")))

    @staticmethod
    def _result_snapshot_payload_dependency_refs(
        payload: dict[str, Any],
    ) -> tuple[bool, tuple[dict[str, Any], ...]]:
        raw_refs: list[Any] = []
        if str(payload.get("graph_storage_mode") or "").strip().lower() == "delta":
            raw_refs.append(payload.get("graph_base_snapshot_ref"))
        result_payload = payload.get("result") if isinstance(payload.get("result"), dict) else {}
        projection = result_payload.get("projection") if isinstance(result_payload.get("projection"), dict) else {}
        projection_refs = [
            projection[key]
            for key in ("source_result_snapshot_ref", "sourceResultSnapshotRef")
            if key in projection and projection[key] not in (None, {})
        ]
        raw_refs.extend(projection_refs)
        refs: list[dict[str, Any]] = []
        seen: set[str] = set()
        for raw_ref in raw_refs:
            try:
                ref = _normalize_snapshot_ref(raw_ref)
            except (TypeError, ValueError, OverflowError):
                return False, ()
            snapshot_id = str(ref.get("snapshot_id") or "").strip() if ref else ""
            graph_hash = str(ref.get("graph_hash") or "").strip() if ref else ""
            if not _is_sha256_hex(snapshot_id) or graph_hash != snapshot_id:
                return False, ()
            if snapshot_id in seen:
                continue
            seen.add(snapshot_id)
            refs.append({"snapshot_id": snapshot_id, "graph_hash": snapshot_id})
        if len(projection_refs) > 1:
            try:
                normalized_projection_ids = {
                    str((_normalize_snapshot_ref(value) or {}).get("snapshot_id") or "").strip()
                    for value in projection_refs
                }
            except (TypeError, ValueError, OverflowError):
                return False, ()
            if len(normalized_projection_ids) != 1:
                return False, ()
        return True, tuple(refs)

    def _result_snapshot_dependency_closure_is_reconstructable(
        self,
        case_id: str,
        snapshot_ref: Optional[dict[str, Any]],
    ) -> bool:
        try:
            root = _normalize_snapshot_ref(snapshot_ref)
        except (TypeError, ValueError, OverflowError):
            return False
        root_id = str(root.get("snapshot_id") or "").strip() if root else ""
        if not _is_sha256_hex(root_id) or str(root.get("graph_hash") or "").strip() != root_id:
            return False
        states: dict[str, int] = {}
        refs_by_id: dict[str, dict[str, Any]] = {root_id: {"snapshot_id": root_id, "graph_hash": root_id}}
        stack: list[tuple[str, bool, int]] = [(root_id, False, 0)]
        while stack:
            snapshot_id, expanded, depth = stack.pop()
            if depth > _RESULT_SNAPSHOT_DEPENDENCY_MAX_DEPTH:
                return False
            if expanded:
                states[snapshot_id] = 2
                continue
            state = states.get(snapshot_id, 0)
            if state == 1:
                return False
            if state == 2:
                continue
            if len(refs_by_id) > _RESULT_SNAPSHOT_DEPENDENCY_MAX_NODES:
                return False
            validated = self._read_valid_result_snapshot_payload(
                case_id=case_id,
                snapshot_ref=refs_by_id[snapshot_id],
            )
            if validated is None:
                return False
            _, payload = validated
            dependencies_valid, dependencies = self._result_snapshot_payload_dependency_refs(payload)
            if not dependencies_valid:
                return False
            states[snapshot_id] = 1
            stack.append((snapshot_id, True, depth))
            for dependency in reversed(dependencies):
                dependency_id = str(dependency.get("snapshot_id") or "").strip()
                if dependency_id == snapshot_id or states.get(dependency_id, 0) == 1:
                    return False
                refs_by_id.setdefault(dependency_id, dependency)
                if states.get(dependency_id, 0) != 2:
                    stack.append((dependency_id, False, depth + 1))
        return all(
            self._result_snapshot_ref_is_reconstructable(case_id, ref)
            for ref in refs_by_id.values()
        )

    def _result_snapshot_payload_dependencies_are_reconstructable(
        self,
        case_id: str,
        payload: dict[str, Any],
    ) -> bool:
        valid, dependencies = self._result_snapshot_payload_dependency_refs(payload)
        return valid and all(
            self._result_snapshot_dependency_closure_is_reconstructable(case_id, dependency)
            for dependency in dependencies
        )

    def _job_result_manifest_path(self, case_id: str, job_id: str) -> Path:
        normalized_job_id = str(job_id or "").strip()
        try:
            if str(uuid.UUID(normalized_job_id)) != normalized_job_id:
                raise ValueError
        except (AttributeError, TypeError, ValueError):
            raise FlowJobResultManifestError("flow_job_result_job_invalid") from None
        return self._job_result_manifests_dir(case_id) / f"{normalized_job_id}.json"

    @staticmethod
    def _job_result_manifest_text(
        *,
        case_id: str,
        job_id: str,
        snapshot_id: str,
        case_lifecycle_generation: int,
        case_lifecycle_binding_digest: str,
    ) -> str:
        body = {
            "version": _FLOW_JOB_RESULT_MANIFEST_VERSION,
            "case_id": case_id,
            "case_lifecycle_generation": case_lifecycle_generation,
            "case_lifecycle_binding_digest": case_lifecycle_binding_digest,
            "job_id": job_id,
            "result_snapshot_ref": {
                "snapshot_id": snapshot_id,
                "graph_hash": snapshot_id,
            },
        }
        body_text = dumps_canonical_json(body)
        digest = hashlib.sha256(_FLOW_JOB_RESULT_MANIFEST_DOMAIN + body_text.encode("utf-8")).hexdigest()
        return dumps_canonical_json({**body, "manifest_sha256": digest})

    def persist_job_result_manifest(
        self,
        *,
        case_id: str,
        job_id: str,
        snapshot_ref: dict[str, Any],
        case_lifecycle_binding: Optional[FlowCaseLifecycleBindingV1] = None,
    ) -> dict[str, Any]:
        self._job_result_manifest_path(case_id, job_id)
        try:
            binding = (
                self.freeze_case_lifecycle_binding(case_id)
                if case_lifecycle_binding is None
                else self.require_case_lifecycle_binding_current(
                    case_lifecycle_binding,
                    case_id=case_id,
                )
            )
        except FlowResultSnapshotError:
            raise FlowJobResultManifestError("flow_job_result_case_invalid")
        expected_generation = binding.generation
        try:
            with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                    if not self._case_binding_is_publishable(
                        case_id,
                        expected_generation=expected_generation,
                    ):
                        raise FlowJobResultManifestError("flow_job_result_case_invalid")
                    return self._persist_job_result_manifest_locked(
                        case_id=case_id,
                        job_id=job_id,
                        snapshot_ref=snapshot_ref,
                        expected_generation=expected_generation,
                        expected_binding_digest=binding.binding_digest,
                    )
        except OSError:
            raise FlowJobResultManifestError("flow_job_result_manifest_unavailable") from None

    def _persist_job_result_manifest_locked(
        self,
        *,
        case_id: str,
        job_id: str,
        snapshot_ref: dict[str, Any],
        expected_generation: int,
        expected_binding_digest: str,
    ) -> dict[str, Any]:
        path = self._job_result_manifest_path(case_id, job_id)
        normalized_case_id = case_id
        normalized_job_id = path.stem
        ref = _normalize_snapshot_ref(snapshot_ref)
        snapshot_id = str(ref.get("snapshot_id") or "").strip() if ref else ""
        if (
            not snapshot_id
            or len(snapshot_id) != 64
            or any(character not in "0123456789abcdef" for character in snapshot_id)
            or str(ref.get("graph_hash") or "").strip() != snapshot_id
            or not self._result_snapshot_dependency_closure_is_reconstructable(
                case_id=normalized_case_id,
                snapshot_ref={"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
            )
        ):
            raise FlowJobResultManifestError("flow_job_result_snapshot_invalid")
        manifest_text = self._job_result_manifest_text(
            case_id=normalized_case_id,
            job_id=normalized_job_id,
            snapshot_id=snapshot_id,
            case_lifecycle_generation=expected_generation,
            case_lifecycle_binding_digest=expected_binding_digest,
        )
        try:
            created = atomic_create_private_text(path, manifest_text)
            observed = read_private_text(path, max_bytes=_FLOW_JOB_RESULT_MANIFEST_MAX_BYTES)
        except OSError:
            raise FlowJobResultManifestError("flow_job_result_manifest_unavailable") from None
        if observed != manifest_text:
            code = "flow_job_result_manifest_conflict" if not created else "flow_job_result_manifest_invalid"
            raise FlowJobResultManifestError(code)
        manifest = self.get_job_result_manifest(
            case_id=normalized_case_id,
            job_id=normalized_job_id,
            _leases_held=True,
        )
        if manifest is None:
            raise FlowJobResultManifestError("flow_job_result_manifest_invalid")
        return manifest

    def get_job_result_manifest(
        self,
        *,
        case_id: str,
        job_id: str,
        _leases_held: bool = False,
    ) -> Optional[dict[str, Any]]:
        if not _leases_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                        return self.get_job_result_manifest(
                            case_id=case_id,
                            job_id=job_id,
                            _leases_held=True,
                        )
            except (FlowResultSnapshotError, OSError):
                return None
        try:
            path = self._job_result_manifest_path(case_id, job_id)
            text = read_private_text(path, max_bytes=_FLOW_JOB_RESULT_MANIFEST_MAX_BYTES)
            payload = loads_strict_json(text, max_bytes=_FLOW_JOB_RESULT_MANIFEST_MAX_BYTES)
        except (FlowJobResultManifestError, OSError, StrictJSONError):
            return None
        normalized_case_id = case_id
        if not isinstance(payload, dict) or set(payload) != {
            "version",
            "case_id",
            "case_lifecycle_generation",
            "case_lifecycle_binding_digest",
            "job_id",
            "result_snapshot_ref",
            "manifest_sha256",
        }:
            return None
        if (
            payload.get("version") != _FLOW_JOB_RESULT_MANIFEST_VERSION
            or str(payload.get("case_id") or "") != normalized_case_id
            or str(payload.get("job_id") or "") != path.stem
            or not _is_strict_nonnegative_int(payload.get("case_lifecycle_generation"))
            or payload.get("case_lifecycle_generation") < 1
            or payload.get("case_lifecycle_generation")
            != self._case_binding_generation(normalized_case_id)
            or payload.get("case_lifecycle_binding_digest")
            != _flow_case_lifecycle_binding_digest(
                normalized_case_id,
                int(payload["case_lifecycle_generation"]),
            )
        ):
            return None
        ref = payload.get("result_snapshot_ref") if isinstance(payload.get("result_snapshot_ref"), dict) else {}
        if set(ref) != {"snapshot_id", "graph_hash"}:
            return None
        snapshot_id = str(ref.get("snapshot_id") or "").strip()
        if (
            len(snapshot_id) != 64
            or any(character not in "0123456789abcdef" for character in snapshot_id)
            or str(ref.get("graph_hash") or "").strip() != snapshot_id
        ):
            return None
        expected_text = self._job_result_manifest_text(
            case_id=normalized_case_id,
            job_id=path.stem,
            snapshot_id=snapshot_id,
            case_lifecycle_generation=int(payload["case_lifecycle_generation"]),
            case_lifecycle_binding_digest=str(payload["case_lifecycle_binding_digest"]),
        )
        if text != expected_text:
            return None
        if not self._result_snapshot_dependency_closure_is_reconstructable(
            case_id=normalized_case_id,
            snapshot_ref={"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
        ):
            return None
        return {
            "version": _FLOW_JOB_RESULT_MANIFEST_VERSION,
            "case_id": normalized_case_id,
            "case_lifecycle_generation": int(payload["case_lifecycle_generation"]),
            "case_lifecycle_binding_digest": str(payload["case_lifecycle_binding_digest"]),
            "job_id": path.stem,
            "result_snapshot_ref": {
                "snapshot_id": snapshot_id,
                "graph_hash": snapshot_id,
            },
            "manifest_sha256": str(payload.get("manifest_sha256") or ""),
        }

    def has_job_result(self, *, case_id: str, job_id: str) -> bool:
        return self.get_job_result_manifest(case_id=case_id, job_id=job_id) is not None

    def _job_result_manifest_snapshot_ids(self, case_id: str) -> Optional[set[str]]:
        try:
            paths = self._iter_job_result_manifest_paths(case_id)
        except FlowJobResultManifestError:
            return None
        except OSError:
            return None
        snapshot_ids: set[str] = set()
        for path in paths:
            manifest = self.get_job_result_manifest(
                case_id=case_id,
                job_id=path.stem,
                _leases_held=True,
            )
            if manifest is None:
                return None
            ref = manifest.get("result_snapshot_ref") if isinstance(manifest.get("result_snapshot_ref"), dict) else {}
            snapshot_id = str(ref.get("snapshot_id") or "").strip()
            if snapshot_id:
                snapshot_ids.add(snapshot_id)
        return snapshot_ids

    def _iter_job_result_manifest_paths(self, case_id: str) -> list[Path]:
        paths: list[Path] = []
        for path, item in list_private_directory_entries(self._job_result_manifests_dir(case_id)):
            try:
                canonical_job_id = str(uuid.UUID(path.stem))
            except (AttributeError, TypeError, ValueError):
                raise OSError("private_artifact_list_invalid") from None
            if (
                not _is_owner_private_regular_file(item)
                or path.suffix != ".json"
                or canonical_job_id != path.stem
            ):
                raise OSError("private_artifact_list_invalid")
            paths.append(path)
        return sorted(paths, key=lambda path: path.name)

    def _result_snapshot_compute_graph_path(self, case_id: str, snapshot_id: str) -> Path:
        safe_id = safe_fs_name(snapshot_id or "snapshot", "snapshot")
        return self._result_snapshots_dir(case_id) / "_compute" / f"{safe_id}.network-graph.json"

    def _read_valid_result_snapshot_payload(
        self,
        *,
        case_id: str,
        snapshot_ref: Optional[dict[str, Any]],
    ) -> Optional[tuple[Path, dict[str, Any]]]:
        ref = _normalize_snapshot_ref(snapshot_ref)
        if not ref:
            return None
        path = self._result_snapshot_path(case_id, str(ref.get("snapshot_id") or ref.get("graph_hash") or ""))
        if not path.exists():
            return None
        try:
            text = read_private_text(path, max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES)
            payload = loads_strict_json(
                text,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
            )
            if (
                not _valid_result_snapshot_shape(payload)
                or dumps_canonical_json(
                    payload,
                    max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                )
                != text
            ):
                return None
        except (OSError, StrictJSONError):
            return None
        snapshot_id = str(payload.get("snapshot_id") or "").strip()
        graph_hash = str(payload.get("graph_hash") or "").strip()
        ref_id = str(ref.get("snapshot_id") or ref.get("graph_hash") or "").strip()
        ref_snapshot_id = str(ref.get("snapshot_id") or "").strip()
        ref_graph_hash = str(ref.get("graph_hash") or "").strip()
        if (
            not snapshot_id
            or snapshot_id != graph_hash
            or snapshot_id != ref_id
            or snapshot_id != ref_snapshot_id
            or snapshot_id != ref_graph_hash
            or path.stem != snapshot_id
        ):
            return None
        if (
            _result_snapshot_digest(payload) != snapshot_id
            or _result_snapshot_envelope_digest(payload) != payload.get("envelope_sha256")
        ):
            return None
        result_payload = payload.get("result") if isinstance(payload.get("result"), dict) else {}
        candidate = {
            "stats": result_payload.get("stats"),
            "publication_status": result_payload.get("publication_status"),
            "fact_answer_allowed": result_payload.get("fact_answer_allowed"),
        }
        if not _has_complete_candidate_amount_coverage(candidate, expected_case_id=case_id):
            return None
        if payload.get("case_lifecycle_generation") != self._case_binding_generation(case_id):
            return None
        if payload.get("case_lifecycle_binding_digest") != _flow_case_lifecycle_binding_digest(
            case_id,
            int(payload["case_lifecycle_generation"]),
        ):
            return None
        return path, payload

    def _legacy_stats_focus_fact_cache_dir_for_purge(self, case_id: str) -> Path:
        safe_id = safe_fs_name(case_id or "case", "case")
        return Path(self._storage.app_dir) / "flow_stats_focus_cache" / safe_id

    def _iter_result_snapshot_paths(self, case_id: str) -> list[Path]:
        snapshot_dir = self._result_snapshots_dir(case_id)
        paths: list[tuple[Path, os.stat_result]] = []
        for path, item in list_private_directory_entries(snapshot_dir):
            if path.name == "_compute":
                if not _is_owner_private_directory(item):
                    raise OSError("private_artifact_list_invalid")
                continue
            if (
                not _is_owner_private_regular_file(item)
                or path.suffix != ".json"
                or not _is_sha256_hex(path.stem)
            ):
                raise OSError("private_artifact_list_invalid")
            paths.append((path, item))
        return [
            path
            for path, _ in sorted(
                paths,
                key=lambda item: (int(item[1].st_mtime_ns), item[0].name),
                reverse=True,
            )
        ]

    def _iter_result_snapshot_compute_sidecars(
        self,
        case_id: str,
    ) -> list[tuple[Path, os.stat_result]]:
        compute_dir = self._result_snapshots_dir(case_id) / "_compute"
        out: list[tuple[Path, os.stat_result]] = []
        suffix = ".network-graph.json"
        for path, item in list_private_directory_entries(compute_dir):
            snapshot_id = path.name.removesuffix(suffix)
            if (
                not _is_owner_private_regular_file(item)
                or not path.name.endswith(suffix)
                or not _is_sha256_hex(snapshot_id)
            ):
                raise OSError("private_artifact_list_invalid")
            out.append((path, item))
        return sorted(out, key=lambda entry: entry[0].name)

    def _valid_result_snapshot_compute_sidecar(
        self,
        case_id: str,
        path: Path,
        snapshot_id: str,
    ) -> bool:
        validated = self._read_valid_result_snapshot_payload(
            case_id=case_id,
            snapshot_ref={"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
        )
        if validated is None:
            return False
        _, snapshot_payload = validated
        try:
            text = read_private_text(path, max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES)
            payload = loads_strict_json(
                text,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
            )
            return bool(
                isinstance(payload, dict)
                and set(payload)
                == {
                    "version",
                    "case_lifecycle_generation",
                    "case_lifecycle_binding_digest",
                    "snapshot_id",
                    "graph",
                }
                and payload.get("version") == 2
                and payload.get("case_lifecycle_generation")
                == snapshot_payload.get("case_lifecycle_generation")
                and payload.get("case_lifecycle_binding_digest")
                == snapshot_payload.get("case_lifecycle_binding_digest")
                and payload.get("snapshot_id") == snapshot_id
                and _stable_json_text(payload.get("graph"))
                == _stable_json_text(snapshot_payload.get("graph"))
                and dumps_canonical_json(
                    payload,
                    max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                )
                == text
            )
        except (OSError, StrictJSONError):
            return False

    def _listed_result_snapshot_case_ids(self) -> list[str]:
        try:
            cases = self._storage.list_cases(include_deleted=True)
        except Exception:
            raise FlowResultSnapshotError("flow_result_snapshot_case_inventory_unavailable") from None
        return sorted(
            case_id
            for item in cases
            if (case_id := str(getattr(item, "case_id", "") or "").strip())
            and case_id.lower() != "active"
        )

    def _iter_result_snapshot_case_ids(self) -> list[str]:
        case_ids = self._listed_result_snapshot_case_ids()
        try:
            components = {case_bound_storage_name(case_id): case_id for case_id in case_ids}
            if len(components) != len(case_ids):
                raise ValueError
            app_dir = Path(self._storage.app_dir)
            roots = (app_dir / "flow_result_snapshots", app_dir / "flow_job_results")

            def scan_roots() -> tuple[tuple[str, ...], ...]:
                inventories: list[tuple[str, ...]] = []
                for root in roots:
                    names = tuple(sorted(directory.name for directory in list_private_directories(root)))
                    if any(name not in components for name in names):
                        raise ValueError
                    inventories.append(names)
                return tuple(inventories)

            first_inventory = scan_roots()
            if self._listed_result_snapshot_case_ids() != case_ids:
                raise ValueError
            if scan_roots() != first_inventory:
                raise ValueError
        except (OSError, UnicodeError, ValueError):
            raise FlowResultSnapshotError("flow_result_snapshot_case_inventory_unavailable") from None
        return case_ids

    def _purge_legacy_result_snapshot_aliases(self) -> None:
        app_dir = Path(self._storage.app_dir)
        roots = (app_dir / "flow_result_snapshots", app_dir / "flow_job_results")
        views_root = app_dir / "flow_view_metadata_v2"
        for alias in ("active", "case"):
            for root in roots:
                try:
                    remove_path_no_follow_under(app_dir, root / alias)
                except OSError:
                    raise FlowResultSnapshotError("legacy_result_snapshot_purge_failed") from None
            try:
                remove_path_no_follow_under(app_dir, views_root / f"{alias}.json")
            except OSError:
                raise FlowResultSnapshotError("legacy_result_snapshot_purge_failed") from None
        for case_id in self._listed_result_snapshot_case_ids():
            try:
                current_component = case_bound_storage_name(case_id)
                legacy_components = {
                    safe_fs_name(case_id, "case"),
                    legacy_case_bound_storage_name_v1(case_id),
                }
            except (UnicodeError, ValueError):
                continue
            legacy_components.discard(current_component)
            for component in legacy_components:
                for root in roots:
                    try:
                        remove_path_no_follow_under(app_dir, root / component)
                    except OSError:
                        raise FlowResultSnapshotError("legacy_result_snapshot_purge_failed") from None
                try:
                    remove_path_no_follow_under(app_dir, views_root / f"{component}.json")
                except OSError:
                    raise FlowResultSnapshotError("legacy_result_snapshot_purge_failed") from None

    def _load_result_snapshot_gc_meta(self, case_id: str, path: Path) -> Optional[dict[str, Any]]:
        if path != self._result_snapshot_path(case_id, path.stem):
            return None
        try:
            before_stat = path.lstat()
        except OSError:
            return None
        validated = self._read_valid_result_snapshot_payload(
            case_id=case_id,
            snapshot_ref={"snapshot_id": path.stem, "graph_hash": path.stem},
        )
        if validated is None:
            return None
        _, payload = validated
        try:
            path_stat = path.lstat()
        except OSError:
            return None
        if (
            stat.S_ISLNK(path_stat.st_mode)
            or not stat.S_ISREG(path_stat.st_mode)
            or (
                int(before_stat.st_dev),
                int(before_stat.st_ino),
                int(before_stat.st_size),
                int(before_stat.st_mtime_ns),
            )
            != (
                int(path_stat.st_dev),
                int(path_stat.st_ino),
                int(path_stat.st_size),
                int(path_stat.st_mtime_ns),
            )
        ):
            return None
        snapshot_id = str(payload.get("snapshot_id") or "").strip()
        result_payload = payload.get("result") if isinstance(payload.get("result"), dict) else {}
        projection = result_payload.get("projection") if isinstance(result_payload.get("projection"), dict) else {}
        source_ref = _normalize_snapshot_ref(projection.get("source_result_snapshot_ref"))
        graph_base_ref = _normalize_snapshot_ref(payload.get("graph_base_snapshot_ref"))
        return {
            "snapshot_id": snapshot_id,
            "path": path,
            "stored_at": str(payload.get("stored_at") or "").strip(),
            "device": int(path_stat.st_dev),
            "inode": int(path_stat.st_ino),
            "mtime_ns": int(path_stat.st_mtime_ns),
            "size_bytes": max(0, int(path_stat.st_size)),
            "node_count": max(0, int(payload.get("node_count") or 0)),
            "edge_count": max(0, int(payload.get("edge_count") or 0)),
            "graph_storage_mode": str(payload.get("graph_storage_mode") or "full").strip().lower() or "full",
            "graph_storage_chain_depth": max(0, int(payload.get("graph_storage_chain_depth") or 0)),
            "graph_base_snapshot_id": str(graph_base_ref.get("snapshot_id") or graph_base_ref.get("graph_hash") or "").strip()
            if graph_base_ref
            else "",
            "source_snapshot_id": str(source_ref.get("snapshot_id") or source_ref.get("graph_hash") or "").strip()
            if source_ref
            else "",
        }

    def _snapshot_meta_epoch_seconds(self, meta: dict[str, Any]) -> float:
        # GC authority is filesystem metadata captured from the verified
        # private regular file, never an attacker-editable JSON timestamp.
        mtime_ns = int(meta.get("mtime_ns") or 0)
        return max(0.0, float(mtime_ns) / 1_000_000_000.0)

    def _purge_legacy_stats_focus_fact_cache(self, case_id: str) -> None:
        cache_dir = self._legacy_stats_focus_fact_cache_dir_for_purge(case_id)
        try:
            remove_path_no_follow_under(self._storage.app_dir, cache_dir)
        except OSError:
            raise FlowResultSnapshotError("legacy_stats_focus_fact_cache_purge_failed") from None

    def _purge_all_legacy_stats_focus_fact_caches(self) -> None:
        root = Path(self._storage.app_dir) / "flow_stats_focus_cache"
        try:
            remove_path_no_follow_under(self._storage.app_dir, root)
        except OSError:
            raise FlowResultSnapshotError("legacy_stats_focus_fact_cache_purge_failed") from None

    def _collect_result_snapshot_dependency_ids(
        self,
        case_id: str,
        *,
        seed_ids: Sequence[str],
        meta_by_id: Optional[dict[str, dict[str, Any]]] = None,
    ) -> set[str]:
        out: set[str] = set()
        pending = [str(item or "").strip() for item in seed_ids if str(item or "").strip()]
        meta_index = meta_by_id if isinstance(meta_by_id, dict) else {}
        while pending:
            snapshot_id = pending.pop()
            if not snapshot_id or snapshot_id in out:
                continue
            out.add(snapshot_id)
            meta = meta_index.get(snapshot_id)
            if meta is None:
                path = self._result_snapshot_path(case_id, snapshot_id)
                if not path.exists():
                    continue
                meta = self._load_result_snapshot_gc_meta(case_id, path)
                if not meta:
                    continue
                meta_index[snapshot_id] = meta
            source_snapshot_id = str(meta.get("source_snapshot_id") or "").strip()
            if source_snapshot_id and source_snapshot_id not in out:
                pending.append(source_snapshot_id)
            graph_base_snapshot_id = str(meta.get("graph_base_snapshot_id") or "").strip()
            if graph_base_snapshot_id and graph_base_snapshot_id not in out:
                pending.append(graph_base_snapshot_id)
        return out

    @staticmethod
    def _result_snapshot_dependency_ids(meta: dict[str, Any]) -> tuple[str, ...]:
        return tuple(
            dependency_id
            for dependency_id in (
                str(meta.get("graph_base_snapshot_id") or "").strip(),
                str(meta.get("source_snapshot_id") or "").strip(),
            )
            if dependency_id
        )

    def _validate_result_snapshot_dependency_inventory(
        self,
        case_id: str,
        metas: Sequence[dict[str, Any]],
    ) -> tuple[bool, dict[str, int]]:
        meta_by_id: dict[str, dict[str, Any]] = {}
        for meta in metas:
            snapshot_id = str(meta.get("snapshot_id") or "").strip()
            if not _is_sha256_hex(snapshot_id) or snapshot_id in meta_by_id:
                return False, {}
            meta_by_id[snapshot_id] = meta
        if len(meta_by_id) > _RESULT_SNAPSHOT_DEPENDENCY_MAX_NODES:
            return False, {}

        states: dict[str, int] = {}
        dependency_depths: dict[str, int] = {}

        for snapshot_id, meta in meta_by_id.items():
            storage_mode = str(meta.get("graph_storage_mode") or "").strip().lower()
            graph_base_id = str(meta.get("graph_base_snapshot_id") or "").strip()
            chain_depth = _to_optional_int(meta.get("graph_storage_chain_depth"))
            if chain_depth is None or chain_depth < 0:
                return False, {}
            if storage_mode == "delta":
                base_meta = meta_by_id.get(graph_base_id)
                base_chain_depth = (
                    _to_optional_int(base_meta.get("graph_storage_chain_depth"))
                    if isinstance(base_meta, dict)
                    else None
                )
                if (
                    base_meta is None
                    or base_chain_depth is None
                    or base_chain_depth < 0
                    or chain_depth != base_chain_depth + 1
                ):
                    return False, {}
            elif storage_mode != "full" or graph_base_id or chain_depth != 0:
                return False, {}

        for root_snapshot_id in meta_by_id:
            if states.get(root_snapshot_id, 0) == 2:
                continue
            states[root_snapshot_id] = 1
            stack: list[tuple[str, int, tuple[str, ...]]] = [
                (
                    root_snapshot_id,
                    0,
                    self._result_snapshot_dependency_ids(meta_by_id[root_snapshot_id]),
                )
            ]
            while stack:
                if len(stack) > _RESULT_SNAPSHOT_DEPENDENCY_MAX_DEPTH + 1:
                    return False, {}
                current_id, next_index, dependencies = stack[-1]
                if next_index < len(dependencies):
                    dependency_id = dependencies[next_index]
                    stack[-1] = (current_id, next_index + 1, dependencies)
                    if dependency_id == current_id or dependency_id not in meta_by_id:
                        return False, {}
                    dependency_state = states.get(dependency_id, 0)
                    if dependency_state == 1:
                        return False, {}
                    if dependency_state == 2:
                        continue
                    states[dependency_id] = 1
                    stack.append(
                        (
                            dependency_id,
                            0,
                            self._result_snapshot_dependency_ids(meta_by_id[dependency_id]),
                        )
                    )
                    continue
                max_dependency_depth = max(
                    (dependency_depths[dependency_id] for dependency_id in dependencies),
                    default=-1,
                )
                dependency_depth = max_dependency_depth + 1
                if dependency_depth > _RESULT_SNAPSHOT_DEPENDENCY_MAX_DEPTH:
                    return False, {}
                dependency_depths[current_id] = dependency_depth
                states[current_id] = 2
                stack.pop()

        for snapshot_id, meta in meta_by_id.items():
            loaded = self._load_result_snapshot(
                case_id,
                {"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
            )
            graph = _normalize_view_graph_blob(loaded.get("runtime_graph"))
            if (
                not _graph_has_payload(graph)
                or len(graph.get("nodes") or []) != int(meta.get("node_count") or 0)
                or len(graph.get("edges") or []) != int(meta.get("edge_count") or 0)
            ):
                return False, {}
        return True, dependency_depths

    def _gc_case_result_snapshots(
        self,
        case_id: str,
        *,
        soft_limit: int = _RESULT_SNAPSHOT_GC_SOFT_LIMIT,
        keep_latest: int = _RESULT_SNAPSHOT_GC_KEEP_LATEST,
        ttl_seconds: int = _RESULT_SNAPSHOT_GC_TTL_SECONDS,
        _inventory_lease_held: bool = False,
    ) -> dict[str, Any]:
        if not _inventory_lease_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    return self._gc_case_result_snapshots(
                        case_id,
                        soft_limit=soft_limit,
                        keep_latest=keep_latest,
                        ttl_seconds=ttl_seconds,
                        _inventory_lease_held=True,
                    )
            except OSError as exc:
                return {
                    "case_id": str(case_id or "").strip(),
                    "inventory_status": "unknown",
                    "before_count": None,
                    "after_count": None,
                    "deleted_count": 0,
                    "before_bytes": None,
                    "after_bytes": None,
                    "deleted_bytes": 0,
                    "gc_ran": False,
                    "blocked_reason": str(exc or "").strip() or "snapshot_inventory_lease_unavailable",
                    "soft_limit": max(0, int(soft_limit or 0)),
                    "keep_latest": max(0, int(keep_latest or 0)),
                    "ttl_seconds": max(0, int(ttl_seconds or 0)),
                }
        try:
            with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                if not self._case_binding_is_publishable(case_id):
                    raise FlowResultSnapshotError("snapshot_case_binding_unavailable")
                return self._gc_case_result_snapshots_locked(
                    case_id,
                    soft_limit=soft_limit,
                    keep_latest=keep_latest,
                    ttl_seconds=ttl_seconds,
                )
        except (FlowResultSnapshotError, OSError) as exc:
            blocked_reason = str(exc or "").strip() or "snapshot_case_lease_unavailable"
            return {
                "case_id": str(case_id or "").strip(),
                "inventory_status": "unknown",
                "before_count": None,
                "after_count": None,
                "deleted_count": 0,
                "before_bytes": None,
                "after_bytes": None,
                "deleted_bytes": 0,
                "gc_ran": False,
                "blocked_reason": blocked_reason,
                "soft_limit": max(0, int(soft_limit or 0)),
                "keep_latest": max(0, int(keep_latest or 0)),
                "ttl_seconds": max(0, int(ttl_seconds or 0)),
            }

    def _gc_case_result_snapshots_locked(
        self,
        case_id: str,
        *,
        soft_limit: int = _RESULT_SNAPSHOT_GC_SOFT_LIMIT,
        keep_latest: int = _RESULT_SNAPSHOT_GC_KEEP_LATEST,
        ttl_seconds: int = _RESULT_SNAPSHOT_GC_TTL_SECONDS,
    ) -> dict[str, Any]:
        snapshot_dir = self._result_snapshots_dir(case_id)
        summary = {
            "case_id": str(case_id or "").strip(),
            "inventory_status": "valid",
            "before_count": 0,
            "after_count": 0,
            "deleted_count": 0,
            "before_bytes": 0,
            "after_bytes": 0,
            "deleted_bytes": 0,
            "gc_ran": False,
            "blocked_reason": "",
            "soft_limit": max(0, int(soft_limit or 0)),
            "keep_latest": max(0, int(keep_latest or 0)),
            "ttl_seconds": max(0, int(ttl_seconds or 0)),
        }

        def block_inventory(reason: str) -> dict[str, Any]:
            summary["inventory_status"] = "unknown"
            summary["before_count"] = None
            summary["after_count"] = None
            summary["before_bytes"] = None
            summary["after_bytes"] = None
            summary["blocked_reason"] = reason
            return summary

        self._purge_legacy_stats_focus_fact_cache(case_id)
        try:
            paths = self._iter_result_snapshot_paths(case_id)
            sidecar_inventory = self._iter_result_snapshot_compute_sidecars(case_id)
        except OSError:
            return block_inventory("invalid_snapshot_inventory")
        summary["before_count"] = len(paths)

        metas: list[dict[str, Any]] = []
        meta_by_id: dict[str, dict[str, Any]] = {}
        invalid_snapshot_inventory = False
        for path in paths:
            meta = self._load_result_snapshot_gc_meta(case_id, path)
            if meta is None:
                invalid_snapshot_inventory = True
                continue
            snapshot_id = str(meta.get("snapshot_id") or "").strip()
            if not snapshot_id or snapshot_id in meta_by_id:
                invalid_snapshot_inventory = True
                continue
            metas.append(meta)
            meta_by_id[snapshot_id] = meta
            summary["before_bytes"] += max(0, int(meta.get("size_bytes") or 0))
        if invalid_snapshot_inventory or len(metas) != len(paths):
            return block_inventory("invalid_snapshot_inventory")

        manifest_snapshot_ids = self._job_result_manifest_snapshot_ids(case_id)
        if manifest_snapshot_ids is None or any(
            snapshot_id not in meta_by_id for snapshot_id in manifest_snapshot_ids
        ):
            return block_inventory("invalid_job_result_manifest_inventory")
        dependency_inventory_valid, dependency_depths = self._validate_result_snapshot_dependency_inventory(
            case_id,
            metas,
        )
        if not dependency_inventory_valid:
            return block_inventory("invalid_snapshot_dependency_inventory")
        sidecar_by_snapshot_id: dict[str, tuple[Path, os.stat_result]] = {}
        for sidecar_path, sidecar_stat in sidecar_inventory:
            source_snapshot_id = sidecar_path.name.removesuffix(".network-graph.json")
            if (
                source_snapshot_id not in meta_by_id
                or source_snapshot_id in sidecar_by_snapshot_id
                or not self._valid_result_snapshot_compute_sidecar(
                    case_id,
                    sidecar_path,
                    source_snapshot_id,
                )
            ):
                return block_inventory("invalid_compute_sidecar_inventory")
            sidecar_by_snapshot_id[source_snapshot_id] = (sidecar_path, sidecar_stat)

        if len(metas) <= max(soft_limit, keep_latest):
            summary["after_bytes"] = summary["before_bytes"]
            summary["after_count"] = len(metas)
            return summary

        metas.sort(
            key=lambda meta: (
                self._snapshot_meta_epoch_seconds(meta),
                int(meta.get("mtime_ns") or 0),
                str(meta.get("snapshot_id") or ""),
            ),
            reverse=True,
        )

        keep_ids: set[str] = {
            str(meta.get("snapshot_id") or "").strip() for meta in metas[: max(1, int(keep_latest or 1))]
        }
        keep_ids.update(manifest_snapshot_ids)
        ttl_cutoff = utc_now().timestamp() - max(0, int(ttl_seconds or 0))
        for meta in metas:
            snapshot_id = str(meta.get("snapshot_id") or "").strip()
            if not snapshot_id:
                continue
            if self._snapshot_meta_epoch_seconds(meta) >= ttl_cutoff:
                keep_ids.add(snapshot_id)

        keep_ids = self._collect_result_snapshot_dependency_ids(
            case_id,
            seed_ids=list(keep_ids),
            meta_by_id=meta_by_id,
        )

        try:
            current_paths = self._iter_result_snapshot_paths(case_id)
            current_sidecar_inventory = self._iter_result_snapshot_compute_sidecars(case_id)
        except OSError:
            return block_inventory("invalid_snapshot_inventory")
        current_metas = [self._load_result_snapshot_gc_meta(case_id, path) for path in current_paths]
        if any(meta is None for meta in current_metas):
            return block_inventory("snapshot_inventory_changed")
        original_generation = {
            (
                str(meta.get("snapshot_id") or ""),
                int(meta.get("device") or 0),
                int(meta.get("inode") or 0),
                int(meta.get("size_bytes") or 0),
                int(meta.get("mtime_ns") or 0),
            )
            for meta in metas
        }
        current_generation = {
            (
                str(meta.get("snapshot_id") or ""),
                int(meta.get("device") or 0),
                int(meta.get("inode") or 0),
                int(meta.get("size_bytes") or 0),
                int(meta.get("mtime_ns") or 0),
            )
            for meta in current_metas
            if isinstance(meta, dict)
        }
        if original_generation != current_generation:
            return block_inventory("snapshot_inventory_changed")
        original_sidecar_generation = {
            (
                path.name,
                int(item.st_dev),
                int(item.st_ino),
                int(item.st_size),
                int(item.st_mtime_ns),
            )
            for path, item in sidecar_inventory
        }
        current_sidecar_generation = {
            (
                path.name,
                int(item.st_dev),
                int(item.st_ino),
                int(item.st_size),
                int(item.st_mtime_ns),
            )
            for path, item in current_sidecar_inventory
        }
        if original_sidecar_generation != current_sidecar_generation:
            return block_inventory("compute_sidecar_inventory_changed")
        current_manifest_snapshot_ids = self._job_result_manifest_snapshot_ids(case_id)
        if current_manifest_snapshot_ids is None or current_manifest_snapshot_ids != manifest_snapshot_ids:
            return block_inventory("job_result_manifest_inventory_changed")
        summary["gc_ran"] = True
        candidates = [
            meta
            for meta in metas
            if str(meta.get("snapshot_id") or "").strip() not in keep_ids
        ]
        candidates.sort(
            key=lambda meta: (
                dependency_depths.get(str(meta.get("snapshot_id") or "").strip(), 0),
                -self._snapshot_meta_epoch_seconds(meta),
                str(meta.get("snapshot_id") or ""),
            ),
            reverse=True,
        )
        for meta in candidates:
            snapshot_id = str(meta.get("snapshot_id") or "").strip()
            path = meta.get("path")
            if not snapshot_id or not isinstance(path, Path):
                continue
            if self._job_result_manifest_snapshot_ids(case_id) != manifest_snapshot_ids:
                block_inventory("job_result_manifest_inventory_changed")
                break
            sidecar = sidecar_by_snapshot_id.get(snapshot_id)
            if sidecar is not None:
                sidecar_path, sidecar_stat = sidecar
                try:
                    sidecar_deleted = remove_private_regular_file_under_if_matches(
                        self._storage.app_dir,
                        sidecar_path,
                        device=int(sidecar_stat.st_dev),
                        inode=int(sidecar_stat.st_ino),
                        size=int(sidecar_stat.st_size),
                        mtime_ns=int(sidecar_stat.st_mtime_ns),
                    )
                except OSError:
                    block_inventory("sidecar_delete_failed")
                    break
                if not sidecar_deleted:
                    block_inventory("compute_sidecar_inventory_changed")
                    break
            try:
                deleted = remove_private_regular_file_under_if_matches(
                    self._storage.app_dir,
                    path,
                    device=int(meta.get("device") or 0),
                    inode=int(meta.get("inode") or 0),
                    size=int(meta.get("size_bytes") or 0),
                    mtime_ns=int(meta.get("mtime_ns") or 0),
                )
            except OSError:
                block_inventory("snapshot_delete_failed")
                break
            if not deleted:
                block_inventory("snapshot_inventory_changed")
                break
            summary["deleted_count"] += 1
            summary["deleted_bytes"] += max(0, int(meta.get("size_bytes") or 0))

        try:
            snapshot_dir.rmdir()
        except OSError:
            pass
        if summary["inventory_status"] == "valid":
            summary["after_count"] = max(0, len(metas) - int(summary["deleted_count"] or 0))
            summary["after_bytes"] = max(
                0,
                int(summary["before_bytes"] or 0) - int(summary["deleted_bytes"] or 0),
            )
        return summary

    def _persist_result_snapshot(
        self,
        case_id: str,
        result: dict[str, Any],
        *,
        base_snapshot_ref: Optional[dict[str, Any]] = None,
        return_storage_metadata: bool = False,
        compact_result_entities: bool = False,
        precomputed_graph_patch: Optional[dict[str, Any]] = None,
        case_lifecycle_binding: Optional[FlowCaseLifecycleBindingV1] = None,
    ) -> Optional[dict[str, Any]] | tuple[Optional[dict[str, Any]], dict[str, Any]]:
        try:
            binding = (
                self.freeze_case_lifecycle_binding(case_id)
                if case_lifecycle_binding is None
                else self.require_case_lifecycle_binding_current(
                    case_lifecycle_binding,
                    case_id=case_id,
                )
            )
        except FlowResultSnapshotError:
            if return_storage_metadata:
                return None, {}
            return None
        expected_generation = binding.generation
        if not _has_complete_candidate_amount_coverage(result, expected_case_id=case_id):
            if return_storage_metadata:
                return None, {}
            return None
        try:
            dumps_canonical_json(
                result,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
            )
        except StrictJSONError:
            if return_storage_metadata:
                return None, {}
            return None
        runtime_graph = _normalize_view_graph_blob(result.get("runtime_graph"))
        if not _graph_has_payload(runtime_graph):
            if return_storage_metadata:
                return None, {}
            return None
        normalized_base_ref = _normalize_snapshot_ref(base_snapshot_ref)
        graph_storage_mode = "full"
        graph_storage_chain_depth = 0
        stored_graph = runtime_graph
        stored_graph_patch: Optional[dict[str, Any]] = None
        if normalized_base_ref:
            try:
                base_chain_depth = 0
                validated_base = self._read_valid_result_snapshot_payload(
                    case_id=case_id,
                    snapshot_ref=normalized_base_ref,
                )
                if validated_base is not None:
                    _, base_payload = validated_base
                    base_chain_depth = max(0, int(base_payload.get("graph_storage_chain_depth") or 0))
                if base_chain_depth < _RESULT_SNAPSHOT_DELTA_MAX_CHAIN_DEPTH:
                    graph_patch = None
                    base_result = self._load_result_snapshot(case_id, normalized_base_ref)
                    base_graph = _normalize_view_graph_blob(base_result.get("runtime_graph"))
                    if _graph_has_payload(base_graph):
                        graph_patch = _validated_runtime_graph_patch(
                            base_graph,
                            runtime_graph,
                            precomputed_graph_patch,
                        )
                    if isinstance(graph_patch, dict):
                        patch_summary = graph_patch.get("summary") if isinstance(graph_patch, dict) else {}
                        op_count = max(0, int(patch_summary.get("op_count") or 0))
                        target_entity_count = max(0, int(patch_summary.get("target_entity_count") or 0))
                        use_delta_storage = (
                            op_count > 0
                            and target_entity_count > 0
                            and op_count <= max(256, int(target_entity_count * 0.05))
                        )
                        if not use_delta_storage:
                            full_bytes = len(_stable_json_text(runtime_graph).encode("utf-8"))
                            delta_bytes = len(
                                _stable_json_text({
                                    "base_snapshot_ref": normalized_base_ref,
                                    "graph_patch": graph_patch,
                                }).encode("utf-8")
                            )
                            use_delta_storage = (
                                op_count > 0
                                and target_entity_count > 0
                                and delta_bytes < int(full_bytes * 0.88)
                            )
                        if use_delta_storage:
                            graph_storage_mode = "delta"
                            graph_storage_chain_depth = base_chain_depth + 1
                            target_runtime_revision = _to_optional_int(runtime_graph.get("runtime_revision"))
                            stored_graph = {
                                "nodes": [],
                                "edges": [],
                                **(
                                    {"runtime_revision": target_runtime_revision}
                                    if target_runtime_revision is not None and target_runtime_revision >= 0
                                    else {}
                                ),
                            }
                            stored_graph_patch = graph_patch
            except Exception:
                graph_storage_mode = "full"
                graph_storage_chain_depth = 0
                stored_graph = runtime_graph
                stored_graph_patch = None
        stored_result_nodes = _normalize_graph_entities(result.get("nodes"))
        stored_result_edges = _normalize_graph_entities(result.get("edges"))
        if compact_result_entities and graph_storage_mode == "delta":
            stored_result_nodes = []
            stored_result_edges = []
        payload = {
            "version": _FLOW_RESULT_SNAPSHOT_VERSION,
            "case_lifecycle_generation": expected_generation,
            "case_lifecycle_binding_digest": binding.binding_digest,
            "node_count": len(runtime_graph["nodes"]),
            "edge_count": len(runtime_graph["edges"]),
            "graph": stored_graph,
            "result": {
                "nodes": stored_result_nodes,
                "edges": stored_result_edges,
                "stats": _clone_json(result.get("stats"), {}) if isinstance(result.get("stats"), dict) else {},
                "publication_status": "blocked",
                "fact_answer_allowed": False,
                "graph_tier": str(result.get("graph_tier") or ""),
                "render_hints": (
                    _clone_json(result.get("render_hints"), {})
                    if isinstance(result.get("render_hints"), dict)
                    else {}
                ),
                "projection": (
                    _clone_json(result.get("projection"), {})
                    if isinstance(result.get("projection"), dict)
                    else {}
                ),
            },
        }
        if graph_storage_mode == "delta" and stored_graph_patch is not None and normalized_base_ref:
            payload["graph_patch"] = stored_graph_patch
            payload["graph_base_snapshot_ref"] = {
                "snapshot_id": str(normalized_base_ref.get("snapshot_id") or ""),
                "graph_hash": str(normalized_base_ref.get("graph_hash") or ""),
            }
        payload["graph_storage_mode"] = graph_storage_mode
        payload["graph_storage_chain_depth"] = graph_storage_chain_depth
        try:
            snapshot_id = _result_snapshot_digest(payload)
        except StrictJSONError:
            if return_storage_metadata:
                return None, {}
            return None
        path = self._result_snapshot_path(case_id, snapshot_id)
        stored_at = utc_now().isoformat()
        stored_payload = {
            "snapshot_id": snapshot_id,
            "graph_hash": snapshot_id,
            "stored_at": stored_at,
            **payload,
        }
        try:
            stored_payload["envelope_sha256"] = _result_snapshot_envelope_digest(stored_payload)
            snapshot_text = dumps_canonical_json(
                stored_payload,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
            )
        except StrictJSONError:
            if return_storage_metadata:
                return None, {}
            return None
        try:
            with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                    if not self._case_binding_is_publishable(
                        case_id,
                        expected_generation=expected_generation,
                    ):
                        if return_storage_metadata:
                            return None, {}
                        return None
                    if not self._result_snapshot_payload_dependencies_are_reconstructable(
                        case_id,
                        stored_payload,
                    ):
                        if return_storage_metadata:
                            return None, {}
                        return None
                    created_new_snapshot = atomic_create_private_text(path, snapshot_text)
                    validated = self._read_valid_result_snapshot_payload(
                        case_id=case_id,
                        snapshot_ref={"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
                    )
                    if validated is None:
                        if return_storage_metadata:
                            return None, {}
                        return None
        except (FlowResultSnapshotError, OSError):
            if return_storage_metadata:
                return None, {}
            return None
        if not created_new_snapshot:
            _, existing = validated
            stored_at = str(existing.get("stored_at") or stored_at)
        if created_new_snapshot:
            try:
                self._gc_case_result_snapshots(case_id)
            except Exception:
                pass
        snapshot_ref = {
            "snapshot_id": snapshot_id,
            "graph_hash": snapshot_id,
            "node_count": len(runtime_graph["nodes"]),
            "edge_count": len(runtime_graph["edges"]),
            "stored_at": stored_at,
            "graph_storage_mode": graph_storage_mode,
        }
        if return_storage_metadata:
            return snapshot_ref, {
                "graph_storage_mode": graph_storage_mode,
                "graph_storage_chain_depth": graph_storage_chain_depth,
                "graph_base_snapshot_ref": normalized_base_ref,
                "graph_patch": stored_graph_patch,
            }
        return snapshot_ref

    def update_result_snapshot_projection_layout(
        self,
        *,
        case_id: str,
        snapshot_ref: dict[str, Any],
        layout_index: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        ref = _normalize_snapshot_ref(snapshot_ref)
        summary = {
            "snapshot_ref": ref,
            "updated_node_count": 0,
            "layout_index_summary": {},
            "layout_sync_signature": "",
            "layout_sync_skipped": False,
        }
        if not ref:
            return summary
        expected_generation = self._case_binding_generation(case_id)
        if expected_generation is None:
            return summary

        validated = self._read_valid_result_snapshot_payload(case_id=case_id, snapshot_ref=ref)
        if validated is None:
            return summary
        _, stored_payload = validated

        graph_payload = stored_payload.get("graph") if isinstance(stored_payload.get("graph"), dict) else {}
        graph_nodes = _normalize_graph_entities(graph_payload.get("nodes"))
        result_payload = stored_payload.get("result") if isinstance(stored_payload.get("result"), dict) else {}
        if not _has_complete_candidate_amount_coverage(
            {
                "stats": result_payload.get("stats"),
                "publication_status": result_payload.get("publication_status"),
                "fact_answer_allowed": result_payload.get("fact_answer_allowed"),
            },
            expected_case_id=case_id,
        ):
            return summary
        projection = result_payload.get("projection") if isinstance(result_payload.get("projection"), dict) else {}
        existing_layout_summary = (
            projection.get("layout_index_summary")
            if isinstance(projection.get("layout_index_summary"), dict)
            else {}
        )
        existing_signature = str(existing_layout_summary.get("signature") or "").strip()

        projected_layout = project_flow_projection_layout_sync(
            layout_index={
                **(layout_index if isinstance(layout_index, dict) else {}),
                "snapshot_id": str(ref.get("snapshot_id") or ref.get("graph_hash") or ""),
            },
            current_nodes=graph_nodes,
        )
        layout_by_id = (
            projected_layout.get("layout_by_id")
            if isinstance(projected_layout.get("layout_by_id"), dict)
            else {}
        )
        node_updates = (
            projected_layout.get("node_updates")
            if isinstance(projected_layout.get("node_updates"), list)
            else []
        )
        layout_index_summary = (
            projected_layout.get("layout_index_summary")
            if isinstance(projected_layout.get("layout_index_summary"), dict)
            else {}
        )
        layout_sync_signature = str(
            projected_layout.get("signature") or layout_index_summary.get("signature") or ""
        ).strip()
        normalized_viewport = (
            layout_index_summary.get("viewport")
            if isinstance(layout_index_summary.get("viewport"), dict)
            else {}
        )
        existing_viewport = (
            existing_layout_summary.get("viewport")
            if isinstance(existing_layout_summary.get("viewport"), dict)
            else {}
        )

        if not layout_by_id and not normalized_viewport:
            return summary

        if layout_sync_signature and layout_sync_signature == existing_signature and not node_updates:
            summary["layout_index_summary"] = existing_layout_summary
            summary["layout_sync_signature"] = layout_sync_signature
            summary["layout_sync_skipped"] = True
            return summary

        updated_node_count = 0
        for update in node_updates:
            if not isinstance(update, dict):
                continue
            node_index = _to_optional_int(update.get("index"))
            node_id = str(update.get("id") or "").strip()
            if node_index is None or node_index < 0 or node_index >= len(graph_nodes) or not node_id:
                continue
            row = graph_nodes[node_index]
            if str(row.get("id") or "").strip() != node_id:
                continue
            x = _to_optional_float(update.get("x"))
            y = _to_optional_float(update.get("y"))
            if x is None or y is None:
                continue
            projected_fields: dict[str, Any] = {
                "x": float(x),
                "y": float(y),
                "cluster_node": bool(update.get("cluster_node")),
            }
            if update.get("projection_cluster_id"):
                projected_fields["projection_cluster_id"] = str(update["projection_cluster_id"])
            if update.get("nodeRenderMode"):
                projected_fields["nodeRenderMode"] = str(update["nodeRenderMode"])
            if update.get("projection_visible") is not None:
                projected_fields["projection_visible"] = bool(update.get("projection_visible"))
            if update.get("projection_collapsed") is not None:
                projected_fields["projection_collapsed"] = bool(update.get("projection_collapsed"))
            if all(row.get(key) == value for key, value in projected_fields.items()):
                continue
            row.update(projected_fields)
            updated_node_count += 1

        if updated_node_count == 0 and normalized_viewport == existing_viewport:
            summary["layout_index_summary"] = existing_layout_summary
            summary["layout_sync_signature"] = existing_signature or layout_sync_signature
            summary["layout_sync_skipped"] = True
            return summary

        runtime_revision = _to_optional_int(
            graph_payload.get("runtime_revision") or graph_payload.get("runtimeRevision")
        )
        stored_payload["graph"] = {
            **graph_payload,
            "nodes": graph_nodes,
            "edges": _normalize_graph_entities(graph_payload.get("edges")),
            **({"runtime_revision": runtime_revision} if runtime_revision is not None else {}),
        }
        layout_index_summary = {
            **layout_index_summary,
            "version": int(layout_index_summary.get("version") or 1),
            "synced_at": utc_now().isoformat(),
            "updated_node_count": updated_node_count,
        }
        projection["layout_index_summary"] = layout_index_summary
        result_payload["projection"] = projection
        stored_payload["result"] = result_payload
        try:
            snapshot_id = _result_snapshot_digest(stored_payload)
        except StrictJSONError:
            return summary
        original_snapshot_id = str(ref.get("snapshot_id") or ref.get("graph_hash") or "").strip()
        if snapshot_id == original_snapshot_id:
            summary["layout_index_summary"] = layout_index_summary
            summary["layout_sync_signature"] = layout_sync_signature
            summary["layout_sync_skipped"] = True
            return summary
        stored_at = utc_now().isoformat()
        stored_payload["snapshot_id"] = snapshot_id
        stored_payload["graph_hash"] = snapshot_id
        stored_payload["stored_at"] = stored_at
        target_path = self._result_snapshot_path(case_id, snapshot_id)
        try:
            stored_payload["envelope_sha256"] = _result_snapshot_envelope_digest(stored_payload)
            snapshot_text = dumps_canonical_json(
                stored_payload,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
            )
        except StrictJSONError:
            return summary
        try:
            with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                if not self._case_binding_is_publishable(
                    case_id,
                    expected_generation=expected_generation,
                ):
                    return summary
                if not self._result_snapshot_ref_is_reconstructable(case_id, ref):
                    return summary
                if not self._result_snapshot_payload_dependencies_are_reconstructable(
                    case_id,
                    stored_payload,
                ):
                    return summary
                created_new_snapshot = atomic_create_private_text(target_path, snapshot_text)
                persisted = self._read_valid_result_snapshot_payload(
                    case_id=case_id,
                    snapshot_ref={"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
                )
                if persisted is None:
                    return summary
        except (FlowResultSnapshotError, OSError):
            return summary
        _, persisted_payload = persisted
        stored_at = str(persisted_payload.get("stored_at") or stored_at)
        if created_new_snapshot:
            try:
                self._gc_case_result_snapshots(case_id)
            except Exception:
                pass
        summary["snapshot_ref"] = {
            "snapshot_id": snapshot_id,
            "graph_hash": snapshot_id,
            "node_count": max(0, int(persisted_payload.get("node_count") or 0)),
            "edge_count": max(0, int(persisted_payload.get("edge_count") or 0)),
            "stored_at": stored_at,
            "graph_storage_mode": str(persisted_payload.get("graph_storage_mode") or "full"),
        }
        summary["updated_node_count"] = updated_node_count
        summary["layout_index_summary"] = layout_index_summary
        summary["layout_sync_signature"] = layout_sync_signature
        return summary

    def collect_result_snapshot_metrics(
        self,
        *,
        case_id: Optional[str] = None,
        _inventory_lease_held: bool = False,
    ) -> dict[str, Any]:
        if not _inventory_lease_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    return self.collect_result_snapshot_metrics(
                        case_id=case_id,
                        _inventory_lease_held=True,
                    )
            except OSError:
                return {
                    "inventory_status": "unknown",
                    "blocked_reasons": ["inventory_lease_unavailable"],
                    "case_count": None,
                    "snapshot_count": None,
                    "total_bytes": None,
                    "delta_snapshot_count": None,
                    "full_snapshot_count": None,
                    "max_chain_depth": None,
                    "gc_policy": {
                        "soft_limit": _RESULT_SNAPSHOT_GC_SOFT_LIMIT,
                        "keep_latest": _RESULT_SNAPSHOT_GC_KEEP_LATEST,
                        "ttl_seconds": _RESULT_SNAPSHOT_GC_TTL_SECONDS,
                    },
                    "cases": [],
                }
        requested_case_id = str(case_id or "").strip()
        try:
            target_case_ids = [requested_case_id] if requested_case_id else self._iter_result_snapshot_case_ids()
        except FlowResultSnapshotError:
            return {
                "inventory_status": "unknown",
                "blocked_reasons": ["case_inventory_unavailable"],
                "case_count": None,
                "snapshot_count": None,
                "total_bytes": None,
                "delta_snapshot_count": None,
                "full_snapshot_count": None,
                "max_chain_depth": None,
                "gc_policy": {
                    "soft_limit": _RESULT_SNAPSHOT_GC_SOFT_LIMIT,
                    "keep_latest": _RESULT_SNAPSHOT_GC_KEEP_LATEST,
                    "ttl_seconds": _RESULT_SNAPSHOT_GC_TTL_SECONDS,
                },
                "cases": [],
            }
        cases: list[dict[str, Any]] = []
        total_snapshot_count = 0
        total_bytes = 0
        delta_snapshot_count = 0
        max_chain_depth = 0
        inventory_status = "valid"
        blocked_reasons: list[str] = []
        for target_case_id in target_case_ids:
            try:
                with private_exclusive_file_lock(self._result_snapshot_case_lock_path(target_case_id)):
                    if not self._case_binding_is_publishable(target_case_id):
                        raise FlowResultSnapshotError("snapshot_case_binding_unavailable")
                    paths = self._iter_result_snapshot_paths(target_case_id)
                    sidecar_inventory = self._iter_result_snapshot_compute_sidecars(target_case_id)
                    metas = [self._load_result_snapshot_gc_meta(target_case_id, path) for path in paths]
                    valid_metas = [meta for meta in metas if isinstance(meta, dict)]
                    if len(valid_metas) != len(paths):
                        raise FlowResultSnapshotError("invalid_snapshot_inventory")
                    meta_by_id = {
                        str(meta.get("snapshot_id") or "").strip(): meta for meta in valid_metas
                    }
                    if len(meta_by_id) != len(valid_metas):
                        raise FlowResultSnapshotError("invalid_snapshot_inventory")
                    manifest_snapshot_ids = self._job_result_manifest_snapshot_ids(target_case_id)
                    if manifest_snapshot_ids is None or any(
                        snapshot_id not in meta_by_id for snapshot_id in manifest_snapshot_ids
                    ):
                        raise FlowResultSnapshotError("invalid_job_result_manifest_inventory")
                    dependencies_valid, _ = self._validate_result_snapshot_dependency_inventory(
                        target_case_id,
                        valid_metas,
                    )
                    if not dependencies_valid:
                        raise FlowResultSnapshotError("invalid_snapshot_dependency_inventory")
                    seen_sidecars: set[str] = set()
                    for sidecar_path, _ in sidecar_inventory:
                        source_snapshot_id = sidecar_path.name.removesuffix(".network-graph.json")
                        if (
                            source_snapshot_id in seen_sidecars
                            or source_snapshot_id not in meta_by_id
                            or not self._valid_result_snapshot_compute_sidecar(
                                target_case_id,
                                sidecar_path,
                                source_snapshot_id,
                            )
                        ):
                            raise FlowResultSnapshotError("invalid_compute_sidecar_inventory")
                        seen_sidecars.add(source_snapshot_id)
            except (FlowResultSnapshotError, OSError) as exc:
                blocked_reason = str(exc or "").strip() or "invalid_snapshot_inventory"
                inventory_status = "unknown"
                blocked_reasons.append(blocked_reason)
                cases.append({
                    "case_id": target_case_id,
                    "inventory_status": "unknown",
                    "blocked_reason": blocked_reason,
                    "snapshot_count": None,
                    "total_bytes": None,
                    "delta_snapshot_count": None,
                    "full_snapshot_count": None,
                    "max_chain_depth": None,
                    "latest_stored_at": None,
                    "oldest_stored_at": None,
                })
                continue
            if not valid_metas and case_id is None:
                continue
            case_total_bytes = sum(max(0, int(meta.get("size_bytes") or 0)) for meta in valid_metas)
            case_delta_count = sum(1 for meta in valid_metas if str(meta.get("graph_storage_mode") or "") == "delta")
            case_max_chain_depth = max([max(0, int(meta.get("graph_storage_chain_depth") or 0)) for meta in valid_metas] or [0])
            case_summary = {
                "case_id": target_case_id,
                "inventory_status": "valid",
                "blocked_reason": "",
                "snapshot_count": len(valid_metas),
                "total_bytes": case_total_bytes,
                "delta_snapshot_count": case_delta_count,
                "full_snapshot_count": max(0, len(valid_metas) - case_delta_count),
                "max_chain_depth": case_max_chain_depth,
                "latest_stored_at": "",
                "oldest_stored_at": "",
            }
            if valid_metas:
                valid_metas.sort(
                    key=lambda meta: (
                        self._snapshot_meta_epoch_seconds(meta),
                        int(meta.get("mtime_ns") or 0),
                        str(meta.get("snapshot_id") or ""),
                    ),
                    reverse=True,
                )
                case_summary["latest_stored_at"] = str(valid_metas[0].get("stored_at") or "").strip()
                case_summary["oldest_stored_at"] = str(valid_metas[-1].get("stored_at") or "").strip()
            cases.append(case_summary)
            total_snapshot_count += len(valid_metas)
            total_bytes += case_total_bytes
            delta_snapshot_count += case_delta_count
            max_chain_depth = max(max_chain_depth, case_max_chain_depth)
        return {
            "inventory_status": inventory_status,
            "blocked_reasons": sorted(set(blocked_reasons)),
            "case_count": len(cases),
            "snapshot_count": total_snapshot_count if inventory_status == "valid" else None,
            "total_bytes": total_bytes if inventory_status == "valid" else None,
            "delta_snapshot_count": delta_snapshot_count if inventory_status == "valid" else None,
            "full_snapshot_count": (
                max(0, total_snapshot_count - delta_snapshot_count) if inventory_status == "valid" else None
            ),
            "max_chain_depth": max_chain_depth if inventory_status == "valid" else None,
            "gc_policy": {
                "soft_limit": _RESULT_SNAPSHOT_GC_SOFT_LIMIT,
                "keep_latest": _RESULT_SNAPSHOT_GC_KEEP_LATEST,
                "ttl_seconds": _RESULT_SNAPSHOT_GC_TTL_SECONDS,
            },
            "cases": cases,
        }

    def gc_result_snapshots(
        self,
        *,
        case_id: Optional[str] = None,
        _inventory_lease_held: bool = False,
    ) -> dict[str, Any]:
        if not _inventory_lease_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    return self.gc_result_snapshots(
                        case_id=case_id,
                        _inventory_lease_held=True,
                    )
            except OSError:
                return {
                    "inventory_status": "unknown",
                    "blocked_reasons": ["inventory_lease_unavailable"],
                    "case_count": None,
                    "deleted_count": None,
                    "deleted_bytes": None,
                    "before_count": None,
                    "after_count": None,
                    "cases": [],
                }
        requested_case_id = str(case_id or "").strip()
        try:
            target_case_ids = [requested_case_id] if requested_case_id else self._iter_result_snapshot_case_ids()
        except FlowResultSnapshotError:
            return {
                "inventory_status": "unknown",
                "blocked_reasons": ["case_inventory_unavailable"],
                "case_count": None,
                "deleted_count": None,
                "deleted_bytes": None,
                "before_count": None,
                "after_count": None,
                "cases": [],
            }
        cases = [
            self._gc_case_result_snapshots(
                target_case_id,
                _inventory_lease_held=True,
            )
            for target_case_id in target_case_ids
            if target_case_id
        ]
        inventory_status = (
            "valid"
            if all(str(item.get("inventory_status") or "") == "valid" for item in cases)
            else "unknown"
        )
        blocked_reasons = sorted(
            {
                str(item.get("blocked_reason") or "").strip()
                for item in cases
                if str(item.get("blocked_reason") or "").strip()
            }
        )
        return {
            "inventory_status": inventory_status,
            "blocked_reasons": blocked_reasons,
            "case_count": len(cases),
            "deleted_count": (
                sum(max(0, int(item.get("deleted_count") or 0)) for item in cases)
                if inventory_status == "valid"
                else None
            ),
            "deleted_bytes": (
                sum(max(0, int(item.get("deleted_bytes") or 0)) for item in cases)
                if inventory_status == "valid"
                else None
            ),
            "before_count": (
                sum(max(0, int(item.get("before_count") or 0)) for item in cases)
                if inventory_status == "valid"
                else None
            ),
            "after_count": (
                sum(max(0, int(item.get("after_count") or 0)) for item in cases)
                if inventory_status == "valid"
                else None
            ),
            "cases": cases,
        }

    def _load_result_snapshot(
        self,
        case_id: str,
        snapshot_ref: Optional[dict[str, Any]],
        *,
        _chain_depth: int = 0,
    ) -> dict[str, Any]:
        ref = _normalize_snapshot_ref(snapshot_ref)
        empty = _attach_graph_render_metadata({"nodes": [], "edges": [], "stats": {}, "runtime_graph": {"nodes": [], "edges": []}})
        if not ref:
            return empty
        if _chain_depth > (_RESULT_SNAPSHOT_DELTA_MAX_CHAIN_DEPTH + 4):
            return empty
        validated_payload = self._read_valid_result_snapshot_payload(case_id=case_id, snapshot_ref=ref)
        if validated_payload is None:
            return empty
        _, payload = validated_payload
        result_payload = payload.get("result") if isinstance(payload.get("result"), dict) else {}
        runtime_graph = _normalize_view_graph_blob(payload.get("graph"))
        graph_storage_mode = str(payload.get("graph_storage_mode") or "full").strip().lower() or "full"
        if graph_storage_mode == "delta":
            base_ref = _normalize_snapshot_ref(payload.get("graph_base_snapshot_ref"))
            graph_patch = payload.get("graph_patch") if isinstance(payload.get("graph_patch"), dict) else {}
            if base_ref and graph_patch:
                base_result = self._load_result_snapshot(case_id, base_ref, _chain_depth=_chain_depth + 1)
                base_graph = _normalize_view_graph_blob(base_result.get("runtime_graph"))
                if _graph_has_payload(base_graph):
                    runtime_graph = _apply_runtime_graph_patch(
                        base_graph,
                        graph_patch,
                        runtime_revision=_to_optional_int(runtime_graph.get("runtime_revision")),
                        inherit_runtime_revision=False,
                    )
        if (
            len(runtime_graph.get("nodes") or []) != int(payload.get("node_count") or 0)
            or len(runtime_graph.get("edges") or []) != int(payload.get("edge_count") or 0)
        ):
            return empty
        try:
            modern_nodes, modern_edges = _runtime_graph_to_entities(
                runtime_graph.get("nodes") or [],
                runtime_graph.get("edges") or [],
            )
        except TxnAmountCoverageIncompleteError:
            return empty
        result = {
            "nodes": modern_nodes,
            "edges": modern_edges,
            "stats": _clone_json(result_payload.get("stats"), {}),
            "publication_status": str(result_payload.get("publication_status") or ""),
            "fact_answer_allowed": result_payload.get("fact_answer_allowed"),
            "runtime_graph": runtime_graph,
            "graph_tier": str(result_payload.get("graph_tier") or ""),
            "render_hints": _clone_json(result_payload.get("render_hints"), {}),
            "projection": _clone_json(result_payload.get("projection"), {}),
        }
        if not _has_complete_candidate_amount_coverage(result, expected_case_id=case_id):
            return empty
        return result

    def persist_result_snapshot(
        self,
        *,
        case_id: str,
        result: dict[str, Any],
        base_snapshot_ref: Optional[dict[str, Any]] = None,
        case_lifecycle_binding: Optional[FlowCaseLifecycleBindingV1] = None,
    ) -> Optional[dict[str, Any]]:
        return self._persist_result_snapshot(
            case_id,
            result,
            base_snapshot_ref=base_snapshot_ref,
            case_lifecycle_binding=case_lifecycle_binding,
        )

    def get_result_snapshot(
        self,
        *,
        case_id: str,
        snapshot_ref: dict[str, Any],
        _leases_held: bool = False,
    ) -> dict[str, Any]:
        if not _leases_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                        return self.get_result_snapshot(
                            case_id=case_id,
                            snapshot_ref=snapshot_ref,
                            _leases_held=True,
                        )
            except (FlowResultSnapshotError, OSError, TypeError, ValueError, OverflowError):
                ref = None
                return _attach_graph_render_metadata({
                    "nodes": [],
                    "edges": [],
                    "stats": {},
                    "runtime_graph": {"nodes": [], "edges": []},
                    "result_snapshot_ref": ref,
                })
        ref = _normalize_snapshot_ref(snapshot_ref)
        result = self._load_result_snapshot(case_id, ref)
        result["result_snapshot_ref"] = ref
        return _attach_graph_render_metadata(result)

    def get_result_snapshot_source_ref(
        self,
        *,
        case_id: str,
        snapshot_ref: dict[str, Any],
        _leases_held: bool = False,
    ) -> Optional[dict[str, Any]]:
        if not _leases_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                        return self.get_result_snapshot_source_ref(
                            case_id=case_id,
                            snapshot_ref=snapshot_ref,
                            _leases_held=True,
                        )
            except (FlowResultSnapshotError, OSError, TypeError, ValueError, OverflowError):
                return None
        ref = _normalize_snapshot_ref(snapshot_ref)
        if not ref:
            return None
        validated_payload = self._read_valid_result_snapshot_payload(case_id=case_id, snapshot_ref=ref)
        if validated_payload is None:
            return None
        _, payload = validated_payload
        result_payload = payload.get("result") if isinstance(payload.get("result"), dict) else {}
        projection = result_payload.get("projection") if isinstance(result_payload.get("projection"), dict) else {}
        source_ref = _normalize_snapshot_ref(
            projection.get("source_result_snapshot_ref") or projection.get("sourceResultSnapshotRef")
        )
        return source_ref or ref

    def get_result_snapshot_compute_path(
        self,
        *,
        case_id: str,
        snapshot_ref: dict[str, Any],
        _leases_held: bool = False,
    ) -> str:
        if not _leases_held:
            try:
                with private_exclusive_file_lock(self._result_snapshot_inventory_lock_path()):
                    with private_exclusive_file_lock(self._result_snapshot_case_lock_path(case_id)):
                        return self.get_result_snapshot_compute_path(
                            case_id=case_id,
                            snapshot_ref=snapshot_ref,
                            _leases_held=True,
                        )
            except (FlowResultSnapshotError, OSError, TypeError, ValueError, OverflowError):
                return ""
        try:
            binding = self.freeze_case_lifecycle_binding(case_id)
        except FlowResultSnapshotError:
            return ""
        expected_generation = binding.generation
        ref = _normalize_snapshot_ref(snapshot_ref)
        if not ref:
            return ""
        ref_storage_mode = str(ref.get("graph_storage_mode") or "").strip().lower()
        if ref_storage_mode == "delta":
            return ""
        snapshot_id = str(ref.get("snapshot_id") or ref.get("graph_hash") or "").strip()
        validated_payload = self._read_valid_result_snapshot_payload(case_id=case_id, snapshot_ref=ref)
        if validated_payload is None:
            return ""
        path, payload = validated_payload
        compute_path = self._result_snapshot_compute_graph_path(case_id, snapshot_id)

        def existing_compute_path() -> str:
            try:
                text = read_private_text(compute_path, max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES)
                compute_payload = loads_strict_json(
                    text,
                    max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
                    max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                )
                if (
                    not isinstance(compute_payload, dict)
                    or set(compute_payload) != {"version", "snapshot_id", "graph"}
                    or compute_payload.get("version") != 1
                    or str(compute_payload.get("snapshot_id") or "").strip() != snapshot_id
                    or _stable_json_text(compute_payload.get("graph")) != _stable_json_text(payload.get("graph"))
                    or dumps_canonical_json(
                        compute_payload,
                        max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                    )
                    != text
                ):
                    return ""
            except (OSError, StrictJSONError):
                return ""
            return str(compute_path)

        if ref_storage_mode == "full":
            cached_compute_path = existing_compute_path()
            if cached_compute_path:
                return cached_compute_path
        graph_storage_mode = str(payload.get("graph_storage_mode") or "full").strip().lower() or "full"
        if graph_storage_mode == "delta":
            return ""
        cached_compute_path = existing_compute_path()
        if cached_compute_path:
            return cached_compute_path
        graph = payload.get("graph") if isinstance(payload.get("graph"), dict) else {}
        graph_nodes = graph.get("nodes") if isinstance(graph, dict) else None
        graph_edges = graph.get("edges") if isinstance(graph, dict) else None
        if isinstance(graph_nodes, list) and isinstance(graph_edges, list) and graph_nodes:
            try:
                _runtime_graph_to_entities(graph_nodes, graph_edges)
            except TxnAmountCoverageIncompleteError:
                return ""
            return self._write_result_snapshot_compute_graph(
                case_id=case_id,
                snapshot_id=snapshot_id,
                compute_path=compute_path,
                graph={"nodes": graph_nodes, "edges": graph_edges},
                expected_generation=expected_generation,
                expected_binding_digest=binding.binding_digest,
            )
        return ""

    def _write_result_snapshot_compute_graph(
        self,
        *,
        case_id: str,
        snapshot_id: str,
        compute_path: Path,
        graph: dict[str, Any],
        expected_generation: int,
        expected_binding_digest: str,
    ) -> str:
        try:
            payload = {
                "version": 2,
                "case_lifecycle_generation": expected_generation,
                "case_lifecycle_binding_digest": expected_binding_digest,
                "snapshot_id": snapshot_id,
                "graph": graph,
            }
            payload_text = dumps_canonical_json(
                payload,
                max_nodes=_FLOW_RESULT_SNAPSHOT_MAX_JSON_NODES,
                max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES,
            )
            if not self._case_binding_is_publishable(
                case_id,
                expected_generation=expected_generation,
            ):
                return ""
            if not self._result_snapshot_dependency_closure_is_reconstructable(
                case_id,
                {"snapshot_id": snapshot_id, "graph_hash": snapshot_id},
            ):
                return ""
            atomic_create_private_text(compute_path, payload_text)
            if read_private_text(compute_path, max_bytes=_FLOW_RESULT_SNAPSHOT_MAX_BYTES) != payload_text:
                return ""
            return str(compute_path)
        except (FlowResultSnapshotError, OSError, StrictJSONError):
            return ""

    def get_result_snapshot_patch(
        self,
        *,
        case_id: str,
        base_snapshot_ref: dict[str, Any],
        target_snapshot_ref: dict[str, Any],
    ) -> dict[str, Any]:
        base_ref = _normalize_snapshot_ref(base_snapshot_ref)
        target_ref = _normalize_snapshot_ref(target_snapshot_ref)
        if not base_ref or not target_ref:
            return _attach_graph_render_metadata({
                "base_snapshot_ref": base_ref,
                "result_snapshot_ref": target_ref,
                "runtime_graph": {"nodes": [], "edges": []},
                "runtime_graph_patch": None,
                "patch_kind": "full",
                "nodes": [],
                "edges": [],
                "stats": {},
                "projection": {},
            })
        base_result = self._load_result_snapshot(case_id, base_ref)
        target_result = self._load_result_snapshot(case_id, target_ref)
        return _build_result_snapshot_patch_payload(
            base_ref=base_ref,
            target_ref=target_ref,
            base_result=base_result,
            target_result=target_result,
        )

    def expand_result_snapshot(
        self,
        *,
        case_id: str,
        base_snapshot_ref: dict[str, Any],
        expand_request: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        base_ref = _normalize_snapshot_ref(base_snapshot_ref)
        base_result = self._load_result_snapshot(case_id, base_ref)
        base_projection = _normalize_projection_payload(base_result.get("projection"))
        source_snapshot_ref, full_result = self._materialize_projection_source_result(
            case_id=case_id,
            base_snapshot_ref=base_ref,
            base_projection=base_projection,
        )
        source_snapshot_ref = source_snapshot_ref or base_ref
        merged_request = _merge_projection_expand_request(base_projection, expand_request)
        viewport = merged_request.get("viewport") if isinstance(merged_request.get("viewport"), dict) else {}
        if (
            viewport
            and not merged_request.get("reset")
            and not _to_text_list(merged_request.get("cluster_ids"))
            and not _to_text_list(merged_request.get("tile_ids"))
            and not _to_text_list(merged_request.get("node_ids"))
            and not _to_text_list(merged_request.get("path_node_ids"))
            and not str(merged_request.get("search_query") or "").strip()
        ):
            viewport_targets = _select_projection_targets_from_viewport(
                base_result.get("runtime_graph"),
                viewport,
            )
            if viewport_targets.get("cluster_ids") or viewport_targets.get("node_ids"):
                merged_request = {
                    **merged_request,
                    "cluster_ids": _to_text_list(viewport_targets.get("cluster_ids")),
                    "node_ids": _to_text_list(viewport_targets.get("node_ids")),
                }
        projected_result = _try_build_partial_projected_result(
            full_result=full_result,
            source_snapshot_ref=source_snapshot_ref,
            request_context=merged_request,
            base_result=base_result,
            base_projection=base_projection,
        )
        if projected_result is None:
            projected_result = _build_projected_result(
                full_result=full_result,
                source_snapshot_ref=source_snapshot_ref,
                request_context=merged_request,
                base_projection=base_projection,
            )
        if projected_result is None:
            fallback = self.get_result_snapshot(case_id=case_id, snapshot_ref=base_ref or source_snapshot_ref or {})
            fallback["base_snapshot_ref"] = base_ref
            fallback["patch_kind"] = "full"
            fallback["runtime_graph_patch"] = None
            return _attach_graph_render_metadata(fallback)
        if base_projection.get("source_query") and not _normalize_snapshot_ref(
            _normalize_projection_payload(projected_result.get("projection")).get("source_result_snapshot_ref")
        ):
            projection = _normalize_projection_payload(projected_result.get("projection"))
            projection["source_query"] = _clone_json(base_projection.get("source_query"), {})
            projected_result["projection"] = projection
        target_snapshot_ref, snapshot_write_meta = self._persist_result_snapshot(
            case_id=case_id,
            result=projected_result,
            base_snapshot_ref=base_ref or source_snapshot_ref,
            return_storage_metadata=True,
            compact_result_entities=bool(projected_result.get("_partial_expand_fast_path")),
            precomputed_graph_patch=(
                projected_result.get("_partial_graph_patch")
                if isinstance(projected_result.get("_partial_graph_patch"), dict)
                else None
            ),
        )
        if not target_snapshot_ref:
            fallback = _attach_graph_render_metadata(projected_result)
            fallback["base_snapshot_ref"] = base_ref
            fallback["patch_kind"] = "full"
            fallback["runtime_graph_patch"] = None
            return fallback
        payload = _build_result_snapshot_patch_payload(
            base_ref=base_ref or source_snapshot_ref or {},
            target_ref=target_snapshot_ref,
            base_result=base_result,
            target_result=projected_result,
            precomputed_patch=(
                snapshot_write_meta.get("graph_patch") if isinstance(snapshot_write_meta, dict) else None
            ),
        )
        payload["projection_action"] = "expand"
        return payload

    @staticmethod
    def _empty_view_state() -> dict[str, Any]:
        graph = {"nodes": [], "edges": []}
        return {
            "schema_version": 1,
            "runtime_view": {},
            "view_state_v2": {
                "schema_version": 2,
                "graph": dict(graph),
                "filters": {},
                "counts": {},
            },
            "graph": dict(graph),
            "filters": {},
            "counts": {},
        }

    @classmethod
    def _public_view_item(cls, *, case_id: str, view_id: str) -> dict[str, Any]:
        return {
            "view_id": view_id,
            "case_id": case_id,
            "view_name": "Saved flow view",
            "graph_query": {},
            "view_state": cls._empty_view_state(),
            "created_at": "",
            "updated_at": "",
        }

    def _purge_legacy_flow_view_artifacts(self) -> None:
        for name in ("flow_views", "flow_view_snapshots"):
            try:
                remove_path_no_follow_under(self._storage.app_dir, Path(self._storage.app_dir) / name)
            except OSError:
                raise FlowViewMetadataError("legacy_flow_view_purge_failed") from None

    def _load_case_views(self, case_id: str) -> dict:
        path = self._case_views_path(case_id)
        if not path.exists():
            return {"version": _FLOW_VIEW_METADATA_VERSION, "case_id": case_id, "views": []}
        try:
            text = read_private_text(path, max_bytes=_FLOW_VIEW_METADATA_MAX_BYTES)
            payload = loads_strict_json(
                text,
                max_bytes=_FLOW_VIEW_METADATA_MAX_BYTES,
                max_nodes=_FLOW_VIEW_METADATA_MAX_JSON_NODES,
            )
            if dumps_canonical_json(
                payload,
                max_nodes=_FLOW_VIEW_METADATA_MAX_JSON_NODES,
                max_bytes=_FLOW_VIEW_METADATA_MAX_BYTES,
            ) != text:
                raise StrictJSONError("flow_view_metadata_not_canonical")
        except (OSError, StrictJSONError):
            raise FlowViewMetadataError("flow_view_metadata_invalid") from None
        if (
            not isinstance(payload, dict)
            or set(payload) != {"version", "case_id", "views"}
            or payload.get("version") != _FLOW_VIEW_METADATA_VERSION
            or str(payload.get("case_id") or "") != str(case_id or "").strip()
            or not isinstance(payload.get("views"), list)
        ):
            raise FlowViewMetadataError("flow_view_metadata_invalid")

        views: list[dict[str, Any]] = []
        seen: set[str] = set()
        for raw in payload["views"]:
            if not isinstance(raw, dict) or set(raw) != {"view_id", "case_id"}:
                raise FlowViewMetadataError("flow_view_metadata_invalid")
            view_id = str(raw.get("view_id") or "")
            try:
                if str(uuid.UUID(view_id)) != view_id or str(raw.get("case_id") or "") != str(case_id or "").strip():
                    raise ValueError
            except (AttributeError, TypeError, ValueError):
                raise FlowViewMetadataError("flow_view_metadata_invalid") from None
            if view_id in seen:
                raise FlowViewMetadataError("flow_view_metadata_invalid")
            seen.add(view_id)
            views.append(self._public_view_item(case_id=str(case_id or "").strip(), view_id=view_id))
        return {"version": _FLOW_VIEW_METADATA_VERSION, "case_id": str(case_id or "").strip(), "views": views}

    def _save_case_views(self, case_id: str, data: dict) -> None:
        path = self._case_views_path(case_id)
        stored_views: list[dict[str, str]] = []
        seen: set[str] = set()
        for raw in list(data.get("views") or []):
            if not isinstance(raw, dict):
                raise FlowViewMetadataError("flow_view_metadata_invalid")
            view_id = str(raw.get("view_id") or "")
            try:
                if str(uuid.UUID(view_id)) != view_id or view_id in seen:
                    raise ValueError
            except (AttributeError, TypeError, ValueError):
                raise FlowViewMetadataError("flow_view_metadata_invalid") from None
            seen.add(view_id)
            stored_views.append({"view_id": view_id, "case_id": str(case_id or "").strip()})
        stored_payload = {
            "version": _FLOW_VIEW_METADATA_VERSION,
            "case_id": str(case_id or "").strip(),
            "views": stored_views,
        }
        try:
            text = dumps_canonical_json(
                stored_payload,
                max_nodes=_FLOW_VIEW_METADATA_MAX_JSON_NODES,
                max_bytes=_FLOW_VIEW_METADATA_MAX_BYTES,
            )
        except StrictJSONError:
            raise FlowViewMetadataError("flow_view_metadata_invalid") from None
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            if path.parent.is_symlink() or path.parent.resolve(strict=True) != path.parent.absolute():
                raise OSError
            if os.name != "nt":
                os.chmod(path.parent, 0o700)
            atomic_write_private_text(path, text)
            if read_private_text(path, max_bytes=_FLOW_VIEW_METADATA_MAX_BYTES) != text:
                raise OSError
        except OSError:
            raise FlowViewMetadataError("flow_view_metadata_write_failed") from None

    def _find_case_view(
        self,
        *,
        case_id: str,
        view_id: str,
    ) -> Tuple[str, dict, dict]:
        normalized_case_id = str(case_id or "").strip()
        target = str(view_id or "").strip()
        if not normalized_case_id or not target:
            raise FlowViewNotFoundError(view_id)
        try:
            if str(uuid.UUID(target)) != target:
                raise FlowViewNotFoundError(view_id)
        except (AttributeError, TypeError, ValueError) as exc:
            raise FlowViewNotFoundError(view_id) from exc
        data = self._load_case_views(normalized_case_id)
        if str(data.get("case_id") or "").strip() != normalized_case_id:
            raise FlowViewNotFoundError(view_id)
        for item in list(data.get("views") or []):
            if (
                str(item.get("view_id") or "").strip() == target
                and str(item.get("case_id") or "").strip() == normalized_case_id
            ):
                return normalized_case_id, data, item
        raise FlowViewNotFoundError(view_id)

    def _table_exists(self, con: DuckDBEngine, table: str) -> bool:
        rows = con.query(
            "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name=? LIMIT 1",
            (table,),
        )
        return bool(rows)

    def _table_columns(self, con: DuckDBEngine, table: str) -> set[str]:
        rows = con.query(
            "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name=?",
            (table,),
        )
        return {str(row[0]) for row in rows if row and row[0]}
