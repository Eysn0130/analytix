from __future__ import annotations

import base64
import difflib
import hashlib
import html
import json
import math
import re
import time
import uuid
from collections import defaultdict
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal
from getpass import getuser
from io import BytesIO
from pathlib import Path
from typing import Any, Callable, Iterable, Mapping, Optional, Sequence

import pandas as pd
try:
    from sqlglot import exp as _sqlglot_exp, parse as _sqlglot_parse
    from sqlglot.errors import ParseError as _SqlglotParseError
except Exception:  # pragma: no cover - exercised when packaging misses the parser dependency.
    _sqlglot_exp = None
    _sqlglot_parse = None

    class _SqlglotParseError(Exception):
        pass

from app.core.case_project_doc import (
    CASE_DIRECTION_DOC_FILENAME,
    CASE_PROJECT_DOC_FILENAME,
    CASE_RUNTIME_PROJECT_DOC_FILENAME,
    case_direction_doc_path,
    case_project_doc_path,
    case_runtime_project_doc_path,
)
from app.core.analysis_compute import try_materialize_rule_pattern_index, try_materialize_rule_txn_index
from app.core.data_engine_client import DataEngineUnavailableError
from app.core.db_engine import DuckDBEngine
from app.core.diagnostic_duckdb_migration import DiagnosticDuckDBMigrationError
from app.core.fc_import_norm_insert import ts_norm_expr as _ts_norm_expr
from app.core.import_count_semantics import (
    IMPORT_COUNTS_VERSION,
    is_import_success_status,
    known_public_import_count,
)
from app.core.storage import CaseStorage
from app.domain.case_taxonomy import (
    build_case_direction_context,
    build_case_direction_focus,
)
from app.domain.analysis_workbench_boundary import (
    normalize_case_sql_query_ref,
    opaque_case_bound_ref,
    project_case_sql_capability_boundary,
    project_case_sql_diagnostic,
    project_temp_scope_failure,
    project_temp_scope_public,
    project_workbench_history_item,
)
from app.domain.analysis_account_fact_boundary import (
    account_fact_operation_requires_host_authority,
    account_fact_skill_requires_host_authority,
    project_account_fact_boundary,
)
from app.domain.analysis_maintenance_public_projection import (
    evidence_pack_scope_requires_host_authority,
    evidence_pack_skill_requires_host_authority,
    project_analysis_evidence_pack_public,
    project_analysis_maintenance_public,
)
from app.domain.ordinary_diagnostic_projection import (
    project_diagnostic_timestamp,
    project_operational_event,
    project_query_log_diagnostic,
    project_query_tool_name,
)
from app.domain.workspace_artifact_renderer import WorkspaceArtifactRenderer
from app.orchestration.cache_janitor import CacheJanitorSummary
from app.orchestration.memory_retention import MemoryJanitorSummary
from app.orchestration.retention_policy import DEFAULT_RUNTIME_RETENTION_POLICY
from app.orchestration.run_log_retention import RunLogRetentionSummary, is_run_log_anchor
from app.orchestration.workspace_janitor import WorkspaceJanitorSummary
from app.repositories.analysis_revision import (
    bump_stats_flow_source_revision,
    get_stats_flow_source_revision,
)
from app.repositories.privacy_projection_repository import PrivacyProjectionRepository
from app.repositories.txn_daily_aggregate import (
    TxnAmountCoverageIncompleteError,
    TxnAmountCoverageV1,
    TxnDailyAggregateStore,
)
from app.utils.fs import atomic_write_private_text
from app.utils.mixed_fund_tracking import enrich_mixed_fund_tracking
from docx import Document
from docx.enum.section import WD_SECTION
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Mm, Pt
from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.cidfonts import UnicodeCIDFont
from reportlab.platypus import ListFlowable, ListItem, Paragraph, SimpleDocTemplate, Spacer


RULE_TXN_INDEX_CACHE_KEY = "rule_txn_index:v2"
RULE_TXN_INDEX_VERSION = 2
RULE_PATTERN_INDEX_CACHE_KEY = "rule_pattern_index:v9"
RULE_PATTERN_INDEX_VERSION = 9
RULE_TXN_SOURCE_SIGNATURE_DOMAIN = "analytix.rule-transaction-source/v2"
RULE_TXN_RESULT_SIGNATURE_DOMAIN = "analytix.rule-transaction-index-result/v2"
RULE_PATTERN_RESULT_SIGNATURE_DOMAIN = "analytix.rule-pattern-index-result/v9"
_TXN_CLEAN_STATE_COLUMNS = frozenset({"clean_invalid", "clean_failed", "clean_reversal"})


class TransactionFactSourceUnavailableError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("transaction_fact_source_unavailable")


class AnalysisGraphContractError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("analysis_graph_contract_invalid")


RULE_DEFINITIONS: dict[str, dict[str, str]] = {
    "FAST_IN_FAST_OUT": {"title": "短时快进快出", "risk_type": "flow_pattern", "severity": "high"},
    "SMALL_FAST_IN_OUT": {"title": "小额分散快进快出", "risk_type": "flow_pattern", "severity": "medium"},
    "SINGLE_FAST_IN_OUT_CANDIDATE": {"title": "单组小额快进快出候选", "risk_type": "flow_pattern_candidate", "severity": "low"},
    "HIGH_FREQ_SMALL_OUT": {"title": "高频小额转出", "risk_type": "flow_pattern", "severity": "medium"},
    "THRESHOLD_SPLIT": {"title": "临界额拆分", "risk_type": "amount_pattern", "severity": "high"},
    "NEAR_THRESHOLD_STRUCTURING": {"title": "临界阈值规避", "risk_type": "amount_pattern", "severity": "medium"},
    "REPEATED_AMOUNT_PATTERN": {"title": "重复金额模式", "risk_type": "amount_pattern", "severity": "medium"},
    "ROUND_AMOUNT_PATTERN": {"title": "整数金额反复出现", "risk_type": "amount_pattern", "severity": "medium"},
    "CASH_QUICK_IN_OUT": {"title": "现金快存快取", "risk_type": "cash_pattern", "severity": "medium"},
    "SINGLE_CASH_QUICK_IN_OUT_CANDIDATE": {"title": "单组现金快存快取候选", "risk_type": "cash_pattern_candidate", "severity": "low"},
    "NIGHT_OFFHOUR_ACTIVITY": {"title": "夜间/非营业时间交易", "risk_type": "time_pattern", "severity": "medium"},
    "SAME_NAME_TRANSFER_CLUSTER": {"title": "同名账户往来集中", "risk_type": "control_link", "severity": "medium"},
    "SHARED_IP_MULTI_ACCOUNT": {"title": "同 IP 多账户", "risk_type": "device_link", "severity": "medium"},
    "SHARED_MAC_MULTI_ACCOUNT": {"title": "同 MAC 多账户", "risk_type": "device_link", "severity": "high"},
    "SHARED_TELLER_MULTI_ACCOUNT": {"title": "同柜员多账户共现", "risk_type": "counter_service_link", "severity": "medium"},
    "FAN_IN_FAN_OUT": {"title": "扇入扇出归集分流", "risk_type": "network_flow", "severity": "high"},
    "CIRCULAR_FLOW_RETURN": {"title": "资金循环回流", "risk_type": "network_flow", "severity": "high"},
    "ASSET_PURCHASE_OUTFLOW": {"title": "大额资产购置线索", "risk_type": "asset_lifestyle_clue", "severity": "medium"},
    "HIGH_CONSUMPTION_LIFESTYLE": {"title": "高消费线索", "risk_type": "asset_lifestyle_clue", "severity": "medium"},
    "PROPERTY_PARKING_PAYMENT": {"title": "物业/停车线索", "risk_type": "asset_lifestyle_clue", "severity": "low"},
    "COUNTERPARTY_CONCENTRATION": {"title": "对手集中度异常", "risk_type": "relationship_pattern", "severity": "medium"},
    "FAILED_RETRY_PROBE": {"title": "失败后重试探测", "risk_type": "operation_pattern", "severity": "high"},
}

RULE_PARAM_DEFAULTS: dict[str, Any] = {
    "rule_engine_version": "economic_crime_fund_rules_v5_2026_04",
    "fast_in_out_window_minutes": 30,
    "fast_in_out_ratio": 0.8,
    "fast_in_out_min_amount": 50000.0,
    "fast_in_out_strong_amount": 200000.0,
    "small_fast_in_out_window_minutes": 60,
    "small_fast_in_out_ratio": 0.75,
    "small_fast_in_out_min_amount": 1000.0,
    "small_fast_in_out_max_amount": 50000.0,
    "small_fast_in_out_min_event_count": 3,
    "small_fast_in_out_min_total_amount": 10000.0,
    "single_fast_in_out_candidate_window_minutes": 30,
    "single_fast_in_out_candidate_min_amount": 1000.0,
    "single_fast_in_out_candidate_max_amount": 50000.0,
    "single_fast_in_out_candidate_min_out_ratio": 0.9,
    "single_fast_in_out_candidate_max_out_ratio": 1.1,
    "single_fast_in_out_candidate_max_event_count": 2,
    "small_amount_threshold": 5000.0,
    "high_freq_window_minutes": 30,
    "high_freq_count_threshold": 8,
    "high_freq_small_min_total_amount": 20000.0,
    "threshold_split_amount": 50000.0,
    "threshold_split_tolerance_rate": 0.05,
    "threshold_split_min_count": 3,
    "near_threshold_amount": 50000.0,
    "near_threshold_lower_rate": 0.9,
    "near_threshold_window_minutes": 14400,
    "near_threshold_min_count": 2,
    "near_threshold_min_total_amount": 90000.0,
    "cash_quick_in_out_window_minutes": 60,
    "cash_quick_in_out_min_amount": 5000.0,
    "cash_quick_in_out_min_event_count": 2,
    "cash_quick_in_out_min_total_amount": 10000.0,
    "cash_quick_candidate_window_minutes": 60,
    "cash_quick_candidate_min_amount": 1000.0,
    "cash_quick_candidate_min_out_ratio": 0.9,
    "cash_quick_candidate_max_out_ratio": 1.1,
    "cash_quick_candidate_max_event_count": 1,
    "repeated_amount_window_minutes": 1440,
    "repeated_amount_min_amount": 1000.0,
    "repeated_amount_min_count": 3,
    "repeated_amount_min_total_amount": 10000.0,
    "round_amount_unit": 10000.0,
    "round_amount_min_amount": 10000.0,
    "round_amount_min_count": 3,
    "round_amount_min_total_amount": 50000.0,
    "night_activity_start_hour": 22,
    "night_activity_end_hour": 6,
    "night_activity_min_count": 3,
    "night_activity_min_total_amount": 20000.0,
    "same_name_transfer_min_count": 2,
    "same_name_transfer_min_total_amount": 10000.0,
    "shared_teller_account_threshold": 3,
    "fan_in_out_window_minutes": 1440,
    "fan_in_out_min_unique_in": 3,
    "fan_in_out_min_unique_out": 3,
    "fan_in_out_min_total_amount": 50000.0,
    "circular_flow_window_minutes": 10080,
    "circular_flow_min_amount": 5000.0,
    "circular_flow_tolerance_rate": 0.15,
    "asset_purchase_min_amount": 50000.0,
    "high_consumption_min_amount": 20000.0,
    "high_consumption_window_minutes": 43200,
    "high_consumption_min_total_amount": 50000.0,
    "property_parking_min_count": 2,
    "property_parking_min_total_amount": 3000.0,
    "shared_ip_account_threshold": 3,
    "shared_mac_account_threshold": 2,
    "counterparty_top3_ratio": 0.7,
    "failed_retry_window_minutes": 10,
    "failed_retry_count_threshold": 3,
    "trace_default_depth": 3,
    "trace_default_time_window_sec": 1800,
    "trace_default_tolerance_rate": 0.03,
}

_RULE_PARAM_INTEGER_MINIMUMS: dict[str, int] = {
    "fast_in_out_window_minutes": 1,
    "small_fast_in_out_window_minutes": 1,
    "small_fast_in_out_min_event_count": 2,
    "single_fast_in_out_candidate_window_minutes": 1,
    "single_fast_in_out_candidate_max_event_count": 1,
    "high_freq_window_minutes": 1,
    "high_freq_count_threshold": 2,
    "threshold_split_min_count": 2,
    "near_threshold_window_minutes": 1,
    "near_threshold_min_count": 2,
    "cash_quick_in_out_window_minutes": 1,
    "cash_quick_in_out_min_event_count": 2,
    "cash_quick_candidate_window_minutes": 1,
    "cash_quick_candidate_max_event_count": 1,
    "repeated_amount_window_minutes": 1,
    "repeated_amount_min_count": 2,
    "round_amount_min_count": 2,
    "night_activity_min_count": 2,
    "same_name_transfer_min_count": 2,
    "shared_teller_account_threshold": 2,
    "fan_in_out_window_minutes": 1,
    "fan_in_out_min_unique_in": 2,
    "fan_in_out_min_unique_out": 2,
    "circular_flow_window_minutes": 1,
    "high_consumption_window_minutes": 1,
    "property_parking_min_count": 1,
    "shared_ip_account_threshold": 2,
    "shared_mac_account_threshold": 2,
    "failed_retry_window_minutes": 1,
    "failed_retry_count_threshold": 2,
    "trace_default_depth": 1,
    "trace_default_time_window_sec": 60,
}

_RULE_PARAM_FLOAT_MINIMUMS: dict[str, float] = {
    "fast_in_out_ratio": 0.1,
    "small_fast_in_out_ratio": 0.1,
    "single_fast_in_out_candidate_min_out_ratio": 0.1,
    "cash_quick_candidate_min_out_ratio": 0.1,
    "round_amount_unit": 1.0,
}

_RULE_PARAM_MAXIMUMS: dict[str, float] = {
    "near_threshold_lower_rate": 0.99,
    "night_activity_start_hour": 23,
    "night_activity_end_hour": 23,
    "counterparty_top3_ratio": 1.0,
    "circular_flow_tolerance_rate": 1.0,
    "trace_default_depth": 6,
    "trace_default_time_window_sec": 86_400,
    "trace_default_tolerance_rate": 1.0,
}

_RULE_PARAM_ORDERED_BOUNDS: tuple[tuple[str, str], ...] = (
    ("fast_in_out_strong_amount", "fast_in_out_min_amount"),
    ("small_fast_in_out_max_amount", "small_fast_in_out_min_amount"),
    ("single_fast_in_out_candidate_max_amount", "single_fast_in_out_candidate_min_amount"),
    ("single_fast_in_out_candidate_max_out_ratio", "single_fast_in_out_candidate_min_out_ratio"),
    ("cash_quick_candidate_max_out_ratio", "cash_quick_candidate_min_out_ratio"),
)

_INVALID_TXN_ID_MARKERS: tuple[str, ...] = (
    "查无信息",
    "无",
    "未知",
    "unknown",
    "__unknown__",
    "null",
    "none",
    "na",
    "n/a",
    "-",
    "--",
)

SAME_FACT_ELIGIBILITY_VERSION = "SameFactEligibilityV1"
SAME_FACT_KEY_VERSION = "analytix.same-fact-dedupe-key/v2"
_SAME_FACT_REJECTION_REASONS = frozenset(
    {
        "transaction_id_missing_or_placeholder",
        "row_identity_missing_or_placeholder",
        "transaction_time_missing_or_invalid",
        "direction_missing_or_invalid",
        "amount_missing_or_invalid",
        "amount_precision_unsupported",
        "balance_missing_or_invalid",
        "balance_precision_unsupported",
        "counterparty_missing_or_placeholder",
        "summary_missing_or_placeholder",
        "remark_missing_or_placeholder",
        "transaction_type_missing_or_placeholder",
        "success_status_missing_or_unrecognized",
        "account_key_missing_or_placeholder",
        "holder_identity_missing_or_placeholder",
        "duplicate_row_identity",
    }
)


def _sql_string_literals(values: Sequence[str]) -> str:
    return ", ".join("'" + str(value).replace("'", "''").lower() + "'" for value in values)


def _valid_txn_id_sql(txn_id_expr: str = "txn_id") -> str:
    invalid_literals = _sql_string_literals(_INVALID_TXN_ID_MARKERS)
    return (
        f"NULLIF(TRIM(COALESCE(CAST({txn_id_expr} AS VARCHAR), '')), '') IS NOT NULL "
        f"AND LOWER(TRIM(CAST({txn_id_expr} AS VARCHAR))) NOT IN ({invalid_literals})"
    )


def _same_fact_success_key_sql(success_expr: str = "is_success") -> str:
    normalized = f"LOWER(TRIM(CAST({success_expr} AS VARCHAR)))"
    success_values = _sql_string_literals(
        ("1", "true", "t", "y", "yes", "ok", "succ", "success", "成功", "通过", "是")
    )
    failure_values = _sql_string_literals(
        ("0", "false", "f", "n", "no", "fail", "failed", "error", "失败", "拒绝", "异常", "超时", "否")
    )
    return f"""
      CASE
        WHEN {normalized} IN ({success_values}) THEN 'success'
        WHEN {normalized} IN ({failure_values}) THEN 'failure'
        ELSE NULL
      END
    """


def _same_fact_txn_rejection_reason_sql(
    *,
    txn_id_expr: str = "txn_id",
    row_id_expr: str = "id",
    txn_ts_expr: str = "txn_ts",
    direction_expr: str = "dc_val",
    amount_expr: str = "amount",
    balance_expr: str = "balance",
    counterparty_account_expr: str = "cp_key",
    counterparty_name_expr: str = "cp_name",
    counterparty_raw_expr: str = "cp_raw",
    summary_expr: str = "summary",
    remark_expr: str = "remark",
    txn_type_expr: str = "txn_type",
    success_expr: str = "is_success",
) -> str:
    amount_value = f"TRY_CAST({amount_expr} AS DOUBLE)"
    balance_value = f"TRY_CAST({balance_expr} AS DOUBLE)"
    counterparty_known = " OR ".join(
        f"({_valid_txn_id_sql(expr)})"
        for expr in (counterparty_account_expr, counterparty_name_expr, counterparty_raw_expr)
    )
    success_key = _same_fact_success_key_sql(success_expr)
    return f"""
      CASE
        WHEN NOT ({_valid_txn_id_sql(txn_id_expr)}) THEN 'transaction_id_missing_or_placeholder'
        WHEN NOT ({_valid_txn_id_sql(row_id_expr)}) THEN 'row_identity_missing_or_placeholder'
        WHEN TRY_CAST({txn_ts_expr} AS TIMESTAMP) IS NULL THEN 'transaction_time_missing_or_invalid'
        WHEN COALESCE(TRIM(CAST({direction_expr} AS VARCHAR)), '') NOT IN ('进', '出') THEN 'direction_missing_or_invalid'
        WHEN {amount_value} IS NULL OR NOT isfinite({amount_value}) THEN 'amount_missing_or_invalid'
        WHEN {amount_value}<>ROUND({amount_value}, 2) THEN 'amount_precision_unsupported'
        WHEN {balance_value} IS NULL OR NOT isfinite({balance_value}) THEN 'balance_missing_or_invalid'
        WHEN {balance_value}<>ROUND({balance_value}, 2) THEN 'balance_precision_unsupported'
        WHEN NOT ({counterparty_known}) THEN 'counterparty_missing_or_placeholder'
        WHEN NOT ({_valid_txn_id_sql(summary_expr)}) THEN 'summary_missing_or_placeholder'
        WHEN NOT ({_valid_txn_id_sql(remark_expr)}) THEN 'remark_missing_or_placeholder'
        WHEN NOT ({_valid_txn_id_sql(txn_type_expr)}) THEN 'transaction_type_missing_or_placeholder'
        WHEN ({success_key}) IS NULL THEN 'success_status_missing_or_unrecognized'
        ELSE NULL
      END
    """


def _same_fact_txn_eligibility_sql(
    *,
    txn_id_expr: str = "txn_id",
    row_id_expr: str = "id",
    txn_ts_expr: str = "txn_ts",
    direction_expr: str = "dc_val",
    amount_expr: str = "amount",
    balance_expr: str = "balance",
    counterparty_account_expr: str = "cp_key",
    counterparty_name_expr: str = "cp_name",
    counterparty_raw_expr: str = "cp_raw",
    summary_expr: str = "summary",
    remark_expr: str = "remark",
    txn_type_expr: str = "txn_type",
    success_expr: str = "is_success",
) -> str:
    rejection_reason = _same_fact_txn_rejection_reason_sql(
        txn_id_expr=txn_id_expr,
        row_id_expr=row_id_expr,
        txn_ts_expr=txn_ts_expr,
        direction_expr=direction_expr,
        amount_expr=amount_expr,
        balance_expr=balance_expr,
        counterparty_account_expr=counterparty_account_expr,
        counterparty_name_expr=counterparty_name_expr,
        counterparty_raw_expr=counterparty_raw_expr,
        summary_expr=summary_expr,
        remark_expr=remark_expr,
        txn_type_expr=txn_type_expr,
        success_expr=success_expr,
    )
    return f"(({rejection_reason}) IS NULL)"


def _same_fact_txn_key_sql(
    *,
    txn_id_expr: str = "txn_id",
    row_id_expr: str = "id",
    txn_ts_expr: str = "txn_ts",
    direction_expr: str = "dc_val",
    amount_expr: str = "amount",
    balance_expr: str = "balance",
    counterparty_account_expr: str = "cp_key",
    counterparty_name_expr: str = "cp_name",
    counterparty_raw_expr: str = "cp_raw",
    summary_expr: str = "summary",
    remark_expr: str = "remark",
    txn_type_expr: str = "txn_type",
    success_expr: str = "is_success",
) -> str:
    eligibility = _same_fact_txn_eligibility_sql(
        txn_id_expr=txn_id_expr,
        row_id_expr=row_id_expr,
        txn_ts_expr=txn_ts_expr,
        direction_expr=direction_expr,
        amount_expr=amount_expr,
        balance_expr=balance_expr,
        counterparty_account_expr=counterparty_account_expr,
        counterparty_name_expr=counterparty_name_expr,
        counterparty_raw_expr=counterparty_raw_expr,
        summary_expr=summary_expr,
        remark_expr=remark_expr,
        txn_type_expr=txn_type_expr,
        success_expr=success_expr,
    )
    amount_value = f"TRY_CAST({amount_expr} AS DOUBLE)"
    balance_value = f"TRY_CAST({balance_expr} AS DOUBLE)"
    success_key = _same_fact_success_key_sql(success_expr)
    return f"""
      CASE
        WHEN {eligibility}
        THEN
          'txn:' || TRIM(CAST({txn_id_expr} AS VARCHAR))
          || '|ts:' || CAST(TRY_CAST({txn_ts_expr} AS TIMESTAMP) AS VARCHAR)
          || '|dc:' || TRIM(CAST({direction_expr} AS VARCHAR))
          || '|amt:' || CAST(ROUND({amount_value}, 2) AS VARCHAR)
          || '|bal:' || CAST(ROUND({balance_value}, 2) AS VARCHAR)
          || '|cp:' || COALESCE(NULLIF(TRIM(CAST({counterparty_account_expr} AS VARCHAR)), ''), NULLIF(TRIM(CAST({counterparty_name_expr} AS VARCHAR)), ''), NULLIF(TRIM(CAST({counterparty_raw_expr} AS VARCHAR)), ''))
          || '|summary:' || TRIM(CAST({summary_expr} AS VARCHAR))
          || '|remark:' || TRIM(CAST({remark_expr} AS VARCHAR))
          || '|type:' || TRIM(CAST({txn_type_expr} AS VARCHAR))
          || '|success:' || ({success_key})
        ELSE NULL
      END
    """


def _project_same_fact_eligibility(
    summary: Mapping[str, Any] | None,
    reason_rows: Sequence[Mapping[str, Any]] | None,
) -> dict[str, Any]:
    unresolved = {
        "contract": SAME_FACT_ELIGIBILITY_VERSION,
        "key_version": SAME_FACT_KEY_VERSION,
        "coverage_status": "unresolved",
        "requested_row_count": None,
        "fact_eligible_row_count": None,
        "family_eligible_row_count": None,
        "rejected_row_count": None,
        "rejection_reason_histogram": None,
        "blocker": "same_fact_eligibility_unavailable",
    }
    if not isinstance(summary, Mapping) or reason_rows is None:
        return unresolved

    requested = _exact_nonnegative_db_int(summary.get("requested_row_count"))
    fact_eligible = _exact_nonnegative_db_int(summary.get("fact_eligible_row_count"))
    family_eligible = _exact_nonnegative_db_int(summary.get("family_eligible_row_count"))
    rejected = _exact_nonnegative_db_int(summary.get("rejected_row_count"))
    if (
        requested is None
        or fact_eligible is None
        or family_eligible is None
        or rejected is None
        or fact_eligible > requested
        or family_eligible > fact_eligible
        or rejected != requested - family_eligible
    ):
        return unresolved

    histogram: dict[str, int] = {}
    for row in reason_rows:
        if not isinstance(row, Mapping):
            return unresolved
        reason = _trim(row.get("rejection_reason"))
        count = _exact_nonnegative_db_int(row.get("row_count"))
        if reason not in _SAME_FACT_REJECTION_REASONS or count is None or count <= 0 or reason in histogram:
            return unresolved
        histogram[reason] = count
    if sum(histogram.values()) != rejected:
        return unresolved

    if requested == 0:
        coverage_status = "empty_unverified"
        blocker = "same_fact_scope_empty_unverified"
    elif rejected > 0:
        coverage_status = "partial"
        blocker = "same_fact_eligibility_incomplete"
    else:
        coverage_status = "complete"
        blocker = ""
    return {
        "contract": SAME_FACT_ELIGIBILITY_VERSION,
        "key_version": SAME_FACT_KEY_VERSION,
        "coverage_status": coverage_status,
        "requested_row_count": requested,
        "fact_eligible_row_count": fact_eligible,
        "family_eligible_row_count": family_eligible,
        "rejected_row_count": rejected,
        "rejection_reason_histogram": dict(sorted(histogram.items())),
        "blocker": blocker,
    }


def _same_fact_family_cte_sql(where_sql: str) -> str:
    fact_rejection_reason_sql = _same_fact_txn_rejection_reason_sql(
        txn_id_expr="t.txn_id",
        row_id_expr="t.id",
        txn_ts_expr="t.txn_ts",
        direction_expr="t.dc_val",
        amount_expr="t.amount",
        balance_expr="t.balance",
        counterparty_account_expr="t.cp_key",
        counterparty_name_expr="t.cp_name",
        counterparty_raw_expr="t.cp_raw",
        summary_expr="t.summary",
        remark_expr="t.remark",
        txn_type_expr="t.txn_type",
        success_expr="t.is_success",
    )
    same_fact_key_sql = _same_fact_txn_key_sql(
        txn_id_expr="t.txn_id",
        row_id_expr="t.id",
        txn_ts_expr="t.txn_ts",
        direction_expr="t.dc_val",
        amount_expr="t.amount",
        balance_expr="t.balance",
        counterparty_account_expr="t.cp_key",
        counterparty_name_expr="t.cp_name",
        counterparty_raw_expr="t.cp_raw",
        summary_expr="t.summary",
        remark_expr="t.remark",
        txn_type_expr="t.txn_type",
        success_expr="t.is_success",
    )
    return f"""
        WITH scoped_input_base AS (
          SELECT
            COALESCE(NULLIF(TRIM(d.open_name), ''), NULLIF(TRIM(t.account_open_name), ''), '') AS holder_name,
            COALESCE(NULLIF(TRIM(d.id_no), ''), NULLIF(TRIM(t.opener_id_no), ''), '') AS id_no,
            TRIM(COALESCE(CAST(t.acct_key AS VARCHAR), '')) AS account_key,
            CAST(TRY_CAST(t.txn_ts AS TIMESTAMP) AS VARCHAR) AS txn_time_key,
            CASE WHEN TRY_CAST(t.amount AS DOUBLE) IS NOT NULL THEN printf('%.2f', TRY_CAST(t.amount AS DOUBLE)) END AS amount_key,
            CASE WHEN TRY_CAST(t.balance AS DOUBLE) IS NOT NULL THEN printf('%.2f', TRY_CAST(t.balance AS DOUBLE)) END AS balance_key,
            TRIM(COALESCE(CAST(t.dc_val AS VARCHAR), '')) AS direction_key,
            COALESCE(NULLIF(TRIM(CAST(t.cp_key AS VARCHAR)), ''), NULLIF(TRIM(CAST(t.counterparty_acct AS VARCHAR)), ''), NULLIF(TRIM(CAST(t.cp_raw AS VARCHAR)), ''), '') AS counterparty_account_key,
            COALESCE(NULLIF(TRIM(CAST(t.cp_name AS VARCHAR)), ''), NULLIF(TRIM(CAST(t.counterparty_name AS VARCHAR)), ''), '') AS counterparty_name_key,
            TRIM(COALESCE(CAST(t.summary AS VARCHAR), '')) AS summary_key,
            TRIM(COALESCE(CAST(t.remark AS VARCHAR), '')) AS remark_key,
            TRIM(COALESCE(CAST(t.txn_type AS VARCHAR), '')) AS txn_type_key,
            TRIM(COALESCE(CAST(t.txn_id AS VARCHAR), '')) AS txn_id,
            TRIM(COALESCE(CAST(t.file_id AS VARCHAR), '')) AS file_id,
            TRIM(COALESCE(CAST(t.id AS VARCHAR), '')) AS row_identity,
            ABS(TRY_CAST(t.amount AS DOUBLE)) AS abs_amount,
            {fact_rejection_reason_sql} AS fact_rejection_reason,
            {same_fact_key_sql} AS same_fact_key
          FROM analysis_txn_detail_idx t
          LEFT JOIN analysis_account_dim d ON d.account_key = t.acct_key
          WHERE {where_sql}
        ),
        scoped_input AS (
          SELECT
            *,
            COUNT(1) OVER (PARTITION BY row_identity) AS row_identity_count,
            CAST(LENGTH(holder_name) AS VARCHAR) || ':' || holder_name
              || '|' || CAST(LENGTH(id_no) AS VARCHAR) || ':' || id_no AS holder_identity_key
          FROM scoped_input_base
        ),
        scoped_txn AS (
          SELECT
            *,
            CASE
              WHEN fact_rejection_reason IS NOT NULL THEN fact_rejection_reason
              WHEN row_identity_count<>1 THEN 'duplicate_row_identity'
              WHEN NOT ({_valid_txn_id_sql("account_key")}) THEN 'account_key_missing_or_placeholder'
              WHEN NOT (({_valid_txn_id_sql("holder_name")}) OR ({_valid_txn_id_sql("id_no")})) THEN 'holder_identity_missing_or_placeholder'
              ELSE NULL
            END AS rejection_reason
          FROM scoped_input
        ),
        eligible_txn AS (
          SELECT *
          FROM scoped_txn
          WHERE rejection_reason IS NULL
        ),
        same_holder_families AS (
          SELECT
            holder_name,
            id_no,
            same_fact_key,
            MIN(txn_time_key) AS txn_time_key,
            MIN(amount_key) AS amount_key,
            MIN(balance_key) AS balance_key,
            MIN(direction_key) AS direction_key,
            MIN(counterparty_account_key) AS counterparty_account_key,
            CASE WHEN COUNT(DISTINCT NULLIF(counterparty_name_key, ''))=1 THEN MIN(NULLIF(counterparty_name_key, '')) ELSE '' END AS counterparty_name_key,
            MIN(summary_key) AS summary_key,
            MIN(remark_key) AS remark_key,
            MIN(txn_type_key) AS txn_type_key,
            COUNT(1) AS row_count,
            COUNT(DISTINCT account_key) AS account_count,
            COUNT(DISTINCT txn_id) AS txn_id_count,
            COUNT(DISTINCT file_id) FILTER (WHERE file_id <> '') AS source_file_count,
            SUM(abs_amount) AS gross_abs_amount,
            MAX(abs_amount) AS canonical_abs_amount,
            SUM(abs_amount) - MAX(abs_amount) AS candidate_duplicate_amount,
            list(DISTINCT account_key ORDER BY account_key)[:12] AS account_keys,
            list(DISTINCT holder_name ORDER BY holder_name)[:12] AS holder_names,
            list(DISTINCT txn_id ORDER BY txn_id)[:12] AS txn_ids,
            list(DISTINCT file_id ORDER BY file_id)[:12] AS source_file_ids
          FROM eligible_txn
          GROUP BY holder_name, id_no, same_fact_key
          HAVING COUNT(1) > 1 AND COUNT(DISTINCT account_key) > 1
        ),
        selected_scope_families AS (
          SELECT
            same_fact_key,
            CASE WHEN COUNT(DISTINCT holder_identity_key)=1 THEN MIN(holder_name) ELSE '' END AS holder_name,
            CASE WHEN COUNT(DISTINCT holder_identity_key)=1 THEN MIN(id_no) ELSE '' END AS id_no,
            COUNT(DISTINCT holder_identity_key) AS holder_identity_count,
            MIN(txn_time_key) AS txn_time_key,
            MIN(amount_key) AS amount_key,
            MIN(balance_key) AS balance_key,
            MIN(direction_key) AS direction_key,
            MIN(counterparty_account_key) AS counterparty_account_key,
            CASE WHEN COUNT(DISTINCT NULLIF(counterparty_name_key, ''))=1 THEN MIN(NULLIF(counterparty_name_key, '')) ELSE '' END AS counterparty_name_key,
            MIN(summary_key) AS summary_key,
            MIN(remark_key) AS remark_key,
            MIN(txn_type_key) AS txn_type_key,
            COUNT(1) AS row_count,
            COUNT(DISTINCT account_key) AS account_count,
            COUNT(DISTINCT txn_id) AS txn_id_count,
            COUNT(DISTINCT file_id) FILTER (WHERE file_id <> '') AS source_file_count,
            SUM(abs_amount) AS gross_abs_amount,
            MAX(abs_amount) AS canonical_abs_amount,
            SUM(abs_amount) - MAX(abs_amount) AS candidate_duplicate_amount,
            list(DISTINCT account_key ORDER BY account_key)[:12] AS account_keys,
            list(DISTINCT holder_name ORDER BY holder_name)[:12] AS holder_names,
            list(DISTINCT txn_id ORDER BY txn_id)[:12] AS txn_ids,
            list(DISTINCT file_id ORDER BY file_id)[:12] AS source_file_ids
          FROM eligible_txn
          GROUP BY same_fact_key
          HAVING COUNT(1) > 1 AND COUNT(DISTINCT account_key) > 1
        ),
        full_case_review_families AS (
          SELECT
            same_fact_key,
            MIN(txn_time_key) AS txn_time_key,
            MIN(amount_key) AS amount_key,
            MIN(balance_key) AS balance_key,
            MIN(direction_key) AS direction_key,
            MIN(counterparty_account_key) AS counterparty_account_key,
            CASE WHEN COUNT(DISTINCT NULLIF(counterparty_name_key, ''))=1 THEN MIN(NULLIF(counterparty_name_key, '')) ELSE '' END AS counterparty_name_key,
            MIN(summary_key) AS summary_key,
            MIN(remark_key) AS remark_key,
            MIN(txn_type_key) AS txn_type_key,
            COUNT(1) AS row_count,
            COUNT(DISTINCT account_key) AS account_count,
            COUNT(DISTINCT holder_identity_key) AS holder_identity_count,
            SUM(abs_amount) AS gross_abs_amount,
            MAX(abs_amount) AS canonical_abs_amount,
            SUM(abs_amount) - MAX(abs_amount) AS candidate_duplicate_amount
          FROM eligible_txn
          GROUP BY same_fact_key
          HAVING COUNT(1) > 1 AND COUNT(DISTINCT account_key) > 1
        )
    """

PASSIVE_MIXED_FUND_REPLENISHMENT_MARKERS: tuple[str, ...] = (
    "利息",
    "结息",
    "利息存入",
    "个人活期结息",
    "批量结息",
)

MEMORY_NOTE_SCOPE_WORKSPACE = "workspace"
MEMORY_NOTE_SCOPE_OPERATOR = "operator"
MEMORY_NOTE_SCOPES = {
    MEMORY_NOTE_SCOPE_WORKSPACE,
    MEMORY_NOTE_SCOPE_OPERATOR,
}
MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK = "workflow_feedback"
MEMORY_NOTE_TYPE_WORKSPACE_REFERENCE = "workspace_reference"
MEMORY_NOTE_TYPE_PROJECT_SIGNAL = "project_signal"
MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE = "operator_preference"
MEMORY_NOTE_TYPES_BY_SCOPE: dict[str, tuple[str, ...]] = {
    MEMORY_NOTE_SCOPE_WORKSPACE: (
        MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK,
        MEMORY_NOTE_TYPE_WORKSPACE_REFERENCE,
        MEMORY_NOTE_TYPE_PROJECT_SIGNAL,
    ),
    MEMORY_NOTE_SCOPE_OPERATOR: (
        MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE,
        MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK,
    ),
}
MEMORY_NOTE_FRESHNESS_STABLE = "stable"
MEMORY_NOTE_FRESHNESS_TIME_SENSITIVE = "time_sensitive"
MEMORY_NOTE_FRESHNESS_VOLATILE = "volatile"
MEMORY_NOTE_FRESHNESS_VALUES = {
    MEMORY_NOTE_FRESHNESS_STABLE,
    MEMORY_NOTE_FRESHNESS_TIME_SENSITIVE,
    MEMORY_NOTE_FRESHNESS_VOLATILE,
}
MEMORY_NOTE_TRUST_CONFIRMED = "confirmed"
MEMORY_NOTE_TRUST_WORKING = "working"
MEMORY_NOTE_TRUST_PREFERENCE = "preference"
MEMORY_NOTE_TRUST_VALUES = {
    MEMORY_NOTE_TRUST_CONFIRMED,
    MEMORY_NOTE_TRUST_WORKING,
    MEMORY_NOTE_TRUST_PREFERENCE,
}
MEMORY_NOTE_DEFAULT_FRESHNESS_BY_TYPE: dict[str, str] = {
    MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK: MEMORY_NOTE_FRESHNESS_TIME_SENSITIVE,
    MEMORY_NOTE_TYPE_WORKSPACE_REFERENCE: MEMORY_NOTE_FRESHNESS_STABLE,
    MEMORY_NOTE_TYPE_PROJECT_SIGNAL: MEMORY_NOTE_FRESHNESS_VOLATILE,
    MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE: MEMORY_NOTE_FRESHNESS_STABLE,
}
MEMORY_NOTE_DEFAULT_TRUST_BY_TYPE: dict[str, str] = {
    MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK: MEMORY_NOTE_TRUST_WORKING,
    MEMORY_NOTE_TYPE_WORKSPACE_REFERENCE: MEMORY_NOTE_TRUST_CONFIRMED,
    MEMORY_NOTE_TYPE_PROJECT_SIGNAL: MEMORY_NOTE_TRUST_WORKING,
    MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE: MEMORY_NOTE_TRUST_PREFERENCE,
}
MEMORY_NOTE_DEFAULT_REVALIDATION_BY_TYPE: dict[str, bool] = {
    MEMORY_NOTE_TYPE_WORKFLOW_FEEDBACK: True,
    MEMORY_NOTE_TYPE_WORKSPACE_REFERENCE: False,
    MEMORY_NOTE_TYPE_PROJECT_SIGNAL: True,
    MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE: False,
}
MEMORY_NOTE_INDEX_DETAIL_FALLBACK_LENGTH = 160


def _is_transient_case_db_conflict(exc: Exception) -> bool:
    message = str(exc or "").lower()
    return (
        "unique file handle conflict" in message
        or "already attached by database" in message
        or "same database file with a different configuration" in message
        or "could not set lock on file" in message
        or "conflicting lock is held" in message
        or "cannot open file" in message
        or "being used by another process" in message
        or "another process is using" in message
        or "另一个程序正在使用此文件" in message
        or "进程无法访问" in message
        or "transactioncontext error" in message
        or "conflict on update" in message
    )

REPORT_TEMPLATE_PRESETS: dict[str, dict[str, Any]] = {
    "standard": {
        "template_label": "标准研判",
        "description": "适合案件工作区内的通用阶段报告。",
        "default_section_plan": ["案件概况", "事实", "推断", "待核实", "建议动作", "签发位"],
        "template_config": {
            "template_id": "standard",
            "template_label": "标准研判",
            "organization_name": "",
            "header_title": "案件资金研判报告",
            "header_subtitle": "阶段性工作底稿",
            "footer_note": "内部研判使用",
            "page_number_style": "cn_simple",
            "signature_label": "签发",
            "signature_name": "",
            "signature_title": "经办分析员",
            "signature_date": "",
            "show_signature_block": True,
        },
    },
    "regulatory_brief": {
        "template_label": "监管简报",
        "description": "强调机构抬头、核心发现和合规化签发位。",
        "default_section_plan": ["案件概况", "核心发现", "资金路径", "风险判断", "待核实", "建议动作", "签发位"],
        "template_config": {
            "template_id": "regulatory_brief",
            "template_label": "监管简报",
            "organization_name": "",
            "header_title": "案件资金研判简报",
            "header_subtitle": "监管核查专用",
            "footer_note": "供复核与审批引用",
            "page_number_style": "compact",
            "signature_label": "签发意见",
            "signature_name": "",
            "signature_title": "审批负责人",
            "signature_date": "",
            "show_signature_block": True,
        },
    },
    "executive_digest": {
        "template_label": "领导摘要",
        "description": "突出摘要、关键结论和待决事项，适合领导阅示。",
        "default_section_plan": ["摘要", "关键结论", "资金路径", "风险提示", "待决事项", "签发位"],
        "template_config": {
            "template_id": "executive_digest",
            "template_label": "领导摘要",
            "organization_name": "",
            "header_title": "案件资金研判摘要",
            "header_subtitle": "重点情况汇报",
            "footer_note": "请结合附件与证据索引阅示",
            "page_number_style": "arabic",
            "signature_label": "呈报",
            "signature_name": "",
            "signature_title": "项目负责人",
            "signature_date": "",
            "show_signature_block": True,
        },
    },
}

KEY_NODE_SCORE_VERSION = "1.0.0"
KEY_NODE_FEATURE_VERSION = "1.1.0"
KEY_NODE_WEIGHT_PROFILE = {"amount": 0.65, "frequency": 0.35}
# Bump when scope-level statistics or answer-facing summary semantics change so stale
# cached payloads do not keep reviving older account-analysis conclusions.
SIGNAL_FEATURE_VERSION = "1.1.0"
TEMP_SCOPE_ACTIVE_LIMIT = 16
TEMP_SCOPE_TTL_HOURS = 24
TEMP_SCOPE_AUDIT_RETENTION_DAYS = 30
_TEMP_SCOPE_SOURCE_KINDS = frozenset(
    {"uploaded_file", "document_selection", "mixed_upload"}
)
_TEMP_SCOPE_FAILURE_CODES = frozenset(
    {
        "source_not_registered",
        "source_path_unavailable",
        "excel_prepare_failed",
        "csv_header_read_failed",
        "source_kind_unsupported",
        "no_transaction_sheet",
        "temp_import_failed",
    }
)
_TEMP_SCOPE_RETRYABLE_FAILURE_CODES = frozenset(
    {"source_path_unavailable", "excel_prepare_failed", "csv_header_read_failed"}
)
_TEMP_SCOPE_WARNING_CODES = frozenset(
    {
        "TEMP_SCOPE_TTL_EXPIRED",
        "TEMP_SCOPE_QUOTA_TRIMMED",
        "TEMP_SCOPE_AUDIT_PURGED",
    }
)


def _stable_slug(prefix: str, *parts: Any) -> str:
    digest = hashlib.sha1("|".join(_trim(part) for part in parts).encode("utf-8")).hexdigest()[:16]
    return f"{prefix}_{digest}"


def _unique_texts(values: Iterable[Any]) -> list[str]:
    items: list[str] = []
    seen: set[str] = set()
    for value in values:
        text = _trim(value)
        if not text or text in seen:
            continue
        seen.add(text)
        items.append(text)
    return items


def _reconcile_count_status(left: Any, right: Any) -> str:
    left_count = _optional_nonnegative_int(left)
    right_count = _optional_nonnegative_int(right)
    if left_count is None or right_count is None:
        return "unavailable"
    return "matched" if left_count == right_count else "mismatch"


def _coalesce_text_expr(available_columns: set[str], candidates: Sequence[str]) -> str:
    expressions = [
        f"NULLIF(TRIM(CAST({column} AS VARCHAR)), '')"
        for column in candidates
        if column in available_columns
    ]
    if not expressions:
        return ""
    return f"COALESCE({', '.join(expressions)})"


def _clip_items(values: Iterable[Any], limit: int) -> list[str]:
    return _unique_texts(values)[: max(1, int(limit))]


def _clamp_score(value: float, *, low: float = 0.0, high: float = 0.99) -> float:
    return round(min(high, max(low, float(value))), 2)


def _quality_label_from_score(score: float) -> str:
    if score >= 0.85:
        return "high"
    if score >= 0.65:
        return "medium"
    return "low"


def _quality_label_display(label: Any) -> str:
    normalized = _trim(label).lower()
    if normalized == "high":
        return "较高"
    if normalized == "medium":
        return "中等"
    if normalized == "low":
        return "较低"
    return "未知"


def _summarize_field_quality(field_coverage: dict[str, Any]) -> dict[str, Any]:
    coverage = {
        str(key): _optional_confidence(value)
        for key, value in dict(field_coverage or {}).items()
    }
    weighted_fields = {
        "counterparty": 0.22,
        "summary": 0.14,
        "remark": 0.12,
        "branch_name": 0.12,
        "location": 0.10,
        "success_status": 0.10,
        "ip_addr": 0.10,
        "mac_addr": 0.10,
    }
    weighted_score = 0.0
    weighted_total = 0.0
    weighted_complete = True
    low_fields: list[str] = []
    missing_rates: dict[str, float | None] = {}
    for field, weight in weighted_fields.items():
        rate = coverage.get(field)
        if rate is None:
            weighted_complete = False
            missing_rates[field] = None
            continue
        weighted_score += rate * weight
        weighted_total += weight
        missing_rates[field] = round(1.0 - rate, 4)
        if rate < 0.5:
            low_fields.append(field)
    score = (
        round(weighted_score / weighted_total, 4)
        if weighted_complete and weighted_total > 0
        else None
    )
    notes: list[str] = []
    if not weighted_complete:
        notes.append("字段覆盖率未完整核验，不能生成确定性质量评分。")
    if (counterparty_rate := coverage.get("counterparty")) is not None and counterparty_rate < 0.6:
        notes.append("对手识别覆盖率偏低，未知对手判断需保留边界。")
    summary_rate = coverage.get("summary")
    remark_rate = coverage.get("remark")
    if summary_rate is not None and remark_rate is not None and summary_rate < 0.5 and remark_rate < 0.5:
        notes.append("摘要与备注可用率偏低，文本语义型判断需降权。")
    ip_rate = coverage.get("ip_addr")
    mac_rate = coverage.get("mac_addr")
    if (
        ip_rate is not None
        and mac_rate is not None
        and 0.03 <= max(ip_rate, mac_rate) < 0.4
        and ip_rate < 0.4
        and mac_rate < 0.4
    ):
        notes.append("设备字段缺失较多，设备共现类判断不宜拉高置信度。")
    branch_rate = coverage.get("branch_name")
    location_rate = coverage.get("location")
    if branch_rate is not None and location_rate is not None and branch_rate < 0.4 and location_rate < 0.4:
        notes.append("网点与地点字段覆盖不足，空间异常判断需谨慎。")
    cash_rate = coverage.get("cash_flag")
    if cash_rate is not None and cash_rate < 0.4:
        notes.append("现金标记字段覆盖不足，存取现判断需保留边界。")
    label = _quality_label_from_score(score) if score is not None else "unknown"
    return {
        "label": label,
        "label_display": _quality_label_display(label),
        "score": score,
        "field_coverage": coverage,
        "missing_rates": missing_rates,
        "low_fields": low_fields,
        "notes": notes,
    }


def _coerce_datetime(value: Any) -> Optional[datetime]:
    if isinstance(value, datetime):
        return value.replace(tzinfo=None) if value.tzinfo is not None else value
    text = _trim(value)
    if not text:
        return None
    normalized = text.replace("Z", "+00:00")
    for candidate in (normalized, normalized.replace("T", " ")):
        try:
            parsed = datetime.fromisoformat(candidate)
            return parsed.replace(tzinfo=None) if parsed.tzinfo is not None else parsed
        except Exception:
            continue
    for pattern in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%d %H:%M", "%Y-%m-%d"):
        try:
            return datetime.strptime(text, pattern)
        except Exception:
            continue
    return None


def _datetime_text(value: Any) -> str:
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    return _trim(value)


def _normalize_direction(value: Any) -> str:
    raw = _trim(value).lower()
    if not raw:
        return "unknown"
    if raw in {"in", "credit", "incoming"}:
        return "in"
    if raw in {"out", "debit", "outgoing"}:
        return "out"
    if any(token in raw for token in ("进", "入", "贷", "收")):
        return "in"
    if any(token in raw for token in ("出", "付", "借", "支")):
        return "out"
    return "unknown"


def _normalize_success(value: Any) -> Optional[bool]:
    raw = _trim(value).lower()
    if not raw:
        return None
    if raw in {"1", "true", "t", "y", "yes"}:
        return True
    if raw in {"0", "false", "f", "n", "no"}:
        return False
    if any(token in raw for token in ("成功", "通过", "succ", "ok")):
        return True
    if any(token in raw for token in ("失败", "拒绝", "异常", "超时", "fail", "error")):
        return False
    return None


def _normalize_reason(value: Any) -> str:
    return " ".join(_trim(value).upper().split())


def _normalize_ip(value: Any) -> str:
    return _trim(value)


def _normalize_mac(value: Any) -> str:
    raw = "".join(ch for ch in _trim(value).upper() if ch.isalnum())
    return raw


def _normalize_bool(value: Any) -> Optional[bool]:
    if isinstance(value, bool):
        return value
    raw = _trim(value).lower()
    if not raw:
        return None
    if raw in {"0", "false", "f", "no", "n", "noncash", "no_cash", "nocash", "非现金", "否"}:
        return False
    if raw in {"1", "true", "t", "yes", "y", "cash", "现金", "是"}:
        return True
    return None


_CASH_NEGATIVE_CUES = (
    "非现金",
    "不含现金",
    "无现金",
    "非现钞",
    "不含现钞",
    "无现钞",
    "noncash",
    "no cash",
)
_CASH_POSITIVE_CUES = ("现金", "现钞", "存现", "取现", "cash")
_CASH_DEPENDENT_RULE_CODES = {
    "SMALL_FAST_IN_OUT",
    "SINGLE_FAST_IN_OUT_CANDIDATE",
    "CASH_QUICK_IN_OUT",
    "SINGLE_CASH_QUICK_IN_OUT_CANDIDATE",
}


def _cash_text_cues(row: Mapping[str, Any]) -> tuple[bool, bool]:
    source = " ".join(
        _trim(row.get(field)).lower()
        for field in ("summary", "txn_type", "remark", "voucher_type")
    )
    negative = any(marker in source for marker in _CASH_NEGATIVE_CUES)
    positive_source = source
    for marker in _CASH_NEGATIVE_CUES:
        positive_source = positive_source.replace(marker, " ")
    positive = any(marker in positive_source for marker in _CASH_POSITIVE_CUES)
    return positive, negative


def _cash_txn_state(row: Mapping[str, Any]) -> str:
    normalized = row.get("cash_flag_norm")
    positive_cue, negative_cue = _cash_text_cues(row)
    if type(normalized) is bool:
        if (normalized and negative_cue) or (not normalized and positive_cue):
            return "conflict"
        return "cash" if normalized else "non_cash"
    return "unknown"


def _cash_rule_input_coverage(txn_rows: Sequence[Mapping[str, Any]]) -> dict[str, Any]:
    counts = {"cash": 0, "non_cash": 0, "unknown": 0, "conflict": 0}
    for row in txn_rows:
        state = _cash_txn_state(row)
        counts[state] += 1
    total_rows = len(txn_rows)
    classified_rows = counts["cash"] + counts["non_cash"]
    return {
        "status": (
            "complete"
            if total_rows > 0 and classified_rows == total_rows
            else ("partial" if total_rows > 0 else "unresolved")
        ),
        "total_rows": total_rows,
        "classified_rows": classified_rows,
        "cash_rows": counts["cash"],
        "non_cash_rows": counts["non_cash"],
        "unknown_rows": counts["unknown"],
        "conflict_rows": counts["conflict"],
    }


def _direction_label(direction: str) -> str:
    if direction == "in":
        return "流入"
    if direction == "out":
        return "流出"
    return "交易"


_CHANNEL_COUNTERPARTY_MARKERS = (
    "财付通",
    "微信",
    "支付宝",
    "云闪付",
    "银联",
    "支付",
    "快捷支付",
    "零钱",
    "扫码",
    "红包",
)

_CASH_BEHAVIOR_MARKERS = (
    "ATM",
    "现金",
    "取款",
    "取现",
    "存款",
    "存入",
    "自助",
)

_STRICT_CASH_BEHAVIOR_MARKERS = (
    "ATM",
    "取款",
    "取现",
    "存款",
    "存现",
    "现金存入",
    "现金取款",
    "现金支取",
)

_STRICT_CASH_BEHAVIOR_EXCLUDE_MARKERS = (
    "现金分期",
    "无卡自助消费",
    "贷款还款",
    "还款",
    "补款转入",
    "转账存入",
    "转帐存入",
    "消费",
)

_INVESTMENT_BEHAVIOR_MARKERS = (
    "基金",
    "理财",
    "投资",
    "保险",
    "证券",
)

_ASSET_PURCHASE_MARKERS = (
    "购房",
    "房款",
    "房产",
    "不动产",
    "首付",
    "尾款",
    "按揭",
    "车款",
    "购车",
    "买车",
    "汽车",
    "4S",
    "车辆",
)

_HIGH_CONSUMPTION_MARKERS = (
    "高消费",
    "奢侈",
    "珠宝",
    "首饰",
    "黄金",
    "名表",
    "名牌",
    "会所",
    "高尔夫",
    "旅游",
    "酒店",
    "消费",
)

_PROPERTY_PARKING_MARKERS = (
    "物业",
    "物业费",
    "停车",
    "停车费",
    "车位",
    "小区",
)


def _normalize_counterparty_display_name(counterparty_key: str, alias_names: Sequence[str]) -> str:
    normalized_aliases = [item for item in (_trim(name) for name in alias_names) if item]
    if not normalized_aliases:
        return _trim(counterparty_key) or "未知对手"
    channel_aliases = [name for name in normalized_aliases if any(marker in name for marker in _CHANNEL_COUNTERPARTY_MARKERS)]
    if len(channel_aliases) >= 2:
        if any("微信" in name for name in channel_aliases) and any("财付通" in name for name in channel_aliases):
            return "财付通/微信通道"
    preferred = sorted(
        normalized_aliases,
        key=lambda item: (
            any(marker in item for marker in ("未知", "__unknown__")),
            item.isdigit(),
            len(item),
            item,
        ),
    )
    return preferred[0] if preferred else (_trim(counterparty_key) or "未知对手")


def _classify_counterparty_type(counterparty_key: str, display_name: str) -> str:
    normalized_key = _trim(counterparty_key).lower()
    normalized_name = _trim(display_name)
    lowered_name = normalized_name.lower()
    if normalized_key in {"__unknown__", ""} or lowered_name in {"未知", "未知对手", "__unknown__", "unknown"}:
        return "unknown"
    if any(marker in normalized_name for marker in _CHANNEL_COUNTERPARTY_MARKERS):
        return "channel"
    if any(token in normalized_name for token in ("有限公司", "公司", "银行", "店", "商行", "超市", "科技", "网络", "平台", "工厂")):
        return "merchant"
    if normalized_name and 2 <= len(normalized_name) <= 6 and not any(ch.isdigit() for ch in normalized_name):
        return "person"
    if normalized_key.isdigit():
        return "account_only"
    return "other"


def _counterparty_type_label(counterparty_type: str) -> str:
    normalized = _trim(counterparty_type).lower()
    if normalized == "unknown":
        return "未识别"
    if normalized == "channel":
        return "支付通道"
    if normalized == "merchant":
        return "商户/机构"
    if normalized == "person":
        return "个人"
    if normalized == "account_only":
        return "仅账号"
    return "其他"


def _counterparty_account_value(counterparty_key: str) -> str:
    normalized_key = _trim(counterparty_key)
    if not normalized_key or normalized_key.startswith("__"):
        return ""
    return normalized_key


def _direction_overview_text(items: Sequence[dict[str, Any]], *, fallback: str) -> str:
    leaders = [dict(item or {}) for item in list(items)[:3] if isinstance(item, dict)]
    if not leaders:
        return fallback
    segments: list[str] = []
    for item in leaders:
        name = _trim(item.get("display_name")) or "未知对手"
        amount = _optional_rounded_float(item.get("total_amount"), 2)
        amount_text = f"{amount} 元" if amount is not None else "金额未核验"
        segments.append(f"{name}（{amount_text}）")
    return "、".join(segments)


def _normalize_rank_metric(value: Any) -> str:
    normalized = _trim(value).lower()
    aliases = {
        "in": "inflow",
        "income": "inflow",
        "in_amount": "inflow",
        "inflow_total": "inflow",
        "out": "outflow",
        "expense": "outflow",
        "out_amount": "outflow",
        "outflow_total": "outflow",
        "amount": "turnover",
        "total": "turnover",
        "turnover_total": "turnover",
        "count": "txn_count",
        "txn": "txn_count",
        "txns": "txn_count",
        "max": "max_single_amount",
        "max_amount": "max_single_amount",
        "single": "max_single_amount",
    }
    normalized = aliases.get(normalized, normalized)
    if normalized not in {"inflow", "outflow", "turnover", "txn_count", "max_single_amount"}:
        return "turnover"
    return normalized


def _rank_metric_field(metric: str) -> str:
    return {
        "inflow": "inflow_total",
        "outflow": "outflow_total",
        "turnover": "turnover_total",
        "txn_count": "txn_count",
        "max_single_amount": "max_single_amount",
    }.get(_normalize_rank_metric(metric), "turnover_total")


def _sort_rank_rows(rows: Sequence[dict[str, Any]], *, metric: str, tie_field: str) -> list[dict[str, Any]]:
    metric_field = _rank_metric_field(metric)
    eligible_rows: list[tuple[dict[str, Any], float]] = []
    for raw_row in rows:
        row = dict(raw_row or {})
        if metric_field == "txn_count":
            metric_value = _optional_nonnegative_int(row.get(metric_field))
        else:
            metric_value = _optional_finite_float(row.get(metric_field))
            if metric_value is not None and metric_value < 0:
                metric_value = None
        if metric_value is not None:
            eligible_rows.append((row, float(metric_value)))

    def _key(entry: tuple[dict[str, Any], float]) -> tuple[Any, ...]:
        item, metric_value = entry
        turnover = _optional_finite_float(item.get("turnover_total"))
        txn_count = _optional_nonnegative_int(item.get("txn_count"))
        return (
            -metric_value,
            turnover is None,
            -(turnover if turnover is not None else 0.0),
            txn_count is None,
            -(txn_count if txn_count is not None else 0),
            str(item.get(tie_field) or ""),
        )

    ordered = sorted(
        eligible_rows,
        key=_key,
    )
    return [{"rank": index, **item} for index, (item, _metric_value) in enumerate(ordered, start=1)]


def _merge_rank_scope_warnings(scope: dict[str, Any], lifecycle_warnings: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
    warnings: list[dict[str, Any]] = []
    seen: set[tuple[str, str]] = set()
    for item in [*list(scope.get("warnings") or []), *list(lifecycle_warnings or [])]:
        warning = dict(item or {})
        code = _trim(warning.get("code"))
        message = _trim(warning.get("message"))
        if not message:
            continue
        key = (code, message)
        if key in seen:
            continue
        seen.add(key)
        warnings.append({"code": code, "message": message, "severity": _trim(warning.get("severity")) or "info"})
    return warnings


def _sql_text_marker_match(expressions: Sequence[str], markers: Sequence[str]) -> str:
    clauses: list[str] = []
    for expression in expressions:
        normalized_expression = _trim(expression)
        if not normalized_expression:
            continue
        for marker in markers:
            normalized_marker = _trim(marker)
            if not normalized_marker:
                continue
            escaped_marker = normalized_marker.replace("'", "''")
            if normalized_marker.upper() == normalized_marker and normalized_marker.isascii():
                clauses.append(f"UPPER(COALESCE({normalized_expression}, '')) LIKE '%{escaped_marker.upper()}%'")
            else:
                clauses.append(f"COALESCE({normalized_expression}, '') LIKE '%{escaped_marker}%'")
    return " OR ".join(clauses) if clauses else "FALSE"


def _fund_business_category_sql(expressions: Sequence[str] = ("cp_name", "cp_raw", "summary", "remark", "txn_type", "cash_flag", "branch_name")) -> str:
    financial_sql = _sql_text_marker_match(expressions, _INVESTMENT_BEHAVIOR_MARKERS + ("申购", "赎回", "认购"))
    channel_sql = _sql_text_marker_match(expressions, _CHANNEL_COUNTERPARTY_MARKERS)
    litigation_sql = _sql_text_marker_match(expressions, ("法院", "执行", "司法", "扣划", "案款", "诉讼", "律师"))
    asset_sql = _sql_text_marker_match(expressions, _ASSET_PURCHASE_MARKERS + ("物业", "停车", "车位", "保险"))
    cash_sql = _sql_text_marker_match(expressions, _CASH_BEHAVIOR_MARKERS + ("户名缺失", "续取", "卡取", "POS", "销户"))
    return (
        "CASE "
        f"WHEN ({financial_sql}) THEN 'financial_product' "
        f"WHEN ({channel_sql}) THEN 'third_party_payment' "
        f"WHEN ({litigation_sql}) THEN 'litigation_enforcement' "
        f"WHEN ({asset_sql}) THEN 'asset_consumption' "
        f"WHEN ({cash_sql}) THEN 'cash_breakpoint' "
        "ELSE 'transfer_unclassified' END"
    )


def _fund_business_category_label(category: Any) -> str:
    normalized_category = _trim(category)
    if normalized_category == "project_business":
        return "普通转账/未分类"
    return {
        "financial_product": "理财基金证券",
        "third_party_payment": "三方支付通道",
        "litigation_enforcement": "法院/执行/扣划",
        "asset_consumption": "资产购置/消费",
        "cash_breakpoint": "现金/户名缺失/柜面断点",
        "transfer_unclassified": "普通转账/未分类",
    }.get(normalized_category, normalized_category or "未分类")


def _sql_text_marker_exclusion(expressions: Sequence[str], markers: Sequence[str]) -> str:
    clauses: list[str] = []
    for expression in expressions:
        normalized_expression = _trim(expression)
        if not normalized_expression:
            continue
        for marker in markers:
            normalized_marker = _trim(marker)
            if not normalized_marker:
                continue
            escaped_marker = normalized_marker.replace("'", "''")
            if normalized_marker.upper() == normalized_marker and normalized_marker.isascii():
                clauses.append(f"UPPER(COALESCE({normalized_expression}, '')) LIKE '%{escaped_marker.upper()}%'")
            else:
                clauses.append(f"COALESCE({normalized_expression}, '') LIKE '%{escaped_marker}%'")
    if not clauses:
        return "TRUE"
    return "NOT (" + " OR ".join(clauses) + ")"


def _mode_label(mode_key: str) -> str:
    labels = {
        "same_name_transfer": "同名账户往来",
        "cash_deposit": "现金/ATM存入",
        "cash_withdrawal": "现金/ATM取现",
        "channel": "第三方支付通道",
        "investment": "理财/基金/保险",
        "unknown_other": "未知对手其他交易",
        "merchant": "商户消费/经营往来",
        "other": "其他转账往来",
    }
    return labels.get(_trim(mode_key), _trim(mode_key) or "其他交易")


def _bucket_overview_text(items: Sequence[dict[str, Any]], *, fallback: str) -> str:
    leaders = [dict(item or {}) for item in list(items)[:3] if isinstance(item, dict)]
    if not leaders:
        return fallback
    segments: list[str] = []
    for item in leaders:
        label = _trim(item.get("label") or item.get("display_name") or item.get("bucket_value")) or "其他"
        total_amount = _optional_rounded_float(item.get("total_amount"), 2)
        txn_count = _optional_nonnegative_int(item.get("txn_count"))
        count_text = f"{txn_count} 笔" if txn_count is not None else "笔数未核验"
        amount_text = f"{total_amount} 元" if total_amount is not None else "金额未核验"
        segments.append(f"{label}（{count_text}，{amount_text}）")
    return "、".join(segments)


def _encode_offset_cursor(offset: int) -> str:
    payload = json.dumps({"offset": max(0, int(offset))}, separators=(",", ":")).encode("utf-8")
    return base64.urlsafe_b64encode(payload).decode("utf-8")


def _decode_offset_cursor(cursor: str) -> int:
    text = _trim(cursor)
    if not text:
        return 0
    try:
        payload = json.loads(base64.urlsafe_b64decode(text.encode("utf-8")).decode("utf-8"))
        return max(0, int(payload.get("offset") or 0))
    except Exception:
        return 0


def _now_text() -> str:
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def _utc_now_text() -> str:
    return datetime.now(timezone.utc).isoformat()


def _today_text() -> str:
    return date.today().isoformat()


def _json_dumps(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False)


def _json_loads(value: Any, default: Any) -> Any:
    if value in (None, ""):
        return default
    try:
        return json.loads(str(value))
    except Exception:
        return default


def _trim(value: Any) -> str:
    return str(value or "").strip()


def _file_sha256(path: Path) -> str:
    if not path.exists():
        return ""
    try:
        return hashlib.sha256(path.read_bytes()).hexdigest()
    except OSError:
        return ""


def _is_lower_sha256_hex(value: Any) -> bool:
    return (
        type(value) is str
        and len(value) == 64
        and all(character in "0123456789abcdef" for character in value)
    )


def _trim_list(values: Iterable[Any]) -> list[str]:
    return [item for item in (_trim(value) for value in values) if item]


def _account_aliases(values: Iterable[Any]) -> list[str]:
    aliases: set[str] = set()
    for value in values:
        text = _trim(value)
        if not text:
            continue
        aliases.add(text)
        aliases.add(re.sub(r"(CNY|RMB|USD|HKD|EUR|JPY|GBP|AUD|CAD|SGD)0?$", "", text, flags=re.IGNORECASE))
    return sorted(alias for alias in aliases if alias)


def _truncate_text(value: Any, max_length: int = 240) -> str:
    text = _trim(value)
    if len(text) <= max_length:
        return text
    return f"{text[:max_length]}..."


def _build_llm_memory_note_index_text(
    *,
    title: str,
    summary: str,
    detail: str,
    memory_type: str,
    freshness: str,
    trust_level: str,
    required_revalidation: bool,
    tags: Sequence[str],
) -> str:
    detail_fallback = _truncate_text(detail, MEMORY_NOTE_INDEX_DETAIL_FALLBACK_LENGTH) if detail and not summary else ""
    return "\n".join(
        part
        for part in [
            title,
            summary,
            detail_fallback,
            memory_type,
            freshness,
            trust_level,
            "required_revalidation" if required_revalidation else "",
            " ".join(_trim_list(tags)),
        ]
        if part
    )


def _slug(prefix: str) -> str:
    return f"{prefix}_{uuid.uuid4().hex[:12]}"


def _closed_audit_record(
    case_id: str,
    event: Mapping[str, Any],
    *,
    generate_event_id: bool,
) -> dict[str, Any]:
    public = project_operational_event(event=event, run_log=False, case_id=case_id)
    return {
        "event_id": public["event_id"] or (_slug("audit") if generate_event_id else ""),
        "case_id": _trim(case_id),
        "event_type": public["event_type"],
        "task_id": public["task_id"],
        "run_id": public["run_id"],
        "turn_id": public["turn_id"],
        "artifact_id": public["artifact_id"],
        "approval_id": public["approval_id"],
        "trace_id": public["trace_id"],
        "span_id": public["span_id"],
        "actor_id": public["actor_id"],
        "actor_role": public["actor_role"],
        "tenant_id": public["tenant_id"],
        "request_id": public["request_id"],
        "session_id": public["session_id"],
        "timestamp": public["timestamp"] or (_utc_now_text() if generate_event_id else ""),
        "payload": public["payload"],
    }


def _closed_run_log_record(
    case_id: str,
    event: Mapping[str, Any],
    *,
    sequence_no: int,
    generate_event_id: bool,
) -> dict[str, Any]:
    public = project_operational_event(event=event, run_log=True, case_id=case_id)
    return {
        "event_id": public["event_id"] or (_slug("runlog") if generate_event_id else ""),
        "case_id": _trim(case_id),
        "task_id": public["task_id"],
        "run_id": public["run_id"],
        "turn_id": public["turn_id"],
        "sequence_no": max(1, _as_int(sequence_no)),
        "event_stage": public["event_stage"],
        "event_type": public["event_type"],
        "source": public["source"],
        "created_at": public["timestamp"] or (_utc_now_text() if generate_event_id else ""),
        "payload": public["payload"],
    }


def _as_int(value: Any) -> int:
    number = _optional_int(value)
    if number is None:
        raise ValueError("analysis_integer_unknown")
    return number


def _as_float(value: Any) -> float:
    number = _optional_finite_float(value)
    if number is None:
        raise ValueError("analysis_number_unknown")
    return number


def _optional_finite_float(value: Any) -> float | None:
    """Parse a factual number without treating missing/invalid input as zero."""

    if value is None or isinstance(value, bool):
        return None
    if isinstance(value, str) and not value.strip():
        return None
    try:
        number = float(value)
    except (TypeError, ValueError, OverflowError):
        return None
    return number if math.isfinite(number) else None


def _optional_int(value: Any) -> int | None:
    """Parse an integer fact without accepting bools, fractions, or defaults."""

    if value is None or isinstance(value, bool):
        return None
    if isinstance(value, str) and not value.strip():
        return None
    try:
        number = float(value)
    except (TypeError, ValueError, OverflowError):
        return None
    if not math.isfinite(number) or not number.is_integer():
        return None
    return int(number)


def _optional_nonnegative_int(value: Any) -> int | None:
    number = _optional_int(value)
    return number if number is not None and number >= 0 else None


def _exact_nonnegative_db_int(value: Any) -> int | None:
    if type(value) is not int or value < 0 or value > 9_007_199_254_740_991:
        return None
    return value


def _exact_single_query_row(value: Any, *, width: int) -> Sequence[Any] | None:
    if (
        type(value) is not list
        or len(value) != 1
        or type(value[0]) not in (list, tuple)
        or len(value[0]) != width
    ):
        return None
    return value[0]


def _optional_positive_int(value: Any) -> int | None:
    number = _optional_int(value)
    return number if number is not None and number > 0 else None


def _first_known_value(*values: Any) -> Any:
    for value in values:
        if value is not None:
            return value
    return None


def _optional_bool(value: Any) -> bool | None:
    if isinstance(value, bool):
        return value
    if isinstance(value, int) and value in {0, 1}:
        return bool(value)
    normalized = _trim(value).lower()
    if normalized in {"true", "1"}:
        return True
    if normalized in {"false", "0"}:
        return False
    return None


def _optional_rounded_float(value: Any, digits: int) -> float | None:
    number = _optional_finite_float(value)
    return round(number, digits) if number is not None else None


def _optional_confidence(value: Any) -> float | None:
    number = _optional_finite_float(value)
    if number is None or number < 0 or number > 1:
        return None
    return round(number, 4)


def _normalize_rule_params(params: Mapping[str, Any] | None) -> dict[str, Any]:
    """Apply absent-key defaults while rejecting every explicit invalid override."""

    overrides = _validate_rule_param_overrides(params)
    normalized = dict(RULE_PARAM_DEFAULTS)
    normalized.update(overrides)
    for upper_key, lower_key in _RULE_PARAM_ORDERED_BOUNDS:
        if float(normalized[upper_key]) < float(normalized[lower_key]):
            raise ValueError(f"rule_parameter_invalid:{upper_key}")
    return normalized


def _validate_rule_param_overrides(params: Mapping[str, Any] | None) -> dict[str, Any]:
    if params is None:
        return {}
    if not isinstance(params, Mapping):
        raise ValueError("rule_parameters_invalid")
    normalized: dict[str, Any] = {}
    for raw_key, value in params.items():
        if not isinstance(raw_key, str) or not raw_key.strip() or raw_key != raw_key.strip():
            raise ValueError("rule_parameter_key_invalid")
        key = raw_key
        if key not in RULE_PARAM_DEFAULTS:
            raise ValueError(f"rule_parameter_unknown:{key}")
        default = RULE_PARAM_DEFAULTS[key]
        if isinstance(default, str):
            if not isinstance(value, str) or not value.strip():
                raise ValueError(f"rule_parameter_invalid:{key}")
            normalized[key] = value.strip()
            continue
        if isinstance(default, int) and not isinstance(default, bool):
            if type(value) is not int:
                raise ValueError(f"rule_parameter_invalid:{key}")
            minimum = _RULE_PARAM_INTEGER_MINIMUMS.get(key, 0)
            maximum = _RULE_PARAM_MAXIMUMS.get(key)
            if value < minimum or (maximum is not None and value > maximum):
                raise ValueError(f"rule_parameter_invalid:{key}")
            normalized[key] = value
            continue
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValueError(f"rule_parameter_invalid:{key}")
        parsed_float = _optional_finite_float(value)
        if parsed_float is None:
            raise ValueError(f"rule_parameter_invalid:{key}")
        minimum = _RULE_PARAM_FLOAT_MINIMUMS.get(key, 0.0)
        maximum = _RULE_PARAM_MAXIMUMS.get(key)
        if parsed_float < minimum or (maximum is not None and parsed_float > maximum):
            raise ValueError(f"rule_parameter_invalid:{key}")
        normalized[key] = parsed_float
    return normalized


def _rule_param_scope_overrides(
    rows: Sequence[Mapping[str, Any]],
    *,
    case_id: str,
) -> dict[str, dict[str, Any]]:
    by_scope: dict[str, dict[str, Any]] = {"system": {}, "case": {}}
    seen: set[tuple[str, str, str]] = set()
    for row in rows:
        scope_type = _trim(row.get("scope_type")).lower()
        scope_key = _trim(row.get("scope_key"))
        key = _trim(row.get("param_key"))
        if scope_type not in by_scope:
            raise ValueError("rule_parameter_scope_invalid")
        expected_scope_key = "global" if scope_type == "system" else _trim(case_id)
        if not key or scope_key != expected_scope_key:
            raise ValueError("rule_parameter_scope_invalid")
        identity = (scope_type, scope_key, key)
        if identity in seen:
            raise ValueError(f"rule_parameter_duplicate:{scope_type}:{key}")
        seen.add(identity)
        raw_json = row.get("param_value_json")
        if raw_json is None or not str(raw_json).strip():
            raise ValueError(f"rule_parameter_invalid:{key}")
        try:
            value = json.loads(str(raw_json))
        except (TypeError, ValueError) as exc:
            raise ValueError(f"rule_parameter_invalid:{key}") from exc
        by_scope[scope_type].update(_validate_rule_param_overrides({key: value}))
    return by_scope


def _complete_amount_sum(values: Iterable[Any], *, digits: int = 2) -> float | None:
    parsed = [_optional_finite_float(value) for value in values]
    if not parsed or any(value is None for value in parsed):
        return None
    return round(sum(float(value) for value in parsed if value is not None), digits)


def _complete_aggregate_amount(
    value: Any,
    *,
    expected_count: Any,
    present_count: Any,
    digits: int = 2,
) -> float | None:
    expected = _optional_nonnegative_int(expected_count)
    present = _optional_nonnegative_int(present_count)
    amount = _optional_finite_float(value)
    if expected is None or present is None or expected <= 0 or present != expected or amount is None:
        return None
    return round(amount, digits)


def _complete_aggregate_count(
    value: Any,
    *,
    expected_count: Any,
    present_count: Any,
) -> int | None:
    expected = _optional_nonnegative_int(expected_count)
    present = _optional_nonnegative_int(present_count)
    count = _optional_nonnegative_int(value)
    if expected is None or present is None or expected <= 0 or present != expected or count is None:
        return None
    return count


def _complete_count_sum(values: Iterable[Any]) -> int | None:
    parsed = [_optional_nonnegative_int(value) for value in values]
    if not parsed or any(value is None for value in parsed):
        return None
    return sum(int(value) for value in parsed if value is not None)


def _complete_rank_amount_facts(row: Mapping[str, Any]) -> dict[str, Any]:
    """Normalize one aggregate rank row without upgrading missing facts to zero."""

    txn_count = _optional_nonnegative_int(row.get("txn_count"))
    in_txn_count = _optional_nonnegative_int(row.get("in_txn_count"))
    out_txn_count = _optional_nonnegative_int(row.get("out_txn_count"))
    in_present_count = _optional_nonnegative_int(row.get("in_amount_present_count"))
    out_present_count = _optional_nonnegative_int(row.get("out_amount_present_count"))
    amount_present_count = _optional_nonnegative_int(row.get("amount_present_count"))
    inflow_total = _complete_aggregate_amount(
        row.get("inflow_total"),
        expected_count=in_txn_count,
        present_count=in_present_count,
    )
    outflow_total = _complete_aggregate_amount(
        row.get("outflow_total"),
        expected_count=out_txn_count,
        present_count=out_present_count,
    )
    if inflow_total is not None and inflow_total < 0:
        inflow_total = None
    if outflow_total is not None and outflow_total < 0:
        outflow_total = None
    amount_coverage_complete = (
        txn_count is not None
        and txn_count > 0
        and in_txn_count is not None
        and out_txn_count is not None
        and in_txn_count + out_txn_count == txn_count
        and in_present_count is not None
        and out_present_count is not None
        and amount_present_count is not None
        and in_present_count + out_present_count == amount_present_count == txn_count
    )
    max_single_amount = _optional_rounded_float(row.get("max_single_amount"), 2)
    if not amount_coverage_complete or max_single_amount is None or max_single_amount < 0:
        max_single_amount = None
    if txn_count is None:
        coverage_status = "unresolved"
    elif txn_count == 0:
        coverage_status = "empty_unverified"
    elif amount_coverage_complete:
        coverage_status = "complete"
    else:
        coverage_status = "partial"
    return {
        "txn_count": txn_count,
        "in_txn_count": in_txn_count,
        "out_txn_count": out_txn_count,
        "in_amount_present_count": in_present_count,
        "out_amount_present_count": out_present_count,
        "amount_present_count": amount_present_count,
        "inflow_total": inflow_total,
        "outflow_total": outflow_total,
        "turnover_total": _complete_amount_sum((inflow_total, outflow_total)),
        "net_flow": (
            round(inflow_total - outflow_total, 2)
            if inflow_total is not None and outflow_total is not None
            else None
        ),
        "max_single_amount": max_single_amount,
        "amount_coverage_status": coverage_status,
    }


def _rule_input_coverage(txn_rows: Sequence[Mapping[str, Any]]) -> dict[str, Any]:
    """Require the factual fields used by deterministic rule evaluation."""

    if not txn_rows:
        return {
            "status": "empty_unverified",
            "txn_count": 0,
            "complete_txn_count": 0,
            "unresolved_txn_count": 0,
            "duplicate_source_record_count": 0,
            "duplicate_txn_id_count": 0,
        }

    source_record_counts: dict[str, int] = defaultdict(int)
    txn_id_counts: dict[str, int] = defaultdict(int)
    for row in txn_rows:
        source_record_id = _trim(row.get("source_record_id"))
        txn_id = _trim(row.get("txn_id"))
        if source_record_id:
            source_record_counts[source_record_id] += 1
        if txn_id:
            txn_id_counts[txn_id] += 1
    duplicate_source_records = {
        source_record_id
        for source_record_id, count in source_record_counts.items()
        if count > 1
    }
    duplicate_txn_ids = {
        txn_id
        for txn_id, count in txn_id_counts.items()
        if count > 1
    }
    complete_count = 0
    for row in txn_rows:
        source_record_id = _trim(row.get("source_record_id"))
        txn_id = _trim(row.get("txn_id"))
        if (
            source_record_id
            and source_record_id not in duplicate_source_records
            and txn_id
            and txn_id not in duplicate_txn_ids
            and _trim(row.get("account_key"))
            and _trim(row.get("direction_norm")) in {"in", "out"}
            and _optional_finite_float(row.get("amount_val")) is not None
            and _coerce_datetime(row.get("txn_ts")) is not None
        ):
            complete_count += 1
    txn_count = len(txn_rows)
    unresolved_count = txn_count - complete_count
    return {
        "status": "complete" if unresolved_count == 0 else "partial",
        "txn_count": txn_count,
        "complete_txn_count": complete_count,
        "unresolved_txn_count": unresolved_count,
        "duplicate_source_record_count": len(duplicate_source_records),
        "duplicate_txn_id_count": len(duplicate_txn_ids),
    }


def _validated_graph_materialization_stats(
    value: Any,
    *,
    expected_txn_count: int,
) -> dict[str, int]:
    expected_count = _exact_nonnegative_db_int(expected_txn_count)
    required_keys = {
        "node_count",
        "edge_count",
        "txn_count",
        "account_node_count",
        "unresolved_amount_edge_count",
    }
    if type(value) is not dict or set(value) != required_keys or expected_count is None:
        raise AnalysisGraphContractError()
    normalized = {
        key: _exact_nonnegative_db_int(value.get(key))
        for key in required_keys
    }
    if (
        any(item is None for item in normalized.values())
        or normalized["txn_count"] != expected_count
        or expected_count <= 0
        or normalized["account_node_count"] <= 0
        or normalized["account_node_count"] > normalized["node_count"]
        or normalized["edge_count"] <= 0
        or normalized["unresolved_amount_edge_count"] != 0
    ):
        raise AnalysisGraphContractError()
    return {key: int(normalized[key]) for key in required_keys}


def _rank_scope_amount_facts(scope_stats: Mapping[str, Any]) -> dict[str, Any]:
    inflow_total = _optional_rounded_float(scope_stats.get("in_amount"), 2)
    outflow_total = _optional_rounded_float(scope_stats.get("out_amount"), 2)
    return {
        "inflow_total": inflow_total,
        "outflow_total": outflow_total,
        "turnover_total": _complete_amount_sum((inflow_total, outflow_total)),
        "net_flow": (
            round(inflow_total - outflow_total, 2)
            if inflow_total is not None and outflow_total is not None
            else None
        ),
    }


def _rank_result_coverage_status(
    scope_stats: Mapping[str, Any],
    *,
    candidate_count: int,
    eligible_count: int,
) -> str:
    scope_status = _trim(scope_stats.get("coverage_status")) or "unresolved"
    if scope_status != "complete":
        return scope_status
    txn_count = _optional_nonnegative_int(scope_stats.get("txn_count"))
    if txn_count is None:
        return "unresolved"
    if txn_count == 0:
        return "empty_unverified"
    if candidate_count <= 0 or eligible_count != candidate_count:
        return "partial"
    return "complete"


def _trace_amount_value(row: Mapping[str, Any]) -> float | None:
    source = row.get("trace_amount_val") if "trace_amount_val" in row else row.get("amount_val")
    number = _optional_finite_float(source)
    return round(abs(number), 2) if number is not None else None


def _quote_identifier(value: str) -> str:
    return '"' + str(value or "").replace('"', '""') + '"'


def _first_finite_number_sql(
    columns: set[str],
    candidates: Sequence[str],
    *,
    alias: str = "",
) -> str:
    branches: list[str] = []
    for name in candidates:
        if name not in columns:
            continue
        quoted = _quote_identifier(name)
        source = f"{alias}.{quoted}" if alias else quoted
        if name.endswith("_val"):
            present = f"{source} IS NOT NULL"
            parsed = f"TRY_CAST({source} AS DOUBLE)"
        else:
            normalized = f"NULLIF(TRIM(CAST({source} AS VARCHAR)), '')"
            present = f"{normalized} IS NOT NULL"
            parsed = f"TRY_CAST({normalized} AS DOUBLE)"
        branches.append(
            f"WHEN {present} THEN CASE WHEN ({parsed}) IS NOT NULL AND isfinite({parsed}) "
            f"THEN {parsed} ELSE NULL END"
        )
    if not branches:
        return "NULL"
    return f"CASE {' '.join(branches)} ELSE NULL END"


_WORKBENCH_ALLOWED_VIEW_POLICY = "cleaned_and_analysis_only"
_WORKBENCH_ALLOWED_ANALYSIS_TABLES = {
    "analysis_account_dim",
    "analysis_case_finding",
    "analysis_case_hypothesis",
    "analysis_case_scope",
    "analysis_entity_node",
    "analysis_evidence_ref",
    "analysis_relation_edge",
    "analysis_rule_hit",
    "analysis_rule_txn_idx",
    "analysis_trace_path",
    "analysis_trace_path_hop",
    "analysis_trace_run",
    "analysis_txn_daily_agg",
    "analysis_txn_detail_idx",
    "analysis_txn_feature",
}
_WORKBENCH_ALLOWED_ANALYSIS_PREFIXES = (
    "analysis_account_",
    "analysis_rule_",
    "analysis_trace_",
    "analysis_txn_",
)
_WORKBENCH_FORBIDDEN_SQL_RE = re.compile(
    r"\b(drop|delete|insert|update|alter|create|attach|detach|copy|export|pragma|install|load|vacuum|truncate|merge|replace)\b",
    re.IGNORECASE,
)
_WORKBENCH_EXTERNAL_SQL_RE = re.compile(
    r"\b(read_csv|read_parquet|read_json|read_text|httpfs|secret|token|s3_|http_|https?:|file:)\b",
    re.IGNORECASE,
)
_WORKBENCH_INCOMPLETE_SQL_RE = re.compile(r"(?:\.\.\.|…)")
_WORKBENCH_FROM_JOIN_RE = re.compile(
    r"\b(?:from|join)\s+((?:\"[^\"]+\"|[A-Za-z_][A-Za-z0-9_]*)(?:\s*\.\s*(?:\"[^\"]+\"|[A-Za-z_][A-Za-z0-9_]*)){0,2})",
    re.IGNORECASE,
)
_WORKBENCH_CTE_RE = re.compile(
    r"(?:\bwith\b|,)\s*(\"[^\"]+\"|[A-Za-z_][A-Za-z0-9_]*)\s+as\s*\(",
    re.IGNORECASE,
)
_WORKBENCH_FORBIDDEN_AST_NODE_NAMES = {
    "addconstraint",
    "alter",
    "attach",
    "cache",
    "command",
    "commit",
    "copy",
    "create",
    "delete",
    "detach",
    "drop",
    "grant",
    "insert",
    "load",
    "merge",
    "pragma",
    "replace",
    "rollback",
    "transaction",
    "truncate",
    "uncache",
    "update",
    "vacuum",
}
_WORKBENCH_FORBIDDEN_FUNCTIONS = {
    "current_setting",
    "export_database",
    "getenv",
    "glob",
    "httpfs",
    "load_extension",
    "read_blob",
    "read_csv",
    "read_csv_auto",
    "read_json",
    "read_json_auto",
    "read_ndjson",
    "read_parquet",
    "read_text",
    "sqlite_scan",
}
_CASE_SQL_RECIPE_DEFINITIONS: tuple[dict[str, Any], ...] = (
    {
        "recipe_id": "pair_amount_one_hop",
        "title": "两主体一跳金额核算",
        "category": "pair_amount",
        "required_parameters": ["payer_name", "receiver_name", "date_start?", "date_end?"],
        "template_sql": """
WITH scoped AS (
  SELECT txn_ts, account_key, account_name, cp_key, cp_name, amount, dc_val, txn_id
  FROM analysis_txn_detail_idx
  WHERE account_name = '{{payer_name}}'
    AND cp_name = '{{receiver_name}}'
    AND dc_val = '出'
    {{date_predicate}}
)
SELECT COUNT(*) AS txn_count, SUM(amount) AS total_amount,
       MIN(txn_ts) AS first_txn_at, MAX(txn_ts) AS last_txn_at,
       COUNT(DISTINCT account_key) AS payer_account_count,
       COUNT(DISTINCT cp_key) AS receiver_account_count
FROM scoped
""".strip(),
        "validation_notes": ["只作一跳聚合；同事实/换卡风险仍需 resolve_duplicate_families 或两方金额 focused owner 复核。"],
    },
    {
        "recipe_id": "subject_account_overview",
        "title": "主体登记账户与进出账概览",
        "category": "subject_dossier",
        "required_parameters": ["holder_name", "date_start?", "date_end?"],
        "template_sql": """
SELECT account_key, account_name,
       COUNT(*) AS txn_count,
       SUM(CASE WHEN dc_val='进' THEN amount ELSE 0 END) AS inflow_amount,
       SUM(CASE WHEN dc_val='出' THEN amount ELSE 0 END) AS outflow_amount,
       MIN(txn_ts) AS first_txn_at,
       MAX(txn_ts) AS last_txn_at
FROM analysis_txn_detail_idx
WHERE account_name = '{{holder_name}}'
  {{date_predicate}}
GROUP BY account_key, account_name
ORDER BY inflow_amount + outflow_amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["账户归属只能按当前清洗字段说明；候选归属须另走主体账户范围核验。"],
    },
    {
        "recipe_id": "top_counterparties",
        "title": "Top 对手方核算",
        "category": "counterparty_rank",
        "required_parameters": ["holder_name|account_key", "direction", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT cp_name, cp_key,
       COUNT(*) AS txn_count,
       SUM(amount) AS total_amount,
       MIN(txn_ts) AS first_txn_at,
       MAX(txn_ts) AS last_txn_at
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND dc_val = '{{direction}}'
  {{date_predicate}}
GROUP BY cp_name, cp_key
ORDER BY total_amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["排行只能定位重点对手；不能替代两方金额事实或去重口径。"],
    },
    {
        "recipe_id": "downstream_destinations",
        "title": "下游去向首层核算",
        "category": "downstream",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT cp_name, cp_key,
       COUNT(*) AS out_txn_count,
       SUM(amount) AS out_amount,
       MIN(txn_ts) AS first_out_at,
       MAX(txn_ts) AS last_out_at
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND dc_val = '出'
  {{date_predicate}}
GROUP BY cp_name, cp_key
ORDER BY out_amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["只说明当前案件可见首层去向；二层穿透仍需 trace_subject_top_outflows/trace_fund。"],
    },
    {
        "recipe_id": "same_fact_card_change_review",
        "title": "同事实/换卡复核候选",
        "category": "duplicate_review",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT txn_ts, dc_val, amount, balance, cp_key, cp_name,
       COUNT(DISTINCT account_key) AS account_count,
       COUNT(*) AS row_count
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  {{date_predicate}}
GROUP BY txn_ts, dc_val, amount, balance, cp_key, cp_name
HAVING COUNT(DISTINCT account_key) > 1
ORDER BY row_count DESC, amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["候选只能提示复核；是否扣减报告金额须由同事实规则和底层交易证据确定。"],
    },
    {
        "recipe_id": "cash_withdrawal_breakpoints",
        "title": "现金取现断点核算",
        "category": "cash_breakpoint",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT account_key, account_name,
       COUNT(*) AS cash_out_count,
       SUM(amount) AS cash_out_amount,
       MIN(txn_ts) AS first_cash_out_at,
       MAX(txn_ts) AS last_cash_out_at
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND dc_val = '出'
  AND (cash_flag = '现金' OR summary LIKE '%取现%' OR txn_type LIKE '%取款%')
  {{date_predicate}}
GROUP BY account_key, account_name
ORDER BY cash_out_amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["现金断点不能直接认定最终去向；需补取现影像、柜台凭证或后续消费材料。"],
    },
    {
        "recipe_id": "financial_product_flows",
        "title": "理财/基金/证券资金核算",
        "category": "financial_products",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT cp_name, cp_key,
       COUNT(*) AS txn_count,
       SUM(CASE WHEN dc_val='出' THEN amount ELSE 0 END) AS purchase_or_transfer_out,
       SUM(CASE WHEN dc_val='进' THEN amount ELSE 0 END) AS redemption_or_return_in
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND (cp_name LIKE '%基金%' OR cp_name LIKE '%证券%' OR cp_name LIKE '%理财%' OR summary LIKE '%理财%' OR summary LIKE '%基金%' OR summary LIKE '%证券%')
  {{date_predicate}}
GROUP BY cp_name, cp_key
ORDER BY purchase_or_transfer_out + redemption_or_return_in DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["资金用途只能写为产品线索；收益、赎回和最终受益需补产品凭证。"],
    },
    {
        "recipe_id": "asset_purchase_flows",
        "title": "买房买车/资产消费线索核算",
        "category": "asset_purchase",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT cp_name, cp_key,
       COUNT(*) AS txn_count,
       SUM(amount) AS total_amount,
       MIN(txn_ts) AS first_txn_at,
       MAX(txn_ts) AS last_txn_at
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND dc_val = '出'
  AND (cp_name LIKE '%房%' OR cp_name LIKE '%车%' OR summary LIKE '%购房%' OR summary LIKE '%购车%' OR summary LIKE '%首付%')
  {{date_predicate}}
GROUP BY cp_name, cp_key
ORDER BY total_amount DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["仅为资产消费线索，不能替代不动产/车辆登记、合同发票和付款凭证。"],
    },
    {
        "recipe_id": "device_contact_address_links",
        "title": "IP/MAC/联系方式/住址关联线索",
        "category": "association",
        "required_parameters": ["holder_name|account_key", "date_start?", "date_end?", "limit"],
        "template_sql": """
SELECT account_key, account_name, ip_addr, mac_addr,
       COUNT(*) AS txn_count,
       MIN(txn_ts) AS first_seen_at,
       MAX(txn_ts) AS last_seen_at
FROM analysis_txn_detail_idx
WHERE {{subject_predicate}}
  AND (NULLIF(TRIM(COALESCE(ip_addr,'')), '') IS NOT NULL OR NULLIF(TRIM(COALESCE(mac_addr,'')), '') IS NOT NULL)
  {{date_predicate}}
GROUP BY account_key, account_name, ip_addr, mac_addr
ORDER BY txn_count DESC
LIMIT {{limit}}
""".strip(),
        "validation_notes": ["设备/IP 是关联线索，不直接认定同一控制人；联系方式、住址、股权需结合对应来源表和材料。"],
    },
)


def _strip_sql_comments(sql: str) -> str:
    text = str(sql or "")
    output: list[str] = []
    index = 0
    in_single = False
    in_double = False
    while index < len(text):
        char = text[index]
        nxt = text[index + 1] if index + 1 < len(text) else ""
        if in_single:
            output.append(char)
            if char == "'" and nxt == "'":
                output.append(nxt)
                index += 2
                continue
            if char == "'":
                in_single = False
            index += 1
            continue
        if in_double:
            output.append(char)
            if char == '"' and nxt == '"':
                output.append(nxt)
                index += 2
                continue
            if char == '"':
                in_double = False
            index += 1
            continue
        if char == "-" and nxt == "-":
            while index < len(text) and text[index] not in "\r\n":
                output.append(" ")
                index += 1
            continue
        if char == "/" and nxt == "*":
            output.extend("  ")
            index += 2
            while index < len(text):
                if text[index] == "*" and index + 1 < len(text) and text[index + 1] == "/":
                    output.extend("  ")
                    index += 2
                    break
                output.append(" ")
                index += 1
            continue
        if char == "'":
            in_single = True
        elif char == '"':
            in_double = True
        output.append(char)
        index += 1
    return "".join(output)


def _mask_sql_literals(sql: str) -> str:
    output: list[str] = []
    index = 0
    in_single = False
    in_double = False
    while index < len(sql):
        char = sql[index]
        nxt = sql[index + 1] if index + 1 < len(sql) else ""
        if in_single:
            output.append(" ")
            if char == "'" and nxt == "'":
                output.append(" ")
                index += 2
                continue
            if char == "'":
                in_single = False
            index += 1
            continue
        if in_double:
            output.append(char)
            if char == '"' and nxt == '"':
                output.append(nxt)
                index += 2
                continue
            if char == '"':
                in_double = False
            index += 1
            continue
        if char == "'":
            in_single = True
            output.append(" ")
            index += 1
            continue
        if char == '"':
            in_double = True
        output.append(char)
        index += 1
    return "".join(output)


def _split_sql_statements(sql: str) -> list[str]:
    statements: list[str] = []
    current: list[str] = []
    in_single = False
    in_double = False
    index = 0
    while index < len(sql):
        char = sql[index]
        nxt = sql[index + 1] if index + 1 < len(sql) else ""
        if in_single:
            current.append(char)
            if char == "'" and nxt == "'":
                current.append(nxt)
                index += 2
                continue
            if char == "'":
                in_single = False
            index += 1
            continue
        if in_double:
            current.append(char)
            if char == '"' and nxt == '"':
                current.append(nxt)
                index += 2
                continue
            if char == '"':
                in_double = False
            index += 1
            continue
        if char == "'":
            in_single = True
            current.append(char)
            index += 1
            continue
        if char == '"':
            in_double = True
            current.append(char)
            index += 1
            continue
        if char == ";":
            statement = "".join(current).strip()
            if statement:
                statements.append(statement)
            current = []
            index += 1
            continue
        current.append(char)
        index += 1
    statement = "".join(current).strip()
    if statement:
        statements.append(statement)
    return statements


def _unquote_identifier(value: str) -> str:
    text = _trim(value)
    if text.startswith('"') and text.endswith('"') and len(text) >= 2:
        text = text[1:-1].replace('""', '"')
    return text


def _workbench_cte_names(masked_sql: str) -> set[str]:
    return {_unquote_identifier(match.group(1)).lower() for match in _WORKBENCH_CTE_RE.finditer(masked_sql)}


def _workbench_table_name(value: str) -> str:
    parts = [
        _unquote_identifier(part.strip()).lower()
        for part in re.split(r"\s*\.\s*", str(value or ""))
        if part.strip()
    ]
    return parts[-1] if parts else ""


def _is_workbench_expression_from(masked_sql: str, match_start: int) -> bool:
    # SQL functions such as EXTRACT(hour FROM ts) contain FROM tokens that are
    # expressions, not table references.
    prefix = str(masked_sql or "")[:match_start]
    open_pos = prefix.rfind("(")
    close_pos = prefix.rfind(")")
    if open_pos <= close_pos:
        return False
    function_prefix = prefix[:open_pos].rstrip()
    match = re.search(r"([A-Za-z_][A-Za-z0-9_]*)\s*$", function_prefix)
    return bool(match and match.group(1).lower() in {"extract"})


def _workbench_referenced_tables(masked_sql: str) -> list[str]:
    tables: list[str] = []
    for match in _WORKBENCH_FROM_JOIN_RE.finditer(masked_sql):
        if _is_workbench_expression_from(masked_sql, match.start()):
            continue
        table_name = _workbench_table_name(match.group(1))
        if table_name and table_name not in tables:
            tables.append(table_name)
    return tables


def _is_allowed_workbench_table(table_name: str) -> bool:
    normalized = _trim(table_name).lower()
    if normalized.startswith("fc_") and normalized.endswith("_norm"):
        return True
    if normalized in _WORKBENCH_ALLOWED_ANALYSIS_TABLES:
        return True
    return any(normalized.startswith(prefix) for prefix in _WORKBENCH_ALLOWED_ANALYSIS_PREFIXES)


def _sqlglot_expression_class(name: str) -> Any:
    return getattr(_sqlglot_exp, name, None) if _sqlglot_exp is not None else None


def _sqlglot_walk_nodes(expression: Any) -> list[Any]:
    nodes: list[Any] = []
    for item in expression.walk():
        node = item[0] if isinstance(item, tuple) and item else item
        if node is not None:
            nodes.append(node)
    return nodes


def _sqlglot_find_all(expression: Any, class_name: str) -> list[Any]:
    node_class = _sqlglot_expression_class(class_name)
    if node_class is None:
        return []
    return list(expression.find_all(node_class))


def _normalize_sqlglot_name(value: Any) -> str:
    return re.sub(r"[^a-z0-9_]", "", _trim(value).lower())


def _sqlglot_node_name(node: Any) -> str:
    raw_name = ""
    try:
        raw_name = _trim(getattr(node, "name", ""))
    except Exception:
        raw_name = ""
    if raw_name:
        return raw_name
    try:
        return _trim(node.sql_name())
    except Exception:
        return ""


def _sqlglot_table_parts(table: Any) -> tuple[str, str, str]:
    table_name = _normalize_sqlglot_name(getattr(table, "name", ""))
    db_name = _normalize_sqlglot_name(getattr(table, "db", ""))
    catalog_name = _normalize_sqlglot_name(getattr(table, "catalog", ""))
    return catalog_name, db_name, table_name


def _sqlglot_has_limit(expression: Any) -> bool:
    return bool(_sqlglot_find_all(expression, "Limit"))


def _sqlglot_has_star_projection(expression: Any) -> bool:
    return bool(_sqlglot_find_all(expression, "Star"))


def _parse_workbench_sql_ast(statement: str) -> Any:
    if _sqlglot_parse is None or _sqlglot_exp is None:
        raise ValueError("Structured SQL parser is unavailable; controlled case SQL fails closed")
    try:
        expressions = _sqlglot_parse(statement, read="duckdb")
    except _SqlglotParseError as exc:
        raise ValueError(f"Controlled Case Workbench SQL parse failed: {exc}") from exc
    if len(expressions) != 1:
        raise ValueError("Controlled Case Workbench accepts exactly one SELECT/WITH statement")
    expression = expressions[0]
    select_class = _sqlglot_expression_class("Select")
    if select_class is None or not isinstance(expression, select_class):
        raise ValueError("Controlled Case Workbench SQL must be a SELECT/WITH query")
    return expression


def _validate_workbench_sql_ast(
    statement: str,
    *,
    allow_select_star_with_limit: bool = False,
) -> tuple[list[str], list[str]]:
    expression = _parse_workbench_sql_ast(statement)
    nodes = _sqlglot_walk_nodes(expression)
    normalized_forbidden_functions = {
        re.sub(r"[^a-z0-9]", "", item.lower()) for item in _WORKBENCH_FORBIDDEN_FUNCTIONS
    }
    for node in nodes:
        class_name = node.__class__.__name__
        normalized_class = re.sub(r"[^a-z0-9]", "", class_name.lower())
        if normalized_class in _WORKBENCH_FORBIDDEN_AST_NODE_NAMES:
            raise ValueError(f"Controlled Case Workbench SQL contains a forbidden statement node: {class_name}")
        node_name = _sqlglot_node_name(node)
        normalized_node_name = re.sub(r"[^a-z0-9]", "", node_name.lower())
        if normalized_node_name in normalized_forbidden_functions or normalized_class in normalized_forbidden_functions:
            raise ValueError(f"external file/network/secret function is not allowed: {node_name or class_name}")
    if _sqlglot_has_star_projection(expression):
        if not allow_select_star_with_limit or not _sqlglot_has_limit(expression):
            raise ValueError("unbounded SELECT * is not allowed; request explicit columns, aggregates, or a bounded preview")

    cte_names: set[str] = set()
    for cte in _sqlglot_find_all(expression, "CTE"):
        alias = _normalize_sqlglot_name(getattr(cte, "alias_or_name", ""))
        if alias:
            cte_names.add(alias)

    base_tables: list[str] = []
    for table in _sqlglot_find_all(expression, "Table"):
        catalog_name, db_name, table_name = _sqlglot_table_parts(table)
        if not table_name or table_name in cte_names:
            continue
        if catalog_name or (db_name and db_name != "main"):
            raise ValueError("cross-database or attached-catalog table references are not allowed")
        if table_name not in base_tables:
            base_tables.append(table_name)
    return base_tables, sorted(cte_names)


def _workbench_json_safe(value: Any) -> Any:
    if isinstance(value, (datetime, date)):
        return value.isoformat()
    if isinstance(value, Decimal):
        return float(value)
    if isinstance(value, bytes):
        return base64.b64encode(value).decode("ascii")
    if isinstance(value, list):
        return [_workbench_json_safe(item) for item in value]
    if isinstance(value, tuple):
        return [_workbench_json_safe(item) for item in value]
    if isinstance(value, dict):
        return {str(key): _workbench_json_safe(item) for key, item in value.items()}
    return value


def _classify_case_sql_error(message: str) -> str:
    normalized = _trim(message).lower()
    if not normalized:
        return "unknown"
    if "parser" in normalized or "parse" in normalized or "syntax" in normalized:
        return "sql_parse_error"
    if "structured sql parser is unavailable" in normalized:
        return "sql_parser_unavailable"
    if "forbidden" in normalized or "not allowed" in normalized or "external file" in normalized:
        return "guardrail_blocked"
    if "raw/source" in normalized or "raw table" in normalized:
        return "raw_table_blocked"
    if "table" in normalized and ("unavailable" in normalized or "does not exist" in normalized or "not found" in normalized):
        return "table_unavailable"
    if "column" in normalized or "binder error" in normalized or "referenced column" in normalized:
        return "column_unavailable"
    if "timeout" in normalized or "too slow" in normalized or "memory" in normalized:
        return "performance_risk"
    if "empty" in normalized or "no rows" in normalized:
        return "empty_result"
    return "execution_error"


def _classify_case_sql_exception(exc: BaseException) -> str:
    try:
        return _classify_case_sql_error(str(exc))
    except Exception:
        return "execution_error"


def _case_sql_plan_summary(plan_text: str) -> dict[str, Any]:
    text_value = _trim(plan_text)
    if not text_value or re.search(
        r"\b(?:physical_plan|logical_plan|projection|filter|aggregate|hash_join|nested_loop_join|order_by|limit|dummy_scan|seq_scan|sequential scan|table_scan|table scan)\b",
        text_value,
        flags=re.IGNORECASE,
    ) is None:
        raise ValueError("case_sql_explain_result_invalid")
    estimated_rows: list[int] = []
    for match in re.finditer(r"~?\s*([0-9][0-9,]*)\s+rows?\b", text_value, flags=re.IGNORECASE):
        try:
            estimated_rows.append(int(match.group(1).replace(",", "")))
        except Exception:
            continue
    full_scan = bool(re.search(r"\b(?:seq_scan|sequential scan|table_scan|table scan)\b", text_value, flags=re.IGNORECASE))
    return {
        "plan_kind": "duckdb_explain",
        "estimated_rows": estimated_rows[:8],
        "estimated_row_upper_bound": max(estimated_rows) if estimated_rows else None,
        "full_table_scan_possible": full_scan,
        "plan_excerpt": text_value[:1200],
    }


def _parse_timestamp_text(value: Any) -> Optional[datetime]:
    text = _trim(value)
    if not text:
        return None
    try:
        stamp = pd.to_datetime(text, errors="coerce")
    except Exception:
        return None
    if stamp is None or pd.isna(stamp):
        return None
    if getattr(stamp, "tzinfo", None) is not None:
        try:
            stamp = stamp.tz_convert(None)
        except Exception:
            try:
                stamp = stamp.tz_localize(None)
            except Exception:
                pass
    try:
        return stamp.to_pydatetime()
    except Exception:
        return None


def _format_timestamp_display(value: Any) -> str:
    parsed = _parse_timestamp_text(value)
    if parsed is None:
        return _trim(value)
    return parsed.strftime("%Y-%m-%d %H:%M:%S")


def _format_duration_display(total_seconds: float) -> str:
    seconds = max(0, int(round(total_seconds)))
    if seconds < 60:
        return f"{seconds} 秒"
    minutes, sec = divmod(seconds, 60)
    if minutes < 60:
        return f"{minutes} 分钟" if sec < 30 else f"{minutes + 1} 分钟"
    hours, minute = divmod(minutes, 60)
    if hours < 24:
        return f"{hours} 小时" if minute == 0 else f"{hours} 小时 {minute} 分钟"
    days, hour = divmod(hours, 24)
    return f"{days} 天" if hour == 0 else f"{days} 天 {hour} 小时"


def _is_passive_mixed_fund_replenishment(row: Mapping[str, Any] | None) -> bool:
    payload = dict(row or {})
    if _trim(payload.get("direction_norm")) != "in":
        return False
    combined = " ".join(
        part
        for part in (
            _trim(payload.get("summary")),
            _trim(payload.get("txn_type")),
            _trim(payload.get("remark")),
            _trim(payload.get("counterparty_name")),
            _trim(payload.get("counterparty_key")),
        )
        if part
    )
    if not combined:
        return False
    return any(marker in combined for marker in PASSIVE_MIXED_FUND_REPLENISHMENT_MARKERS)


def _hash_content(content: str) -> str:
    return hashlib.sha1(content.encode("utf-8")).hexdigest()


def _safe_user() -> str:
    try:
        return getuser()
    except Exception:
        return "system"


def _normalize_case_status(status: str) -> str:
    raw = _trim(status)
    if "归档" in raw or "结案" in raw:
        return "archived"
    return "active"


def _entity_prefix(scope_type: str) -> str:
    normalized = _trim(scope_type).lower()
    if normalized == "account":
        return "acct"
    if normalized == "person":
        return "person"
    if normalized == "counterparty":
        return "cp"
    if normalized == "device":
        return "device"
    if normalized == "branch":
        return "branch"
    if normalized == "location":
        return "loc"
    if normalized == "voucher":
        return "voucher"
    if normalized == "teller":
        return "teller"
    if normalized == "time_range":
        return "time"
    return normalized or "entity"


def _build_entity_id(scope_type: str, scope_value: str) -> str:
    prefix = _entity_prefix(scope_type)
    stable = "".join(ch for ch in _trim(scope_value) if ch.isalnum())[:48] or uuid.uuid4().hex[:8]
    return f"{prefix}_{stable}"


class AnalysisRepository:
    LARGE_REFRESH_TXN_ROW_LIMIT = 200_000

    BASE_WORKSPACE_FILES = (
        "CASE.md",
        "WORKSPACE_MEMORY.md",
        "HYPOTHESES.md",
        "TIMELINE.md",
        "EVIDENCE_INDEX.md",
    )

    def __init__(
        self,
        *,
        privacy_projection_repository: PrivacyProjectionRepository | None = None,
    ) -> None:
        self._storage = CaseStorage()
        self._artifact_renderer = WorkspaceArtifactRenderer()
        self._privacy_projection_repository = privacy_projection_repository or PrivacyProjectionRepository()
        self._daily_agg = TxnDailyAggregateStore(self._storage)

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    def case_exists(self, case_id: str) -> bool:
        return self._storage.get_case(case_id) is not None

    def case_dir(self, case_id: str) -> Path:
        return self._storage.case_dir(case_id)

    def workspace_dir(self, case_id: str) -> Path:
        directory = self.case_dir(case_id) / "workspace"
        directory.mkdir(parents=True, exist_ok=True)
        return directory

    def open_case_engine(self, case_id: str, *, read_only: bool = False) -> DuckDBEngine:
        return self._storage.open_case_engine(case_id, read_only=read_only)

    def _with_case_engine_retry(
        self,
        case_id: str,
        runner: Callable[[DuckDBEngine], Any],
        *,
        read_only: bool = False,
        max_attempts: int = 3,
    ) -> Any:
        last_error: Exception | None = None
        for attempt in range(max_attempts):
            engine: DuckDBEngine | None = None
            try:
                engine = self.open_case_engine(case_id, read_only=read_only)
                return runner(engine)
            except Exception as exc:
                last_error = exc
                if not _is_transient_case_db_conflict(exc) or attempt >= max_attempts - 1:
                    raise
                time.sleep(0.05 * (attempt + 1))
            finally:
                if engine is not None:
                    engine.close()
        if last_error is not None:
            raise last_error
        raise RuntimeError("case_engine_retry_failed_without_error")

    def ensure_analysis_schema(self, engine: DuckDBEngine) -> None:
        definitions: dict[str, list[tuple[str, str]]] = {
            "analysis_case_profile": [
                ("case_id", "TEXT"),
                ("case_name", "TEXT"),
                ("case_number", "TEXT"),
                ("case_type", "TEXT"),
                ("owner", "TEXT"),
                ("tags_json", "TEXT"),
                ("note", "TEXT"),
                ("scope_targets_json", "TEXT"),
                ("initial_direction", "TEXT"),
                ("status", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_case_scope": [
                ("scope_id", "TEXT"),
                ("case_id", "TEXT"),
                ("scope_type", "TEXT"),
                ("scope_value", "TEXT"),
                ("priority", "INTEGER"),
                ("status", "TEXT"),
                ("source", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_case_hypothesis": [
                ("hypothesis_id", "TEXT"),
                ("case_id", "TEXT"),
                ("title", "TEXT"),
                ("content_md", "TEXT"),
                ("confidence", "DOUBLE"),
                ("status", "TEXT"),
                ("source_query_ids_json", "TEXT"),
                ("source_evidence_ids_json", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_case_note": [
                ("note_id", "TEXT"),
                ("case_id", "TEXT"),
                ("title", "TEXT"),
                ("content_md", "TEXT"),
                ("author", "TEXT"),
                ("source_query_ids_json", "TEXT"),
                ("source_evidence_ids_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_case_finding": [
                ("finding_id", "TEXT"),
                ("case_id", "TEXT"),
                ("finding_type", "TEXT"),
                ("title", "TEXT"),
                ("summary_md", "TEXT"),
                ("severity", "TEXT"),
                ("status", "TEXT"),
                ("source_query_ids_json", "TEXT"),
                ("source_evidence_ids_json", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_case_report": [
                ("report_id", "TEXT"),
                ("case_id", "TEXT"),
                ("scope", "TEXT"),
                ("title", "TEXT"),
                ("content_md", "TEXT"),
                ("version", "INTEGER"),
                ("status", "TEXT"),
                ("generated_from_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_entity_node": [
                ("entity_id", "TEXT"),
                ("case_id", "TEXT"),
                ("entity_type", "TEXT"),
                ("entity_key", "TEXT"),
                ("display_name", "TEXT"),
                ("id_no", "TEXT"),
                ("bank_name", "TEXT"),
                ("attrs_json", "TEXT"),
                ("first_seen_at", "TEXT"),
                ("last_seen_at", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_relation_edge": [
                ("edge_id", "TEXT"),
                ("case_id", "TEXT"),
                ("src_entity_id", "TEXT"),
                ("dst_entity_id", "TEXT"),
                ("relation_type", "TEXT"),
                ("weight", "DOUBLE"),
                ("txn_count", "BIGINT"),
                ("amount_sum", "DOUBLE"),
                ("first_seen_at", "TEXT"),
                ("last_seen_at", "TEXT"),
                ("attrs_json", "TEXT"),
            ],
            "analysis_txn_feature": [
                ("feature_id", "TEXT"),
                ("case_id", "TEXT"),
                ("txn_id", "TEXT"),
                ("account_key", "TEXT"),
                ("feature_type", "TEXT"),
                ("feature_value_num", "DOUBLE"),
                ("feature_value_text", "TEXT"),
                ("score", "DOUBLE"),
                ("attrs_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_account_feature": [
                ("feature_id", "TEXT"),
                ("case_id", "TEXT"),
                ("account_key", "TEXT"),
                ("feature_type", "TEXT"),
                ("score", "DOUBLE"),
                ("stats_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_rule_hit": [
                ("rule_hit_id", "TEXT"),
                ("case_id", "TEXT"),
                ("rule_code", "TEXT"),
                ("risk_type", "TEXT"),
                ("severity", "TEXT"),
                ("score", "DOUBLE"),
                ("entity_ids_json", "TEXT"),
                ("txn_ids_json", "TEXT"),
                ("evidence_ids_json", "TEXT"),
                ("summary", "TEXT"),
                ("detail_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_graph_cluster": [
                ("cluster_id", "TEXT"),
                ("case_id", "TEXT"),
                ("cluster_type", "TEXT"),
                ("score", "DOUBLE"),
                ("node_ids_json", "TEXT"),
                ("edge_ids_json", "TEXT"),
                ("summary", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_rule_config": [
                ("config_key", "TEXT"),
                ("scope_type", "TEXT"),
                ("scope_key", "TEXT"),
                ("param_key", "TEXT"),
                ("param_value_json", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_trace_run": [
                ("trace_id", "TEXT"),
                ("case_id", "TEXT"),
                ("seed_type", "TEXT"),
                ("seed_value", "TEXT"),
                ("depth", "INTEGER"),
                ("time_window_sec", "INTEGER"),
                ("tolerance_rate", "DOUBLE"),
                ("status", "TEXT"),
                ("summary_json", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_trace_path": [
                ("path_id", "TEXT"),
                ("trace_id", "TEXT"),
                ("case_id", "TEXT"),
                ("path_index", "INTEGER"),
                ("hop_count", "INTEGER"),
                ("path_score", "DOUBLE"),
                ("amount_match_rate", "DOUBLE"),
                ("time_span_sec", "INTEGER"),
                ("sink_type", "TEXT"),
                ("summary", "TEXT"),
                ("evidence_ids_json", "TEXT"),
                ("detail_json", "TEXT"),
            ],
            "analysis_trace_path_hop": [
                ("path_hop_id", "TEXT"),
                ("path_id", "TEXT"),
                ("hop_index", "INTEGER"),
                ("txn_id", "TEXT"),
                ("src_entity_id", "TEXT"),
                ("dst_entity_id", "TEXT"),
                ("txn_time", "TEXT"),
                ("amount", "DOUBLE"),
                ("direction", "TEXT"),
                ("attrs_json", "TEXT"),
            ],
            "analysis_evidence_ref": [
                ("evidence_id", "TEXT"),
                ("case_id", "TEXT"),
                ("evidence_type", "TEXT"),
                ("ref_table", "TEXT"),
                ("ref_pk", "TEXT"),
                ("title", "TEXT"),
                ("snippet", "TEXT"),
                ("payload_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_query_log": [
                ("query_id", "TEXT"),
                ("case_id", "TEXT"),
                ("tool_name", "TEXT"),
                ("params_json", "TEXT"),
                ("summary_json", "TEXT"),
                ("row_count", "BIGINT"),
                ("duration_ms", "BIGINT"),
                ("created_at", "TEXT"),
            ],
            "analysis_workspace_file": [
                ("file_key", "TEXT"),
                ("case_id", "TEXT"),
                ("file_name", "TEXT"),
                ("content_md", "TEXT"),
                ("version", "INTEGER"),
                ("source_hash", "TEXT"),
                ("render_mode", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_llm_session": [
                ("session_id", "TEXT"),
                ("case_id", "TEXT"),
                ("title", "TEXT"),
                ("pinned_at", "TEXT"),
                ("last_prompt", "TEXT"),
                ("document_ids_json", "TEXT"),
                ("subject_ids_json", "TEXT"),
                ("weak_subject_ids_json", "TEXT"),
                ("account_keys_json", "TEXT"),
                ("messages_json", "TEXT"),
                ("skill_trace_json", "TEXT"),
                ("domain_trace_json", "TEXT"),
                ("focus_summary", "TEXT"),
                ("conclusion_summary", "TEXT"),
                ("next_steps_summary", "TEXT"),
                ("covered_until_turn_id", "TEXT"),
                ("covered_message_count", "BIGINT"),
                ("tags_json", "TEXT"),
                ("index_text", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_memory_note": [
                ("note_id", "TEXT"),
                ("case_id", "TEXT"),
                ("scope", "TEXT"),
                ("owner_id", "TEXT"),
                ("memory_type", "TEXT"),
                ("title", "TEXT"),
                ("summary", "TEXT"),
                ("detail", "TEXT"),
                ("freshness", "TEXT"),
                ("trust_level", "TEXT"),
                ("required_revalidation", "BOOLEAN"),
                ("tags_json", "TEXT"),
                ("source", "TEXT"),
                ("status", "TEXT"),
                ("index_text", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_audit_event": [
                ("event_id", "TEXT"),
                ("case_id", "TEXT"),
                ("event_type", "TEXT"),
                ("task_id", "TEXT"),
                ("run_id", "TEXT"),
                ("turn_id", "TEXT"),
                ("artifact_id", "TEXT"),
                ("approval_id", "TEXT"),
                ("trace_id", "TEXT"),
                ("span_id", "TEXT"),
                ("actor_id", "TEXT"),
                ("actor_role", "TEXT"),
                ("tenant_id", "TEXT"),
                ("request_id", "TEXT"),
                ("session_id", "TEXT"),
                ("payload_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_run_log_event": [
                ("event_id", "TEXT"),
                ("case_id", "TEXT"),
                ("task_id", "TEXT"),
                ("run_id", "TEXT"),
                ("turn_id", "TEXT"),
                ("sequence_no", "BIGINT"),
                ("event_stage", "TEXT"),
                ("event_type", "TEXT"),
                ("source", "TEXT"),
                ("payload_json", "TEXT"),
                ("created_at", "TEXT"),
            ],
            "analysis_scratchpad_workspace": [
                ("workspace_id", "TEXT"),
                ("case_id", "TEXT"),
                ("run_id", "TEXT"),
                ("turn_id", "TEXT"),
                ("owner_type", "TEXT"),
                ("owner_id", "TEXT"),
                ("workspace_kind", "TEXT"),
                ("status", "TEXT"),
                ("parent_workspace_id", "TEXT"),
                ("inherited_context_json", "TEXT"),
                ("retention_state", "TEXT"),
                ("created_at", "TEXT"),
                ("expires_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_workspace_artifact": [
                ("artifact_id", "TEXT"),
                ("case_id", "TEXT"),
                ("workspace_id", "TEXT"),
                ("run_id", "TEXT"),
                ("turn_id", "TEXT"),
                ("artifact_type", "TEXT"),
                ("title", "TEXT"),
                ("content_summary", "TEXT"),
                ("content_md", "TEXT"),
                ("provenance_json", "TEXT"),
                ("is_formalized", "BOOLEAN"),
                ("created_by", "TEXT"),
                ("retention_state", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_phase_judgment": [
                ("judgment_id", "TEXT"),
                ("case_id", "TEXT"),
                ("session_id", "TEXT"),
                ("run_id", "TEXT"),
                ("turn_id", "TEXT"),
                ("main_line_summary", "TEXT"),
                ("stage_conclusion_summary", "TEXT"),
                ("priority_action_summary", "TEXT"),
                ("status", "TEXT"),
                ("confidence_json", "TEXT"),
                ("tags_json", "TEXT"),
                ("source_query_ids_json", "TEXT"),
                ("source_evidence_ids_json", "TEXT"),
                ("source_path_ids_json", "TEXT"),
                ("source_report_ids_json", "TEXT"),
                ("derived_from_json", "TEXT"),
                ("policy_json", "TEXT"),
                ("visible", "BOOLEAN"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
            "analysis_temp_scope": [
                ("scope_id", "TEXT"),
                ("case_id", "TEXT"),
                ("scope_type", "TEXT"),
                ("source_kind", "TEXT"),
                ("scope_signature", "TEXT"),
                ("status", "TEXT"),
                ("source_revision", "BIGINT"),
                ("source_file_ids_json", "TEXT"),
                ("document_ids_json", "TEXT"),
                ("stats_json", "TEXT"),
                ("audit_json", "TEXT"),
                ("expires_at", "TEXT"),
                ("retention_until", "TEXT"),
                ("last_accessed_at", "TEXT"),
                ("created_at", "TEXT"),
                ("updated_at", "TEXT"),
            ],
        }
        primary_keys = {
            "analysis_case_profile": "case_id",
            "analysis_case_scope": "scope_id",
            "analysis_case_hypothesis": "hypothesis_id",
            "analysis_case_note": "note_id",
            "analysis_case_finding": "finding_id",
            "analysis_case_report": "report_id",
            "analysis_entity_node": "entity_id",
            "analysis_relation_edge": "edge_id",
            "analysis_txn_feature": "feature_id",
            "analysis_account_feature": "feature_id",
            "analysis_rule_hit": "rule_hit_id",
            "analysis_graph_cluster": "cluster_id",
            "analysis_rule_config": "config_key",
            "analysis_trace_run": "trace_id",
            "analysis_trace_path": "path_id",
            "analysis_trace_path_hop": "path_hop_id",
            "analysis_evidence_ref": "evidence_id",
            "analysis_query_log": "query_id",
            "analysis_workspace_file": "file_key",
            "analysis_llm_session": "session_id",
            "analysis_memory_note": "note_id",
            "analysis_audit_event": "event_id",
            "analysis_run_log_event": "event_id",
            "analysis_scratchpad_workspace": "workspace_id",
            "analysis_workspace_artifact": "artifact_id",
            "analysis_phase_judgment": "judgment_id",
            "analysis_temp_scope": "scope_id",
        }
        for table_name, columns in definitions.items():
            self._ensure_table(engine, table_name, columns, primary_key=primary_keys.get(table_name))
        self._ensure_indexes(engine)
        self._invalidate_legacy_fact_caches(engine)

    def _invalidate_legacy_fact_caches(self, engine: DuckDBEngine) -> None:
        # These legacy tables predate TurnSecurityContext and cannot prove the
        # thread/turn/epoch/snapshot/source/query/schema binding needed for
        # factual reuse. Remove the schemas so no runtime path can repopulate
        # or read them after migration.
        for table_name in (
            "analysis_skill_cache",
            "analysis_scope_cache",
            "analysis_key_node_features",
            "analysis_signal_features",
            "analysis_refresh_state",
        ):
            engine.execute(f"DROP TABLE IF EXISTS {table_name}")

    def _ensure_table(
        self,
        engine: DuckDBEngine,
        table_name: str,
        columns: Sequence[tuple[str, str]],
        *,
        primary_key: Optional[str] = None,
    ) -> None:
        column_defs = [f"{name} {column_type}" for name, column_type in columns]
        if primary_key:
            column_defs.append(f"PRIMARY KEY({primary_key})")
        engine.execute(f"CREATE TABLE IF NOT EXISTS {table_name}({', '.join(column_defs)})")
        existing = self._table_columns(engine, table_name)
        for name, column_type in columns:
            if name not in existing:
                engine.execute(f"ALTER TABLE {table_name} ADD COLUMN IF NOT EXISTS {name} {column_type}")

    def _ensure_indexes(self, engine: DuckDBEngine) -> None:
        index_sql = [
            "CREATE INDEX IF NOT EXISTS idx_analysis_rule_hit_case_risk ON analysis_rule_hit(case_id, risk_type, score, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_rule_config_scope_param ON analysis_rule_config(scope_type, scope_key, param_key)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_case_finding_case_severity ON analysis_case_finding(case_id, severity, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_entity_node_case_type_key ON analysis_entity_node(case_id, entity_type, entity_key)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_relation_edge_case_src_type_dst ON analysis_relation_edge(case_id, src_entity_id, relation_type, dst_entity_id)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_trace_run_case_seed_created ON analysis_trace_run(case_id, seed_type, seed_value, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_evidence_ref_case_type_ref ON analysis_evidence_ref(case_id, evidence_type, ref_table, ref_pk)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_query_log_case_tool_created ON analysis_query_log(case_id, tool_name, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_workspace_file_case_name ON analysis_workspace_file(case_id, file_name)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_llm_session_case_updated ON analysis_llm_session(case_id, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_llm_session_case_pinned ON analysis_llm_session(case_id, pinned_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_memory_note_case_scope_updated ON analysis_memory_note(case_id, scope, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_memory_note_case_owner_updated ON analysis_memory_note(case_id, owner_id, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_audit_event_case_created ON analysis_audit_event(case_id, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_audit_event_run_turn ON analysis_audit_event(case_id, run_id, turn_id, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_run_log_case_created ON analysis_run_log_event(case_id, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_run_log_run_turn_seq ON analysis_run_log_event(case_id, run_id, turn_id, sequence_no)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_scratchpad_workspace_run_owner ON analysis_scratchpad_workspace(case_id, run_id, owner_type, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_scratchpad_workspace_retention ON analysis_scratchpad_workspace(case_id, retention_state, expires_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_workspace_artifact_run_workspace ON analysis_workspace_artifact(case_id, run_id, workspace_id, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_workspace_artifact_formalized ON analysis_workspace_artifact(case_id, is_formalized, artifact_type, created_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_phase_judgment_case_updated ON analysis_phase_judgment(case_id, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_phase_judgment_session_updated ON analysis_phase_judgment(case_id, session_id, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_phase_judgment_run_turn ON analysis_phase_judgment(case_id, run_id, turn_id, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_temp_scope_case_kind_updated ON analysis_temp_scope(case_id, source_kind, updated_at)",
            "CREATE INDEX IF NOT EXISTS idx_analysis_temp_scope_case_status_expiry ON analysis_temp_scope(case_id, status, expires_at)",
        ]
        for sql in index_sql:
            try:
                engine.execute(sql)
            except Exception:
                continue

    def _table_columns(self, engine: DuckDBEngine, table_name: str) -> set[str]:
        rows = engine.query(
            "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name=?",
            (table_name,),
        )
        return {str(row[0] or "") for row in rows if row and row[0]}

    def _ordered_table_columns(self, engine: DuckDBEngine, table_name: str) -> list[str]:
        rows = engine.query(
            """
            SELECT column_name
              FROM information_schema.columns
             WHERE table_schema='main' AND table_name=?
             ORDER BY ordinal_position
            """,
            (table_name,),
        )
        return [str(row[0]) for row in rows if row and row[0]]

    def _canonical_case_table_signature(
        self,
        engine: DuckDBEngine,
        *,
        table_name: str,
        case_id: str,
        domain: str,
        order_by: str,
    ) -> str:
        columns = self._ordered_table_columns(engine, table_name)
        if not columns or "case_id" not in columns:
            raise TransactionFactSourceUnavailableError()
        digest = hashlib.sha256()

        def update_framed(value: str) -> None:
            encoded = value.encode("utf-8")
            digest.update(len(encoded).to_bytes(8, "big"))
            digest.update(encoded)

        update_framed(domain)
        update_framed(_trim(case_id))
        for column in columns:
            update_framed(column)
        identifier = _quote_identifier(table_name)
        rows = engine.query(
            f"SELECT to_json(r) FROM {identifier} r WHERE r.case_id=? ORDER BY {order_by}, to_json(r)",
            (_trim(case_id),),
        )
        for row in rows:
            if len(row) != 1 or row[0] is None:
                raise TransactionFactSourceUnavailableError()
            update_framed(str(row[0]))
        return digest.hexdigest()

    def _table_exists(self, engine: DuckDBEngine, table_name: str) -> bool:
        return bool(self._table_columns(engine, table_name))

    def _stats_source_revision(self, engine: DuckDBEngine) -> int:
        try:
            return max(1, int(get_stats_flow_source_revision(engine) or 1))
        except Exception:
            return 1

    def _query_dicts(
        self,
        engine: DuckDBEngine,
        sql: str,
        params: Optional[Sequence[Any]] = None,
    ) -> list[dict[str, Any]]:
        with engine.connection_operation() as connection:
            if params is None:
                cursor = connection.execute(sql)
            else:
                cursor = connection.execute(sql, params)
            description = cursor.description or []
            columns = [str(item[0] or "") for item in description]
            rows = cursor.fetchall()
        return [{columns[index]: row[index] for index in range(len(columns))} for row in rows]

    def _normalize_llm_message_versions(self, value: Any, *, fallback_created_at: str) -> list[dict[str, Any]]:
        versions = value if isinstance(value, list) else []
        normalized: list[dict[str, Any]] = []
        for item in versions:
            if not isinstance(item, dict):
                continue
            content = _trim(item.get("content"))
            status = _trim(item.get("status")).lower() or "complete"
            created_at = _trim(item.get("createdAt")) or fallback_created_at
            started_at = _trim(item.get("startedAt")) or created_at
            completed_at = _trim(item.get("completedAt"))
            normalized.append(
                {
                    "content": content,
                    "status": status if status in {"complete", "streaming", "error"} else "complete",
                    "createdAt": created_at,
                    "startedAt": started_at,
                    "completedAt": completed_at,
                    "skillTrace": self._normalize_llm_skill_trace(item.get("skillTrace")),
                }
            )
        return normalized

    def _normalize_llm_messages(self, value: Any) -> list[dict[str, Any]]:
        messages = value if isinstance(value, list) else []
        normalized: list[dict[str, Any]] = []
        for item in messages:
            if not isinstance(item, dict):
                continue
            role = _trim(item.get("role")).lower()
            if role not in {"user", "assistant"}:
                continue
            created_at = _trim(item.get("createdAt")) or _now_text()
            versions = self._normalize_llm_message_versions(item.get("versions"), fallback_created_at=created_at)
            latest_version = versions[-1] if versions else None
            started_at = _trim(latest_version.get("startedAt") if latest_version else item.get("startedAt")) or created_at
            completed_at = _trim(latest_version.get("completedAt") if latest_version else item.get("completedAt"))
            normalized.append(
                {
                    "id": _trim(item.get("id")) or _slug("msg"),
                    "role": role,
                    "content": _trim(latest_version.get("content") if latest_version else item.get("content")),
                    "createdAt": _trim(latest_version.get("createdAt") if latest_version else created_at) or created_at,
                    "startedAt": started_at,
                    "completedAt": completed_at,
                    "status": _trim(latest_version.get("status") if latest_version else item.get("status")).lower() or "complete",
                    "persistForModel": bool(item.get("persistForModel", role == "user")),
                    "skillTrace": self._normalize_llm_skill_trace(item.get("skillTrace")),
                    "versionGroupId": _trim(item.get("versionGroupId")),
                    "versions": versions,
                }
            )
        return normalized

    def _normalize_llm_skill_trace(self, value: Any) -> dict[str, Any]:
        trace = value if isinstance(value, dict) else {}
        if not trace:
            return {}
        return dict(trace)

    def _normalize_llm_session_memory(self, value: Any) -> dict[str, Any]:
        source = value if isinstance(value, dict) else {}
        tags = source.get("tags") if isinstance(source.get("tags"), list) else []
        normalized_tags = []
        for item in tags:
            text = _trim(item)
            if text and text not in normalized_tags:
                normalized_tags.append(text)
        return {
            "focus_summary": _trim(source.get("focus_summary") or source.get("focus") or source.get("direction")),
            "conclusion_summary": _trim(source.get("conclusion_summary") or source.get("conclusion")),
            "next_steps_summary": _trim(source.get("next_steps_summary") or source.get("next_plan") or source.get("next_steps")),
            "covered_until_turn_id": _trim(source.get("covered_until_turn_id") or source.get("coveredUntilTurnId")),
            "covered_message_count": max(0, _as_int(source.get("covered_message_count") or source.get("coveredMessageCount"))),
            "tags": normalized_tags,
        }

    def _normalize_llm_memory_note(self, value: Any, *, actor_id: str = "") -> dict[str, Any]:
        source = value if isinstance(value, dict) else {}
        scope = _trim(source.get("scope")).lower()
        if scope not in MEMORY_NOTE_SCOPES:
            scope = MEMORY_NOTE_SCOPE_WORKSPACE
        owner_id = _trim(source.get("owner_id") or source.get("actor_id") or actor_id)
        if scope == MEMORY_NOTE_SCOPE_WORKSPACE:
            owner_id = ""
        else:
            owner_id = owner_id or "anonymous"
        allowed_types = MEMORY_NOTE_TYPES_BY_SCOPE.get(scope, ())
        memory_type = _trim(source.get("memory_type")).lower()
        if memory_type not in allowed_types:
            memory_type = (
                MEMORY_NOTE_TYPE_PROJECT_SIGNAL
                if scope == MEMORY_NOTE_SCOPE_WORKSPACE
                else MEMORY_NOTE_TYPE_OPERATOR_PREFERENCE
            )
        tags = source.get("tags") if isinstance(source.get("tags"), list) else []
        normalized_tags: list[str] = []
        for item in tags:
            text = _trim(item)
            if text and text not in normalized_tags:
                normalized_tags.append(text)
        note_source = _trim(source.get("source")).lower()
        if note_source not in {"manual", "derived"}:
            note_source = "manual"
        status = _trim(source.get("status")).lower()
        if status not in {"active", "archived"}:
            status = "active"
        title = _truncate_text(source.get("title"), 160)
        summary = _truncate_text(source.get("summary"), 320)
        detail = _truncate_text(source.get("detail"), 2400)
        freshness = _trim(source.get("freshness")).lower()
        if freshness not in MEMORY_NOTE_FRESHNESS_VALUES:
            freshness = MEMORY_NOTE_DEFAULT_FRESHNESS_BY_TYPE.get(memory_type, MEMORY_NOTE_FRESHNESS_TIME_SENSITIVE)
        trust_level = _trim(source.get("trust_level") or source.get("trustLevel")).lower()
        if trust_level not in MEMORY_NOTE_TRUST_VALUES:
            trust_level = MEMORY_NOTE_DEFAULT_TRUST_BY_TYPE.get(memory_type, MEMORY_NOTE_TRUST_WORKING)
        explicit_revalidation = source.get("required_revalidation")
        if explicit_revalidation is None and "requiredRevalidation" in source:
            explicit_revalidation = source.get("requiredRevalidation")
        required_revalidation = _optional_bool(explicit_revalidation)
        if required_revalidation is None:
            required_revalidation = bool(MEMORY_NOTE_DEFAULT_REVALIDATION_BY_TYPE.get(memory_type, True))
        created_at = _trim(source.get("created_at"))
        updated_at = _trim(source.get("updated_at"))
        index_text = _build_llm_memory_note_index_text(
            title=title,
            summary=summary,
            detail=detail,
            memory_type=memory_type,
            freshness=freshness,
            trust_level=trust_level,
            required_revalidation=required_revalidation,
            tags=normalized_tags,
        )
        return {
            "note_id": _trim(source.get("note_id")) or _slug("memnote"),
            "scope": scope,
            "owner_id": owner_id,
            "memory_type": memory_type,
            "title": title or "未命名记忆",
            "summary": summary,
            "detail": detail,
            "freshness": freshness,
            "trust_level": trust_level,
            "required_revalidation": required_revalidation,
            "tags": normalized_tags[:12],
            "source": note_source,
            "status": status,
            "index_text": index_text,
            "created_at": created_at,
            "updated_at": updated_at,
        }

    def _memory_note_row_to_payload(self, row: dict[str, Any]) -> dict[str, Any]:
        memory_type = _trim(row.get("memory_type")) or MEMORY_NOTE_TYPE_PROJECT_SIGNAL
        return {
            "note_id": _trim(row.get("note_id")),
            "scope": _trim(row.get("scope")) or MEMORY_NOTE_SCOPE_WORKSPACE,
            "owner_id": _trim(row.get("owner_id")),
            "memory_type": memory_type,
            "title": _trim(row.get("title")),
            "summary": _trim(row.get("summary")),
            "detail": _trim(row.get("detail")),
            "freshness": _trim(row.get("freshness")) or MEMORY_NOTE_DEFAULT_FRESHNESS_BY_TYPE.get(memory_type, MEMORY_NOTE_FRESHNESS_TIME_SENSITIVE),
            "trust_level": _trim(row.get("trust_level")) or MEMORY_NOTE_DEFAULT_TRUST_BY_TYPE.get(memory_type, MEMORY_NOTE_TRUST_WORKING),
            "required_revalidation": (
                _optional_bool(row.get("required_revalidation"))
                if _optional_bool(row.get("required_revalidation")) is not None
                else bool(MEMORY_NOTE_DEFAULT_REVALIDATION_BY_TYPE.get(memory_type, True))
            ),
            "tags": _trim_list(_json_loads(row.get("tags_json"), [])),
            "source": _trim(row.get("source")) or "manual",
            "status": _trim(row.get("status")) or "active",
            "created_at": _trim(row.get("created_at")),
            "updated_at": _trim(row.get("updated_at")),
        }

    def _normalize_session_data_binding(self, value: Any) -> dict[str, Any]:
        source = value if isinstance(value, dict) else {}
        scope_stats = source.get("scope_stats") if isinstance(source.get("scope_stats"), dict) else {}
        return {
            "binding_version": _optional_positive_int(source.get("binding_version")) or 1,
            "bound_source_revision": _optional_nonnegative_int(source.get("bound_source_revision")),
            "scope_signature": _trim(source.get("scope_signature")),
            "account_keys": _trim_list(source.get("account_keys") or []),
            "document_ids": _trim_list(source.get("document_ids") or []),
            "subject_ids": _trim_list(source.get("subject_ids") or []),
            "weak_subject_ids": _trim_list(source.get("weak_subject_ids") or []),
            "scope_stats": {
                "txn_count": _optional_nonnegative_int(scope_stats.get("txn_count")),
                "account_count": _optional_nonnegative_int(scope_stats.get("account_count")),
                "account_open_name": _trim(scope_stats.get("account_open_name")),
                "first_txn_at": _trim(scope_stats.get("first_txn_at")),
                "last_txn_at": _trim(scope_stats.get("last_txn_at")),
                "in_amount": _optional_rounded_float(scope_stats.get("in_amount"), 2),
                "out_amount": _optional_rounded_float(scope_stats.get("out_amount"), 2),
                "coverage_status": _trim(scope_stats.get("coverage_status")) or "unresolved",
            },
        }

    def _build_session_data_binding(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_ids: Sequence[str],
        subject_ids: Sequence[str],
        weak_subject_ids: Sequence[str],
        account_keys: Sequence[str],
    ) -> dict[str, Any]:
        current_revision = self._stats_source_revision(engine)
        normalized_document_ids = sorted(_trim_list(document_ids))
        normalized_subject_ids = sorted(_trim_list(subject_ids))
        normalized_weak_subject_ids = sorted(_trim_list(weak_subject_ids))
        normalized_account_keys = sorted(_trim_list(account_keys))
        scope_stats: dict[str, Any] = {}
        if normalized_account_keys and self._table_exists(engine, "analysis_txn_detail_idx"):
            where_sql, params = self._materialized_filtered_detail_scope(
                engine,
                case_id=case_id,
                account_keys=normalized_account_keys,
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            scope_stats = {
                "txn_count": _optional_nonnegative_int(scope_stats.get("txn_count")),
                "account_count": _optional_nonnegative_int(scope_stats.get("account_count")),
                "account_open_name": _trim(scope_stats.get("account_open_name")),
                "first_txn_at": _trim(scope_stats.get("first_txn_at")),
                "last_txn_at": _trim(scope_stats.get("last_txn_at")),
                "in_amount": _optional_rounded_float(scope_stats.get("in_amount"), 2),
                "out_amount": _optional_rounded_float(scope_stats.get("out_amount"), 2),
                "coverage_status": _trim(scope_stats.get("coverage_status")) or "unresolved",
            }
        signature_payload = {
            "binding_version": 1,
            "case_id": _trim(case_id),
            "source_revision": current_revision,
            "document_ids": normalized_document_ids,
            "subject_ids": normalized_subject_ids,
            "weak_subject_ids": normalized_weak_subject_ids,
            "account_keys": normalized_account_keys,
            "scope_stats": scope_stats,
        }
        return self._normalize_session_data_binding(
            {
                "binding_version": 1,
                "bound_source_revision": current_revision,
                "scope_signature": _stable_slug("session_scope", _json_dumps(signature_payload)),
                "document_ids": normalized_document_ids,
                "subject_ids": normalized_subject_ids,
                "weak_subject_ids": normalized_weak_subject_ids,
                "account_keys": normalized_account_keys,
                "scope_stats": scope_stats,
            }
        )

    def _merge_session_data_binding_into_skill_trace(
        self,
        trace: Any,
        *,
        binding: dict[str, Any],
    ) -> dict[str, Any]:
        normalized_trace = self._normalize_llm_skill_trace(trace)
        normalized_trace["session_data_binding"] = self._normalize_session_data_binding(binding)
        return normalized_trace

    def _merge_llm_session_skill_trace(
        self,
        existing_trace: Any,
        incoming_trace: Any,
    ) -> dict[str, Any]:
        existing = self._normalize_llm_skill_trace(existing_trace)
        incoming = self._normalize_llm_skill_trace(incoming_trace)
        if not existing:
            return incoming
        if not incoming:
            return existing

        merged = dict(existing)
        list_fields = (
            "planned_skills",
            "executed_skills",
            "execution_graph",
            "approval_requests",
            "denied_requests",
            "execution_errors",
        )
        dict_fields = (
            "planning",
            "policy_trace",
            "execution_control",
            "context_scope",
            "context_plan",
            "answer_strategy",
            "source_reference",
            "case_direction",
            "analysis_lens",
            "prompt_router",
            "prompt_contracts",
            "prompt_mode",
            "prompt_evidence_sufficiency",
            "proactive_follow_up",
        )

        for field_name in list_fields:
            incoming_items = [dict(item or {}) for item in list(incoming.get(field_name) or []) if isinstance(item, dict)]
            existing_items = [dict(item or {}) for item in list(existing.get(field_name) or []) if isinstance(item, dict)]
            if incoming_items:
                merged[field_name] = incoming_items
            elif existing_items:
                merged[field_name] = existing_items

        incoming_thread_items = [dict(item or {}) for item in list(incoming.get("thread_items_v3") or []) if isinstance(item, dict)]
        existing_thread_items = [dict(item or {}) for item in list(existing.get("thread_items_v3") or []) if isinstance(item, dict)]
        if incoming_thread_items or existing_thread_items:
            merged_thread_items = {str(item.get("id") or "").strip(): dict(item or {}) for item in existing_thread_items if str(item.get("id") or "").strip()}
            for item in incoming_thread_items:
                item_id = str(item.get("id") or "").strip()
                if item_id:
                    merged_thread_items[item_id] = dict(item or {})
            merged["thread_items_v3"] = list(merged_thread_items.values()) if merged_thread_items else (incoming_thread_items or existing_thread_items)

        for field_name in dict_fields:
            incoming_value = incoming.get(field_name)
            existing_value = existing.get(field_name)
            if isinstance(existing_value, dict) and isinstance(incoming_value, dict):
                merged[field_name] = {**existing_value, **incoming_value}
            elif incoming_value not in (None, "", {}, []):
                merged[field_name] = incoming_value
            elif field_name in existing:
                merged[field_name] = existing_value

        incoming_protocol = _as_int(incoming.get("protocol_version"))
        existing_protocol = _as_int(existing.get("protocol_version"))
        if incoming_protocol or existing_protocol:
            merged["protocol_version"] = max(existing_protocol, incoming_protocol)

        for key, value in incoming.items():
            if key in {*list_fields, *dict_fields, "thread_items_v3", "protocol_version"}:
                continue
            if value not in (None, "", {}, []):
                merged[key] = value
            elif key not in merged:
                merged[key] = value
        return self._normalize_llm_skill_trace(merged)

    def _merge_llm_session_domain_trace(
        self,
        existing_trace: Any,
        incoming_trace: Any,
    ) -> dict[str, Any]:
        existing = dict(existing_trace or {}) if isinstance(existing_trace, dict) else {}
        incoming = dict(incoming_trace or {}) if isinstance(incoming_trace, dict) else {}
        if not existing:
            return incoming
        if not incoming:
            return existing

        merged = dict(existing)
        for field_name in ("planning", "execution_control"):
            incoming_value = incoming.get(field_name)
            existing_value = existing.get(field_name)
            if isinstance(incoming_value, dict) and incoming_value:
                merged[field_name] = dict(incoming_value)
            elif isinstance(existing_value, dict) and existing_value:
                merged[field_name] = dict(existing_value)

        incoming_graph = [dict(item or {}) for item in list(incoming.get("execution_graph") or []) if isinstance(item, dict)]
        existing_graph = [dict(item or {}) for item in list(existing.get("execution_graph") or []) if isinstance(item, dict)]
        if incoming_graph:
            merged["execution_graph"] = incoming_graph
        elif existing_graph:
            merged["execution_graph"] = existing_graph

        for key, value in incoming.items():
            if key in {"planning", "execution_control", "execution_graph"}:
                continue
            if value not in (None, "", {}, []):
                merged[key] = value
            elif key not in merged:
                merged[key] = value
        return merged

    def _resolve_session_data_staleness(
        self,
        *,
        stored_binding: dict[str, Any],
        current_binding: dict[str, Any],
    ) -> tuple[str, str]:
        bound_revision = _optional_nonnegative_int(stored_binding.get("bound_source_revision"))
        current_revision = _optional_nonnegative_int(current_binding.get("bound_source_revision"))
        stored_signature = _trim(stored_binding.get("scope_signature"))
        current_signature = _trim(current_binding.get("scope_signature"))
        if bound_revision is None:
            return "unknown", "unbound_session_data"
        if current_revision is None:
            return "unknown", "current_source_revision_unresolved"
        if bound_revision <= 0 and not stored_signature:
            return "unknown", "unbound_session_data"
        if bound_revision > 0 and current_revision > 0 and bound_revision != current_revision:
            return "stale", "source_revision_changed"
        if stored_signature and current_signature and stored_signature != current_signature:
            return "stale", "session_scope_changed"
        if bound_revision > 0 or stored_signature:
            return "fresh", ""
        return "unknown", "unbound_session_data"

    def _build_llm_session_index_text(
        self,
        *,
        title: str,
        last_prompt: str,
        messages: Sequence[dict[str, Any]],
        memory: dict[str, Any],
    ) -> str:
        message_snippets = [_truncate_text(item.get("content"), 240) for item in messages[-4:]]
        parts = [
            title,
            last_prompt,
            _trim(memory.get("focus_summary")),
            _trim(memory.get("conclusion_summary")),
            _trim(memory.get("next_steps_summary")),
            " ".join(_trim_list(memory.get("tags") or [])),
            " ".join(item for item in message_snippets if item),
        ]
        return "\n".join(part for part in parts if part)

    def _normalize_llm_session_payload(self, session: dict[str, Any]) -> dict[str, Any]:
        created_at = _trim(session.get("createdAt")) or _now_text()
        updated_at = _trim(session.get("updatedAt")) or created_at
        messages = self._normalize_llm_messages(session.get("messages"))
        memory = self._normalize_llm_session_memory(session.get("memory"))
        title = _trim(session.get("title")) or "新会话"
        last_prompt = _trim(session.get("lastPrompt"))
        normalized = {
            "id": _trim(session.get("id")) or _slug("session"),
            "title": title,
            "createdAt": created_at,
            "updatedAt": updated_at,
            "pinnedAt": _trim(session.get("pinnedAt")),
            "documentIds": _trim_list(session.get("documentIds") or []),
            "subjectIds": _trim_list(session.get("subjectIds") or []),
            "weakSubjectIds": _trim_list(session.get("weakSubjectIds") or []),
            "accountKeys": _trim_list(session.get("accountKeys") or []),
            "lastPrompt": last_prompt,
            "messages": messages,
            "memory": memory,
            "skillTrace": self._normalize_llm_skill_trace(session.get("skillTrace")),
            "domainTrace": session.get("domainTrace") if isinstance(session.get("domainTrace"), dict) else {},
        }
        normalized["indexText"] = self._build_llm_session_index_text(
            title=title,
            last_prompt=last_prompt,
            messages=messages,
            memory=memory,
        )
        return normalized

    def _default_phase_judgment_payload(self) -> dict[str, Any]:
        return {
            "enabled": False,
            "judgment_id": "",
            "case_id": "",
            "session_id": "",
            "run_id": "",
            "turn_id": "",
            "main_line_summary": "",
            "stage_conclusion_summary": "",
            "priority_action_summary": "",
            "status": "empty",
            "confidence": {},
            "tags": [],
            "source_query_ids": [],
            "source_evidence_ids": [],
            "source_path_ids": [],
            "source_report_ids": [],
            "derived_from": {},
            "policy": {"visible": False, "background_only": True},
            "visible": False,
            "created_at": "",
            "updated_at": "",
        }

    def _normalize_llm_phase_judgment(self, value: Any) -> dict[str, Any]:
        source = value if isinstance(value, dict) else {}
        tags = _trim_list(source.get("tags") or [])
        confidence_source = source.get("confidence") if isinstance(source.get("confidence"), dict) else {}
        status = _trim(source.get("status")).lower()
        if status not in {"empty", "session_draft", "working_hypothesis", "evidence_backed", "approval_blocked", "canonical", "stale", "conflict"}:
            status = "empty"
        return {
            "enabled": bool(source.get("enabled")) and status != "empty",
            "judgment_id": _trim(source.get("judgment_id")),
            "case_id": _trim(source.get("case_id")),
            "session_id": _trim(source.get("session_id")),
            "run_id": _trim(source.get("run_id")),
            "turn_id": _trim(source.get("turn_id")),
            "main_line_summary": _truncate_text(source.get("main_line_summary"), 240),
            "stage_conclusion_summary": _truncate_text(source.get("stage_conclusion_summary"), 320),
            "priority_action_summary": _truncate_text(source.get("priority_action_summary"), 240),
            "status": status,
            "confidence": {
                "main_line": _optional_confidence(confidence_source.get("main_line")),
                "conclusion": _optional_confidence(confidence_source.get("conclusion")),
                "action": _optional_confidence(confidence_source.get("action")),
            },
            "tags": tags[:12],
            "source_query_ids": _trim_list(source.get("source_query_ids") or []),
            "source_evidence_ids": _trim_list(source.get("source_evidence_ids") or []),
            "source_path_ids": _trim_list(source.get("source_path_ids") or []),
            "source_report_ids": _trim_list(source.get("source_report_ids") or []),
            "derived_from": source.get("derived_from") if isinstance(source.get("derived_from"), dict) else {},
            "policy": source.get("policy") if isinstance(source.get("policy"), dict) else {},
            "visible": bool(source.get("visible")),
        }

    def _first_text_expr(self, columns: set[str], alias: str, candidates: Sequence[str]) -> str:
        parts = [f"NULLIF(TRIM({alias}.{name}), '')" for name in candidates if name in columns]
        if not parts:
            return "NULL"
        return f"COALESCE({', '.join(parts)})"

    def _first_num_expr(self, columns: set[str], alias: str, candidates: Sequence[str]) -> str:
        return _first_finite_number_sql(columns, candidates, alias=alias)

    def _txn_ts_expr(self, columns: set[str], alias: str) -> str:
        if "txn_ts" in columns:
            if "txn_time" in columns:
                return f"COALESCE({alias}.txn_ts, {_ts_norm_expr(f'{alias}.txn_time')})"
            return f"{alias}.txn_ts"
        if "txn_time" in columns:
            return _ts_norm_expr(f"{alias}.txn_time")
        return "NULL"

    def _profile_row(self, engine: DuckDBEngine, case_id: str) -> Optional[dict[str, Any]]:
        rows = self._query_dicts(engine, "SELECT * FROM analysis_case_profile WHERE case_id=? LIMIT 1", (case_id,))
        if not rows:
            return None
        row = rows[0]
        row["scope_targets_json"] = _trim(row.get("scope_targets_json")) or "[]"
        return row

    def _scope_rows(self, engine: DuckDBEngine, case_id: str) -> list[dict[str, Any]]:
        return self._query_dicts(
            engine,
            "SELECT * FROM analysis_case_scope WHERE case_id=? ORDER BY priority ASC, created_at ASC",
            (case_id,),
        )

    def _hypothesis_rows(self, engine: DuckDBEngine, case_id: str) -> list[dict[str, Any]]:
        return self._query_dicts(
            engine,
            "SELECT * FROM analysis_case_hypothesis WHERE case_id=? ORDER BY updated_at DESC, created_at DESC",
            (case_id,),
        )

    def _finding_rows(self, engine: DuckDBEngine, case_id: str) -> list[dict[str, Any]]:
        return self._query_dicts(
            engine,
            "SELECT * FROM analysis_case_finding WHERE case_id=? ORDER BY updated_at DESC, created_at DESC",
            (case_id,),
        )

    def _note_rows(self, engine: DuckDBEngine, case_id: str, *, day_text: Optional[str] = None) -> list[dict[str, Any]]:
        if day_text:
            return self._query_dicts(
                engine,
                "SELECT * FROM analysis_case_note WHERE case_id=? AND substr(created_at, 1, 10)=? ORDER BY created_at DESC",
                (case_id, day_text),
            )
        return self._query_dicts(
            engine,
            "SELECT * FROM analysis_case_note WHERE case_id=? ORDER BY created_at DESC",
            (case_id,),
        )

    def _evidence_ref_rows_for_ids(
        self,
        engine: DuckDBEngine,
        case_id: str,
        evidence_ids: Sequence[str],
    ) -> list[dict[str, Any]]:
        normalized_ids = _unique_texts(evidence_ids)
        if not normalized_ids:
            return []
        rows = self._query_dicts(
            engine,
            "SELECT * FROM analysis_evidence_ref WHERE case_id=? ORDER BY created_at DESC",
            (case_id,),
        )
        lookup = {
            _trim(row.get("evidence_id")): {
                "evidence_id": _trim(row.get("evidence_id")),
                "evidence_type": _trim(row.get("evidence_type")),
                "title": _trim(row.get("title")),
                "snippet": _trim(row.get("snippet")),
                "payload_json": _json_loads(row.get("payload_json"), {}),
            }
            for row in rows
            if _trim(row.get("evidence_id"))
        }
        return [lookup[item] for item in normalized_ids if item in lookup]

    def _insert_case_note_record(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        title: str,
        content_md: str,
        author: str,
        source_query_ids: Sequence[str],
        source_evidence_ids: Sequence[str],
    ) -> tuple[str, str]:
        note_id = _slug("note")
        created_at = _now_text()
        linked_query_ids = _trim_list(source_query_ids)
        linked_evidence_ids = _trim_list(source_evidence_ids)
        engine.execute(
            """
            INSERT INTO analysis_case_note(
                note_id, case_id, title, content_md, author, source_query_ids_json, source_evidence_ids_json, created_at
            ) VALUES (?,?,?,?,?,?,?,?)
            """,
            (
                note_id,
                case_id,
                _trim(title) or "未命名笔记",
                content_md,
                _trim(author) or _safe_user(),
                _json_dumps(linked_query_ids),
                _json_dumps(linked_evidence_ids),
                created_at,
            ),
        )
        note_evidence_id = self._upsert_evidence_ref(
            engine,
            case_id=case_id,
            evidence_type="case_note",
            ref_table="analysis_case_note",
            ref_pk=note_id,
            title=_trim(title) or "未命名笔记",
            snippet=_trim(content_md)[:200],
            payload={
                "case_id": case_id,
                "note_id": note_id,
                "source_query_ids": linked_query_ids,
                "source_evidence_ids": linked_evidence_ids,
            },
        )
        return note_id, note_evidence_id

    def _workspace_rows(self, engine: DuckDBEngine, case_id: str) -> list[dict[str, Any]]:
        return self._query_dicts(
            engine,
            "SELECT file_name, version, updated_at FROM analysis_workspace_file WHERE case_id=? ORDER BY file_name ASC",
            (case_id,),
        )

    def _build_workspace_projection(
        self,
        *,
        profile: Optional[dict[str, Any]],
        workspace_rows: Sequence[dict[str, Any]],
    ) -> dict[str, Any]:
        rows = list(workspace_rows or [])
        refresh_candidates = [
            _trim((profile or {}).get("updated_at")),
            *[_trim(row.get("updated_at")) for row in rows],
        ]
        refresh_points = [item for item in refresh_candidates if item]
        return {
            "files": [row["file_name"] for row in rows],
            "file_count": len(rows),
            "last_refresh_at": sorted(refresh_points)[-1] if refresh_points else "",
            "rendered_artifact": bool(rows),
        }

    def _build_project_docs_projection(self, case_id: str) -> dict[str, Any]:
        case_dir = self._storage.case_dir(case_id)
        visible_docs = [
            (
                CASE_PROJECT_DOC_FILENAME,
                "stable_case_background",
                case_project_doc_path(case_dir),
            ),
            (
                CASE_DIRECTION_DOC_FILENAME,
                "working_direction",
                case_direction_doc_path(case_dir),
            ),
        ]
        files: list[dict[str, Any]] = []
        for file_name, role, path in visible_docs:
            exists = path.exists()
            text = ""
            if exists:
                try:
                    text = path.read_text(encoding="utf-8", errors="ignore")
                except OSError:
                    text = ""
            files.append(
                {
                    "file_name": file_name,
                    "role": role,
                    "exists": exists,
                    "sha256": _file_sha256(path),
                    "content_chars": len(text),
                    "preview": text[:1200],
                }
            )
        runtime_path = case_runtime_project_doc_path(case_dir)
        return {
            "editable_files": files,
            "runtime_project_doc": {
                "file_name": CASE_RUNTIME_PROJECT_DOC_FILENAME,
                "exists": runtime_path.exists(),
                "sha256": _file_sha256(runtime_path),
                "source_files": [CASE_PROJECT_DOC_FILENAME, CASE_DIRECTION_DOC_FILENAME],
            },
        }

    def _query_log_rows(self, engine: DuckDBEngine, case_id: str, limit: int = 20) -> list[dict[str, Any]]:
        rows = self._query_dicts(
            engine,
            "SELECT * FROM analysis_query_log WHERE case_id=? ORDER BY created_at DESC LIMIT ?",
            (case_id, max(1, int(limit))),
        )
        projected: list[dict[str, Any]] = []
        for row in rows:
            if _trim(row.get("case_id")) != _trim(case_id):
                raise RuntimeError("diagnostic_case_binding_mismatch")
            history = project_workbench_history_item(
                case_id=case_id,
                row={
                    **row,
                    "params_json": _json_loads(row.get("params_json"), {}),
                    "summary_json": _json_loads(row.get("summary_json"), {}),
                },
                host_registry_member=True,
            )
            projected.append(
                {
                    "query_id": history["query_ref"],
                    "tool_name": history["tool_category"],
                    "row_count": None,
                    "row_count_status": history["result_check_status"],
                    "created_at": project_diagnostic_timestamp(row.get("created_at")),
                }
            )
        return projected

    def _count(self, engine: DuckDBEngine, table_name: str, case_id: str) -> int | None:
        if not self._table_exists(engine, table_name):
            return None
        rows = engine.query(f"SELECT COUNT(1) FROM {table_name} WHERE case_id=?", (case_id,))
        return _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None

    def _analysis_refresh_txn_count(self, case_id: str) -> int | None:
        try:
            engine = self.open_case_engine(case_id, read_only=True)
        except (FileNotFoundError, DataEngineUnavailableError, DiagnosticDuckDBMigrationError):
            return None
        try:
            try:
                return self._analysis_refresh_txn_count_from_engine(engine, case_id)
            except (DataEngineUnavailableError, DiagnosticDuckDBMigrationError):
                return None
        finally:
            engine.close()

    def _analysis_refresh_txn_count_from_engine(self, engine: DuckDBEngine, case_id: str) -> int | None:
        if not self._table_exists(engine, "fc_transaction_norm"):
            return None
        columns = set(self._table_columns(engine, "fc_transaction_norm"))
        if not self._txn_cleaning_authority_ready(engine, case_id, columns):
            return None
        predicate = self._txn_clean_acceptance_predicate(columns, alias="t")
        rows = engine.query(
            f"SELECT COUNT(1) FROM fc_transaction_norm t WHERE t.case_id=? AND {predicate}",
            (case_id,),
        )
        row = _exact_single_query_row(rows, width=1)
        if row is None:
            return None
        return _exact_nonnegative_db_int(row[0])

    def _upsert_profile(self, engine: DuckDBEngine, payload: dict[str, Any]) -> None:
        scope_targets_json = _trim(payload.get("scope_targets_json")) or "[]"
        existing = self._profile_row(engine, payload["case_id"])
        if existing:
            engine.execute(
                """
                UPDATE analysis_case_profile
                   SET case_name=?, case_number=?, case_type=?, owner=?, tags_json=?, note=?,
                       scope_targets_json=?, initial_direction=?, status=?, created_at=?, updated_at=?
                 WHERE case_id=?
                """,
                (
                    payload["case_name"],
                    payload["case_number"],
                    payload["case_type"],
                    payload["owner"],
                    payload["tags_json"],
                    payload["note"],
                    scope_targets_json,
                    payload["initial_direction"],
                    payload["status"],
                    payload["created_at"],
                    payload["updated_at"],
                    payload["case_id"],
                ),
            )
            return
        engine.execute(
            """
            INSERT INTO analysis_case_profile(
                case_id, case_name, case_number, case_type, owner, tags_json, note,
                scope_targets_json, initial_direction, status, created_at, updated_at
            ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
            """,
            (
                payload["case_id"],
                payload["case_name"],
                payload["case_number"],
                payload["case_type"],
                payload["owner"],
                payload["tags_json"],
                payload["note"],
                scope_targets_json,
                payload["initial_direction"],
                payload["status"],
                payload["created_at"],
                payload["updated_at"],
            ),
        )

    def _upsert_workspace_file(
        self,
        engine: DuckDBEngine,
        case_id: str,
        file_name: str,
        content_md: str,
        *,
        render_mode: str,
    ) -> dict[str, Any]:
        file_name = self._normalize_ordinary_workspace_file_name(file_name)
        content_md = self._privacy_projection_repository.project_ordinary_artifact_text(
            case_id,
            content_md,
        )
        file_key = f"{case_id}:{file_name}"
        source_hash = _hash_content(content_md)
        updated_at = _now_text()
        rows = self._query_dicts(
            engine,
            "SELECT version, source_hash, updated_at FROM analysis_workspace_file WHERE file_key=? LIMIT 1",
            (file_key,),
        )
        current = rows[0] if rows else None
        if current:
            version = int(current.get("version") or 1)
            if _trim(current.get("source_hash")) != source_hash:
                version += 1
                updated_at = _now_text()
            else:
                updated_at = _trim(current.get("updated_at")) or updated_at
            engine.execute(
                """
                UPDATE analysis_workspace_file
                   SET content_md=?, version=?, source_hash=?, render_mode=?, updated_at=?
                 WHERE file_key=?
                """,
                (content_md, version, source_hash, render_mode, updated_at, file_key),
            )
        else:
            version = 1
            engine.execute(
                """
                INSERT INTO analysis_workspace_file(
                    file_key, case_id, file_name, content_md, version, source_hash, render_mode, updated_at
                ) VALUES (?,?,?,?,?,?,?,?)
                """,
                (file_key, case_id, file_name, content_md, version, source_hash, render_mode, updated_at),
            )
        target = self.workspace_dir(case_id) / file_name
        target.parent.mkdir(parents=True, exist_ok=True)
        if target.is_symlink() or target.parent.is_symlink():
            raise RuntimeError("workspace_artifact_path_invalid")
        atomic_write_private_text(target, content_md)
        return {
            "file_name": file_name,
            "content_md": content_md,
            "version": version,
            "updated_at": updated_at,
        }

    def write_workspace_projection_file(
        self,
        case_id: str,
        *,
        file_name: str,
        content_md: str,
        render_mode: str,
    ) -> dict[str, Any]:
        normalized_file_name = self._normalize_ordinary_workspace_file_name(file_name)
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            return self._upsert_workspace_file(
                engine,
                case_id,
                normalized_file_name,
                content_md,
                render_mode=render_mode,
            )
        finally:
            engine.close()

    def attach_report_workspace_projection(
        self,
        case_id: str,
        *,
        report_id: str,
        workspace_file: Optional[dict[str, Any]] = None,
        confirmed_workspace_file: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        del case_id, report_id, workspace_file, confirmed_workspace_file
        # A workspace/report association is a publication effect. The legacy
        # repository path has no PublicationReceipt input and therefore cannot
        # prove the report hash, claim ledger, snapshot, or PII projection.
        raise RuntimeError("report_publication_receipt_required")

    def _upsert_evidence_ref(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        evidence_type: str,
        ref_table: str,
        ref_pk: str,
        title: str,
        snippet: str,
        payload: dict[str, Any],
    ) -> str:
        if _trim(evidence_type) == "txn" and _optional_finite_float(payload.get("amount")) is None:
            # A legacy reference must not turn an unresolved amount into a
            # citable transaction fact. Host EvidenceReceipts are issued by a
            # separate authority and are never synthesized here.
            return ""
        rows = self._query_dicts(
            engine,
            """
            SELECT evidence_id FROM analysis_evidence_ref
             WHERE case_id=? AND ref_table=? AND ref_pk=?
             LIMIT 1
            """,
            (case_id, ref_table, ref_pk),
        )
        created_at = _now_text()
        payload_json = _json_dumps(payload)
        if rows:
            return _trim(rows[0].get("evidence_id"))
        evidence_id = _slug("ev")
        engine.execute(
            """
            INSERT INTO analysis_evidence_ref(
                evidence_id, case_id, evidence_type, ref_table, ref_pk, title, snippet, payload_json, created_at
            ) VALUES (?,?,?,?,?,?,?,?,?)
            """,
            (evidence_id, case_id, evidence_type, ref_table, ref_pk, title, snippet, payload_json, created_at),
        )
        return evidence_id

    def _append_query_log(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        query_id: str | None = None,
        tool_name: str,
        params: dict[str, Any],
        summary: dict[str, Any],
        row_count: int | None,
        duration_ms: int,
    ) -> str:
        if evidence_pack_skill_requires_host_authority(tool_name):
            return ""
        candidate_query_id = _trim(query_id) or _slug("q")
        public_params, public_summary = project_query_log_diagnostic(
            case_id=case_id,
            query_id=candidate_query_id,
            params=params,
            summary=summary,
        )
        query_id = public_summary["query_ref"]
        engine.execute(
            """
            INSERT INTO analysis_query_log(
                query_id, case_id, tool_name, params_json, summary_json, row_count, duration_ms, created_at
            ) VALUES (?,?,?,?,?,?,?,?)
            """,
            (
                query_id,
                case_id,
                project_query_tool_name(tool_name),
                _json_dumps(public_params),
                _json_dumps(public_summary),
                None,
                max(0, int(duration_ms)),
                _now_text(),
            ),
        )
        return query_id

    def append_query_log_entry(
        self,
        case_id: str,
        *,
        query_id: str,
        tool_name: str,
        params: dict[str, Any],
        summary: dict[str, Any],
        row_count: int,
        duration_ms: int,
    ) -> str:
        if evidence_pack_skill_requires_host_authority(tool_name):
            return ""
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        candidate_query_id = _trim(query_id) or _slug("q")
        normalized_query_id = normalize_case_sql_query_ref(
            case_id=case_id,
            query_id=candidate_query_id,
        )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing = self._query_dicts(
                engine,
                "SELECT query_id FROM analysis_query_log WHERE case_id=? AND query_id=? LIMIT 1",
                (case_id, normalized_query_id),
            )
            if existing:
                return normalized_query_id
            return self._append_query_log(
                engine,
                case_id=case_id,
                query_id=candidate_query_id,
                tool_name=tool_name,
                params=params,
                summary=summary,
                row_count=row_count,
                duration_ms=duration_ms,
            )
        finally:
            engine.close()

    def _top_accounts(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        limit: int = 6,
        time_range: Optional[dict[str, str]] = None,
        account_filter: str = "",
    ) -> list[dict[str, Any]]:
        if not self._table_exists(engine, "fc_transaction_norm"):
            return []
        columns = self._table_columns(engine, "fc_transaction_norm")
        account_key_expr = self._first_text_expr(
            columns,
            "t",
            ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
        )
        if account_key_expr == "NULL":
            raise TransactionFactSourceUnavailableError()
        account_name_expr = self._first_text_expr(columns, "t", ("account_open_name",))
        amount_expr = self._first_num_expr(columns, "t", ("clean_amount", "amount_val", "amount"))
        txn_ts_expr = self._txn_ts_expr(columns, "t")
        in_expr = self._first_text_expr(columns, "t", ("dc_final", "clean_dc_flag", "dc_flag"))
        where_parts = ["t.case_id=?", f"{account_key_expr} IS NOT NULL"]
        params: list[Any] = [case_id]
        if account_filter:
            where_parts.append(f"{account_key_expr}=?")
            params.append(account_filter)
        if time_range and _trim(time_range.get("start")) and txn_ts_expr != "NULL":
            where_parts.append(f"{txn_ts_expr} >= {_ts_norm_expr('?')}")
            params.append(_trim(time_range.get("start")))
        if time_range and _trim(time_range.get("end")) and txn_ts_expr != "NULL":
            where_parts.append(f"{txn_ts_expr} <= {_ts_norm_expr('?')}")
            params.append(_trim(time_range.get("end")))
        sql = f"""
            SELECT
                {account_key_expr} AS account_key,
                COALESCE({account_name_expr}, {account_key_expr}) AS display_name,
                COUNT(1) AS txn_count,
                COUNT({amount_expr}) AS amount_present_count,
                COUNT(1) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_txn_count,
                COUNT(1) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_txn_count,
                COUNT({amount_expr}) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_amount_present_count,
                COUNT({amount_expr}) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_amount_present_count,
                SUM(ABS({amount_expr})) AS turnover_amount,
                SUM(ABS({amount_expr})) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_amount,
                SUM(ABS({amount_expr})) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_amount,
                MIN({txn_ts_expr}) AS first_seen_at,
                MAX({txn_ts_expr}) AS last_seen_at
            FROM fc_transaction_norm t
            WHERE {' AND '.join(where_parts)}
            GROUP BY 1, 2
            ORDER BY txn_count DESC, turnover_amount DESC, account_key ASC
            LIMIT ?
        """
        params.append(max(1, int(limit)))
        rows = self._query_dicts(engine, sql, params)
        for row in rows:
            txn_count = _optional_nonnegative_int(row.get("txn_count"))
            row["txn_count"] = txn_count
            row["turnover_amount"] = _complete_aggregate_amount(
                row.get("turnover_amount"),
                expected_count=txn_count,
                present_count=row.get("amount_present_count"),
            )
            row["in_amount"] = _complete_aggregate_amount(
                row.get("in_amount"),
                expected_count=row.get("in_txn_count"),
                present_count=row.get("in_amount_present_count"),
            )
            row["out_amount"] = _complete_aggregate_amount(
                row.get("out_amount"),
                expected_count=row.get("out_txn_count"),
                present_count=row.get("out_amount_present_count"),
            )
            for internal_key in (
                "amount_present_count",
                "in_txn_count",
                "out_txn_count",
                "in_amount_present_count",
                "out_amount_present_count",
            ):
                row.pop(internal_key, None)
            row["first_seen_at"] = _trim(row.get("first_seen_at"))
            row["last_seen_at"] = _trim(row.get("last_seen_at"))
        return rows

    def _summary_case_direction_context(self, profile: Mapping[str, Any]) -> dict[str, Any]:
        return build_case_direction_context(
            profile.get("case_type"),
            _json_loads(profile.get("tags_json"), []),
            profile.get("note"),
        )

    def _summary_case_direction_focus(self, profile: Mapping[str, Any], theme: str) -> str:
        return build_case_direction_focus(
            profile.get("case_type"),
            _json_loads(profile.get("tags_json"), []),
            theme,
            profile.get("note"),
        )

    def _build_account_set_summaries(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        time_range: Optional[dict[str, str]] = None,
    ) -> dict[str, Any]:
        if not self._table_exists(engine, "fc_transaction_norm"):
            return {
                "grouped_count": None,
                "ungrouped_count": None,
                "total_count": None,
                "items": [],
            }
        columns = self._table_columns(engine, "fc_transaction_norm")
        account_key_expr = self._first_text_expr(
            columns,
            "t",
            ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
        )
        if account_key_expr == "NULL":
            return {
                "grouped_count": None,
                "ungrouped_count": None,
                "total_count": None,
                "items": [],
            }
        holder_name_expr = self._first_text_expr(columns, "t", ("account_open_name",))
        holder_id_expr = self._first_text_expr(columns, "t", ("opener_id_no",))
        amount_expr = self._first_num_expr(columns, "t", ("clean_amount", "amount_val", "amount"))
        txn_ts_expr = self._txn_ts_expr(columns, "t")
        in_expr = self._first_text_expr(columns, "t", ("dc_final", "clean_dc_flag", "dc_flag"))
        where_parts = ["t.case_id=?", f"{account_key_expr} IS NOT NULL"]
        params: list[Any] = [case_id]
        if time_range and _trim(time_range.get("start")) and txn_ts_expr != "NULL":
            where_parts.append(f"{txn_ts_expr} >= {_ts_norm_expr('?')}")
            params.append(_trim(time_range.get("start")))
        if time_range and _trim(time_range.get("end")) and txn_ts_expr != "NULL":
            where_parts.append(f"{txn_ts_expr} <= {_ts_norm_expr('?')}")
            params.append(_trim(time_range.get("end")))
        holder_name_sql = (
            f"NULLIF(TRIM(MAX(CASE WHEN {holder_name_expr} IS NOT NULL AND TRIM(CAST({holder_name_expr} AS VARCHAR)) <> '' "
            f"THEN CAST({holder_name_expr} AS VARCHAR) END)), '')"
            if holder_name_expr != "NULL"
            else "NULL"
        )
        holder_id_sql = (
            f"NULLIF(TRIM(MAX(CASE WHEN {holder_id_expr} IS NOT NULL AND TRIM(CAST({holder_id_expr} AS VARCHAR)) <> '' "
            f"THEN CAST({holder_id_expr} AS VARCHAR) END)), '')"
            if holder_id_expr != "NULL"
            else "NULL"
        )
        sql = f"""
            SELECT
                {account_key_expr} AS account_key,
                {holder_name_sql} AS holder_name,
                {holder_id_sql} AS holder_id_no,
                COUNT(1) AS txn_count,
                COUNT({amount_expr}) AS amount_present_count,
                COUNT(1) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_txn_count,
                COUNT(1) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_txn_count,
                COUNT({amount_expr}) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_amount_present_count,
                COUNT({amount_expr}) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_amount_present_count,
                SUM(ABS({amount_expr})) AS turnover_amount,
                SUM(ABS({amount_expr})) FILTER (WHERE {in_expr} LIKE '%进%' OR {in_expr} LIKE '%入%' OR {in_expr} LIKE '%贷%' OR {in_expr} LIKE '%收%') AS in_amount,
                SUM(ABS({amount_expr})) FILTER (WHERE {in_expr} LIKE '%出%' OR {in_expr} LIKE '%付%' OR {in_expr} LIKE '%借%') AS out_amount,
                MIN({txn_ts_expr}) AS first_seen_at,
                MAX({txn_ts_expr}) AS last_seen_at
            FROM fc_transaction_norm t
            WHERE {' AND '.join(where_parts)}
            GROUP BY 1
            ORDER BY 1
        """
        rows = self._query_dicts(engine, sql, params)
        grouped: dict[str, dict[str, Any]] = {}
        for row in rows:
            account_key = _trim(row.get("account_key"))
            if not account_key:
                continue
            holder_name = _trim(row.get("holder_name"))
            holder_id_no = _trim(row.get("holder_id_no"))
            if holder_name and not holder_id_no:
                normalized_holder_compact = re.sub(r"\s+", "", holder_name)
                normalized_account_compact = re.sub(r"\s+", "", account_key)
                if normalized_holder_compact in {
                    normalized_account_compact,
                    f"账户{normalized_account_compact}",
                    f"账号{normalized_account_compact}",
                    f"卡号{normalized_account_compact}",
                }:
                    holder_name = ""
            txn_count = _optional_nonnegative_int(row.get("txn_count"))
            turnover_amount = _complete_aggregate_amount(
                row.get("turnover_amount"),
                expected_count=txn_count,
                present_count=row.get("amount_present_count"),
            )
            in_amount = _complete_aggregate_amount(
                row.get("in_amount"),
                expected_count=row.get("in_txn_count"),
                present_count=row.get("in_amount_present_count"),
            )
            out_amount = _complete_aggregate_amount(
                row.get("out_amount"),
                expected_count=row.get("out_txn_count"),
                present_count=row.get("out_amount_present_count"),
            )
            first_seen_at = _trim(row.get("first_seen_at"))
            last_seen_at = _trim(row.get("last_seen_at"))
            if holder_id_no:
                group_key = f"id:{holder_id_no}"
                grouping_status = "grouped"
                label = holder_name or account_key
                reason = "按证件号归集"
            elif holder_name:
                group_key = f"name:{holder_name}"
                grouping_status = "grouped"
                label = holder_name
                reason = "按户名归集"
            else:
                group_key = f"acct:{account_key}"
                grouping_status = "ungrouped"
                label = account_key
                reason = "账户缺少户名，暂按单账户保留"
            item = grouped.get(group_key)
            if item is None:
                item = {
                    "set_id": group_key,
                    "label": label,
                    "holder_name": holder_name,
                    "holder_id_no": holder_id_no,
                    "grouping_status": grouping_status,
                    "reason": reason,
                    "account_count": 0,
                    "account_keys": [],
                    "txn_count": txn_count,
                    "turnover_amount": turnover_amount,
                    "in_amount": in_amount,
                    "out_amount": out_amount,
                    "first_seen_at": first_seen_at,
                    "last_seen_at": last_seen_at,
                }
                grouped[group_key] = item
            item["account_count"] = _as_int(item.get("account_count")) + 1
            item["account_keys"] = _unique_texts([*(item.get("account_keys") or []), account_key])
            if len(item["account_keys"]) > 1:
                item["txn_count"] = _complete_count_sum((item.get("txn_count"), txn_count))
                item["turnover_amount"] = _complete_amount_sum((item.get("turnover_amount"), turnover_amount))
                item["in_amount"] = _complete_amount_sum((item.get("in_amount"), in_amount))
                item["out_amount"] = _complete_amount_sum((item.get("out_amount"), out_amount))
            if first_seen_at and (not _trim(item.get("first_seen_at")) or first_seen_at < _trim(item.get("first_seen_at"))):
                item["first_seen_at"] = first_seen_at
            if last_seen_at and (not _trim(item.get("last_seen_at")) or last_seen_at > _trim(item.get("last_seen_at"))):
                item["last_seen_at"] = last_seen_at

        def _account_set_sort_key(item: Mapping[str, Any]) -> tuple[Any, ...]:
            txn_count = _optional_nonnegative_int(item.get("txn_count"))
            turnover_amount = _optional_finite_float(item.get("turnover_amount"))
            return (
                0 if _trim(item.get("grouping_status")) == "grouped" else 1,
                txn_count is None,
                -(txn_count if txn_count is not None else 0),
                turnover_amount is None,
                -(turnover_amount if turnover_amount is not None else 0.0),
                _trim(item.get("label")),
            )

        items = sorted(grouped.values(), key=_account_set_sort_key)
        grouped_count = sum(1 for item in items if _trim(item.get("grouping_status")) == "grouped")
        ungrouped_count = sum(1 for item in items if _trim(item.get("grouping_status")) == "ungrouped")
        return {
            "grouped_count": grouped_count,
            "ungrouped_count": ungrouped_count,
            "total_count": len(items),
            "items": items[:32],
            "truncated": len(items) > 32,
        }

    def _build_scope_summary(self, scopes: Sequence[dict[str, Any]]) -> dict[str, int]:
        counts = {
            "confirmed_accounts": 0,
            "person_clues": 0,
            "device_clues": 0,
            "time_range_clues": 0,
        }
        for item in scopes:
            assertion_level = self._scope_source_assertion_level(_trim(item.get("source")))
            if assertion_level != "confirmed":
                continue
            scope_type = _trim(item.get("scope_type")).lower()
            if scope_type == "account":
                counts["confirmed_accounts"] += 1
            elif scope_type == "person":
                counts["person_clues"] += 1
            elif scope_type == "device":
                counts["device_clues"] += 1
            elif scope_type == "time_range":
                counts["time_range_clues"] += 1
        return counts

    def _build_semantic_scope_targets(
        self,
        scopes: Sequence[dict[str, Any]],
        top_accounts: Sequence[dict[str, Any]],
    ) -> dict[str, Any]:
        items: list[dict[str, Any]] = []
        for scope in scopes:
            scope_type = _trim(scope.get("scope_type")) or "entity"
            scope_value = _trim(scope.get("scope_value"))
            if not scope_value:
                continue
            source = _trim(scope.get("source")) or "unknown"
            display_name = scope_value
            for account in top_accounts:
                if _trim(account.get("account_key")) == scope_value:
                    display_name = f"{scope_value} / {_trim(account.get('display_name')) or scope_value}"
                    break
            items.append(
                {
                    "entity_id": _build_entity_id(scope_type, scope_value),
                    "entity_type": scope_type,
                    "display_name": display_name,
                    "priority": _as_int(scope.get("priority")),
                    "status": _trim(scope.get("status")) or "active",
                    "source": source,
                    "assertion_level": self._scope_source_assertion_level(source),
                }
            )
        confirmed_targets = [item for item in items if item["assertion_level"] == "confirmed"]
        bootstrap_targets = [item for item in items if item["source"] == "auto_txn_bootstrap"]
        candidate_targets = [item for item in items if item["assertion_level"] != "confirmed"]
        return {
            "confirmed_targets": confirmed_targets[:8],
            "candidate_targets": candidate_targets[:8],
            "bootstrap_targets": bootstrap_targets[:8],
            "counts": {
                "confirmed": len(confirmed_targets),
                "candidate": len(candidate_targets),
                "bootstrap": len(bootstrap_targets),
            },
        }

    def _case_profile_payload_from_case_info(self, case_id: str, case_info: Any) -> dict[str, Any]:
        return {
            "case_id": case_id,
            "case_name": _trim(getattr(case_info, "name", "")) or case_id,
            "case_number": _trim(getattr(case_info, "case_no", "")),
            "case_type": _trim(getattr(case_info, "case_type", "")),
            "owner": _trim(getattr(case_info, "owner", "")),
            "tags_json": _json_dumps(getattr(case_info, "tags", []) or []),
            "note": _trim(getattr(case_info, "summary", "")),
            "scope_targets_json": "[]",
            "initial_direction": _trim(getattr(case_info, "summary", "")),
            "status": _normalize_case_status(_trim(getattr(case_info, "status", ""))),
            "created_at": _trim(getattr(case_info, "created_at", "")),
            "updated_at": _trim(getattr(case_info, "updated_at", "")) or _trim(getattr(case_info, "created_at", "")),
        }

    def _bootstrap_scope_rows_from_top_accounts(
        self,
        *,
        case_id: str,
        top_accounts: Sequence[dict[str, Any]],
    ) -> list[dict[str, Any]]:
        rows: list[dict[str, Any]] = []
        for index, item in enumerate(top_accounts[:6], start=1):
            account_key = _trim(item.get("account_key"))
            if not account_key:
                continue
            rows.append(
                {
                    "scope_id": _stable_slug("scope", case_id, account_key),
                    "case_id": case_id,
                    "scope_type": "account",
                    "scope_value": account_key,
                    "priority": index,
                    "status": "active",
                    "source": "auto_txn_bootstrap",
                    "created_at": "",
                }
            )
        return rows

    def _scope_source_assertion_level(self, source: str) -> str:
        normalized = _trim(source).lower()
        if normalized in {"user_confirmed", "manual_confirmed", "confirmed"}:
            return "confirmed"
        if normalized in {"case_record", "case_profile", "user_declared"}:
            return "user_declared"
        if normalized == "auto_txn_bootstrap":
            return "candidate"
        return "candidate"

    def _ensure_bootstrap_scope(self, engine: DuckDBEngine, case_id: str) -> None:
        existing_count = self._count(engine, "analysis_case_scope", case_id)
        if existing_count is None or existing_count > 0:
            return
        top_accounts = self._top_accounts(engine, case_id, limit=6)
        created_at = _now_text()
        for index, item in enumerate(top_accounts, start=1):
            account_key = _trim(item.get("account_key"))
            if not account_key:
                continue
            engine.execute(
                """
                INSERT INTO analysis_case_scope(
                    scope_id, case_id, scope_type, scope_value, priority, status, source, created_at
                ) VALUES (?,?,?,?,?,?,?,?)
                """,
                (
                    _slug("scope"),
                    case_id,
                    "account",
                    account_key,
                    index,
                    "active",
                    "auto_txn_bootstrap",
                    created_at,
                ),
            )

    def _ensure_default_hypothesis(self, engine: DuckDBEngine, case_id: str, profile_payload: dict[str, Any]) -> None:
        existing_count = self._count(engine, "analysis_case_hypothesis", case_id)
        if existing_count is None or existing_count > 0:
            return
        tags = _json_loads(profile_payload.get("tags_json"), [])
        tags_text = "、".join([_trim(item) for item in tags if _trim(item)]) or "待补充标签"
        note = _trim(profile_payload.get("note"))
        content_lines = [
            "## 假设说明",
            f"- 案件类型：{_trim(profile_payload.get('case_type')) or '待补充'}",
            f"- 当前标签：{tags_text}",
        ]
        if note:
            content_lines.append(f"- 初始线索：{note}")
        content_lines.extend(
            [
                "",
                "## 待验证方向",
                "1. 核实候选账户线索是否存在异常汇聚、分散或短时高频转移。",
                "2. 结合后续规则命中、交易切片和图谱结果补齐证据链。",
            ]
        )
        created_at = _now_text()
        hypothesis_id = _slug("hyp")
        engine.execute(
            """
            INSERT INTO analysis_case_hypothesis(
                hypothesis_id, case_id, title, content_md, confidence, status,
                source_query_ids_json, source_evidence_ids_json, created_at, updated_at
            ) VALUES (?,?,?,?,?,?,?,?,?,?)
            """,
            (
                hypothesis_id,
                case_id,
                "初始侦查方向",
                "\n".join(content_lines),
                0.35,
                "open",
                "[]",
                "[]",
                created_at,
                created_at,
            ),
        )
        self._upsert_evidence_ref(
            engine,
            case_id=case_id,
            evidence_type="hypothesis",
            ref_table="analysis_case_hypothesis",
            ref_pk=hypothesis_id,
            title="初始侦查方向",
            snippet="系统基于案件元信息生成的待验证方向。",
            payload={"case_id": case_id, "hypothesis_id": hypothesis_id},
        )

    def sync_case_baseline(self, case_id: str) -> dict[str, Any]:
        case_info = self._storage.get_case(case_id)
        if case_info is None:
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self.workspace_dir(case_id)
            self._ensure_bootstrap_scope(engine, case_id)
            top_accounts = self._top_accounts(engine, case_id, limit=6)
            existing_profile = self._profile_row(engine, case_id)
            payload = {
                "case_id": case_id,
                "case_name": _trim(getattr(case_info, "name", "")) or case_id,
                "case_number": _trim(getattr(case_info, "case_no", "")),
                "case_type": _trim(getattr(case_info, "case_type", "")),
                "owner": _trim(getattr(case_info, "owner", "")),
                "tags_json": _json_dumps(getattr(case_info, "tags", []) or []),
                "note": _trim(getattr(case_info, "summary", "")),
                "scope_targets_json": "[]",
                "initial_direction": _trim(getattr(case_info, "summary", "")) or "围绕候选账户线索和关键时间窗先做结构化排查。",
                "status": _normalize_case_status(_trim(getattr(case_info, "status", ""))),
                "created_at": _trim(getattr(case_info, "created_at", "")) or _now_text(),
                "updated_at": _now_text(),
            }
            self._upsert_profile(engine, payload)
            self._ensure_default_hypothesis(engine, case_id, payload)
            profile_evidence_id = self._upsert_evidence_ref(
                engine,
                case_id=case_id,
                evidence_type="case_profile",
                ref_table="analysis_case_profile",
                ref_pk=case_id,
                title="研判方向",
                snippet=payload["note"][:180],
                payload={
                    "case_id": case_id,
                    "case_name": payload["case_name"],
                    "tags": _json_loads(payload["tags_json"], []),
                },
            )
            if existing_profile is None:
                self._storage.record_case_audit(
                    case_id,
                    "analysis.bootstrap",
                    extra={"workspace_dir": str(self.workspace_dir(case_id)), "profile_evidence_id": profile_evidence_id},
                )
            return {
                "profile": payload,
                "scope_targets": [],
                "top_accounts": top_accounts,
            }
        finally:
            engine.close()

    def _normalize_workspace_file_name(self, file_name: str) -> str:
        normalized = _trim(file_name).replace("\\", "/")
        if not normalized:
            raise ValueError("file_name is empty")
        if normalized.startswith("/") or ".." in normalized.split("/"):
            raise ValueError("file_name is invalid")
        if normalized in self.BASE_WORKSPACE_FILES:
            return normalized
        if normalized.startswith("memory/") and normalized.endswith(".md"):
            return normalized
        if normalized.startswith("reports/") and normalized.endswith(".md"):
            return normalized
        raise ValueError(f"unsupported workspace file: {normalized}")

    def _normalize_ordinary_workspace_file_name(self, file_name: str) -> str:
        normalized = self._normalize_workspace_file_name(file_name)
        if normalized.startswith("reports/"):
            raise RuntimeError("report_publication_receipt_required")
        return normalized

    def validate_ordinary_workspace_files(self, files: Sequence[str]) -> list[str]:
        return [self._normalize_ordinary_workspace_file_name(item) for item in list(files)]

    def _build_workspace_timeline(
        self,
        case_payload: dict[str, Any],
        notes: Sequence[dict[str, Any]],
        imports: Sequence[dict[str, Any]],
        top_accounts: Sequence[dict[str, Any]],
    ) -> list[dict[str, str]]:
        items: list[dict[str, str]] = []
        if _trim(case_payload.get("created_at")):
            items.append({"time": _trim(case_payload.get("created_at")), "label": "案件建档"})
        if imports:
            latest_import = imports[0]
            import_time = _trim(latest_import.get("finished_at") or latest_import.get("created_at"))
            if import_time:
                items.append({"time": import_time, "label": f"最近一次导入：{_trim(latest_import.get('filename')) or '数据导入'}"})
        for account in top_accounts[:3]:
            if _trim(account.get("last_seen_at")):
                items.append(
                    {
                        "time": _trim(account.get("last_seen_at")),
                        "label": f"候选账户活跃：{_trim(account.get('account_key')) or '未命名账户'}",
                    }
                )
        for note in notes[:5]:
            note_time = _trim(note.get("created_at"))
            note_title = _trim(note.get("title")) or "分析纪要"
            if note_time:
                items.append({"time": note_time, "label": f"分析笔记：{note_title}"})
        items.sort(key=lambda item: item.get("time") or "")
        return items[:12]

    @staticmethod
    def _verified_workspace_stat(stats: Any, field_name: str) -> Optional[int]:
        count_status = getattr(stats, "count_status", None)
        if not isinstance(count_status, Mapping) or count_status.get(field_name) != "verified":
            return None
        value = getattr(stats, field_name, None)
        if type(value) is not int or value < 0:
            return None
        return value

    def _render_workspace_file(
        self,
        engine: DuckDBEngine,
        case_id: str,
        file_name: str,
        *,
        render_mode: str = "rendered",
    ) -> dict[str, Any]:
        normalized_file_name = self._normalize_ordinary_workspace_file_name(file_name)
        profile = self._profile_row(engine, case_id) or {}
        scopes = self._scope_rows(engine, case_id)
        hypotheses = self._hypothesis_rows(engine, case_id)
        findings = self._finding_rows(engine, case_id)
        notes = self._note_rows(engine, case_id)
        evidence_rows = self._query_dicts(
            engine,
            "SELECT * FROM analysis_evidence_ref WHERE case_id=? ORDER BY created_at DESC",
            (case_id,),
        )
        query_rows = self._query_log_rows(engine, case_id, limit=20)
        stats = self._storage.get_case_stats(case_id, engine=engine)
        imports = self._query_dicts(
            engine,
            """
            SELECT filename, created_at, finished_at, rows_imported_norm, rows_imported, status
              FROM import_file_log
             WHERE case_id=?
             ORDER BY COALESCE(finished_at, created_at) DESC
             LIMIT 10
            """,
            (case_id,),
        ) if self._table_exists(engine, "import_file_log") else []
        top_accounts = self._top_accounts(engine, case_id, limit=6)
        scope_summary = self._build_scope_summary(scopes)
        semantic_scope_targets = self._build_semantic_scope_targets(scopes, top_accounts)
        timeline_items = self._build_workspace_timeline(profile, notes, imports, top_accounts)
        stats_payload = {
            "transactions": self._verified_workspace_stat(stats, "transactions"),
            "accounts": self._verified_workspace_stat(stats, "accounts"),
            "persons": self._verified_workspace_stat(stats, "persons"),
        }
        if normalized_file_name == "CASE.md":
            content_md = self._artifact_renderer.render_case(
                case_payload=profile,
                tags=_json_loads(profile.get("tags_json"), []),
                scope_summary=scope_summary,
                semantic_scope_targets=semantic_scope_targets,
                hypothesis_count=len(hypotheses),
                finding_count=len(findings),
                stats_payload=stats_payload,
            )
        elif normalized_file_name == "WORKSPACE_MEMORY.md":
            workspace_memory_rows = self._query_dicts(
                engine,
                """
                SELECT *
                  FROM analysis_memory_note
                 WHERE case_id=? AND scope=? AND status='active'
                 ORDER BY updated_at DESC, created_at DESC
                 LIMIT 50
                """,
                (case_id, MEMORY_NOTE_SCOPE_WORKSPACE),
            )
            content_md = self._artifact_renderer.render_workspace_memory(
                [self._memory_note_row_to_payload(row) for row in workspace_memory_rows]
            )
        elif normalized_file_name == "HYPOTHESES.md":
            content_md = self._artifact_renderer.render_hypotheses(hypotheses)
        elif normalized_file_name == "TIMELINE.md":
            content_md = self._artifact_renderer.render_timeline(timeline_items)
        elif normalized_file_name == "EVIDENCE_INDEX.md":
            content_md = self._artifact_renderer.render_evidence_index(evidence_rows, query_rows)
        else:
            day_text = normalized_file_name.split("/", 1)[1].replace(".md", "")
            memory_notes = [
                {
                    **item,
                    "source_query_ids_json": _json_loads(item.get("source_query_ids_json"), []),
                    "source_evidence_ids_json": _json_loads(item.get("source_evidence_ids_json"), []),
                }
                for item in self._note_rows(engine, case_id, day_text=day_text)
            ]
            content_md = self._artifact_renderer.render_memory(day_text, memory_notes)
        return self._upsert_workspace_file(engine, case_id, normalized_file_name, content_md, render_mode=render_mode)

    def render_workspace_files(
        self,
        case_id: str,
        files: Sequence[str],
        *,
        sync_baseline: bool = True,
        render_mode: str = "manual",
    ) -> list[dict[str, Any]]:
        requested = list(files) if files else [*self.BASE_WORKSPACE_FILES, f"memory/{_today_text()}.md"]
        normalized_requested = self.validate_ordinary_workspace_files(requested)
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if sync_baseline:
            self.sync_case_baseline(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            return [
                self._render_workspace_file(engine, case_id, item, render_mode=render_mode)
                for item in normalized_requested
            ]
        finally:
            engine.close()

    def list_workspace_files(self, case_id: str) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            return self._workspace_rows(engine, case_id)
        finally:
            engine.close()

    def list_llm_memory_notes(
        self,
        case_id: str,
        *,
        scope: str = "",
        actor_id: str = "",
        limit: int = 100,
        include_archived: bool = False,
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_scope = _trim(scope).lower()
        normalized_actor_id = _trim(actor_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            params: list[Any] = [case_id]
            sql = """
                SELECT *
                  FROM analysis_memory_note
                 WHERE case_id=?
            """
            if not include_archived:
                sql += " AND status='active'"
            if normalized_scope in MEMORY_NOTE_SCOPES:
                sql += " AND scope=?"
                params.append(normalized_scope)
                if normalized_scope == MEMORY_NOTE_SCOPE_OPERATOR:
                    sql += " AND owner_id=?"
                    params.append(normalized_actor_id or "anonymous")
            elif normalized_actor_id:
                sql += " AND (scope=? OR (scope=? AND owner_id=?))"
                params.extend(
                    [
                        MEMORY_NOTE_SCOPE_WORKSPACE,
                        MEMORY_NOTE_SCOPE_OPERATOR,
                        normalized_actor_id,
                    ]
                )
            else:
                sql += " AND scope=?"
                params.append(MEMORY_NOTE_SCOPE_WORKSPACE)
            sql += " ORDER BY updated_at DESC, created_at DESC LIMIT ?"
            params.append(max(1, min(_as_int(limit or 100), 300)))
            rows = self._query_dicts(engine, sql, params)
            return [self._memory_note_row_to_payload(row) for row in rows]
        finally:
            engine.close()

    def save_llm_memory_note(
        self,
        case_id: str,
        *,
        note: dict[str, Any],
        actor_id: str = "",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized = self._normalize_llm_memory_note(note, actor_id=actor_id)
        now_text = _now_text()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing_rows = self._query_dicts(
                engine,
                "SELECT created_at FROM analysis_memory_note WHERE case_id=? AND note_id=? LIMIT 1",
                (case_id, normalized["note_id"]),
            )
            created_at = _trim(existing_rows[0].get("created_at")) if existing_rows else now_text
            normalized["created_at"] = created_at or now_text
            normalized["updated_at"] = now_text
            engine.execute(
                """
                MERGE INTO analysis_memory_note AS target
                USING (
                    SELECT
                        ? AS note_id,
                        ? AS case_id,
                        ? AS scope,
                        ? AS owner_id,
                    ? AS memory_type,
                    ? AS title,
                    ? AS summary,
                    ? AS detail,
                    ? AS freshness,
                    ? AS trust_level,
                    ? AS required_revalidation,
                    ? AS tags_json,
                    ? AS source,
                    ? AS status,
                    ? AS index_text,
                        ? AS created_at,
                        ? AS updated_at
                ) AS source
                ON target.note_id = source.note_id
                WHEN MATCHED THEN UPDATE SET
                    case_id = source.case_id,
                    scope = source.scope,
                    owner_id = source.owner_id,
                    memory_type = source.memory_type,
                    title = source.title,
                    summary = source.summary,
                    detail = source.detail,
                    freshness = source.freshness,
                    trust_level = source.trust_level,
                    required_revalidation = source.required_revalidation,
                    tags_json = source.tags_json,
                    source = source.source,
                    status = source.status,
                    index_text = source.index_text,
                    created_at = COALESCE(NULLIF(target.created_at, ''), source.created_at),
                    updated_at = source.updated_at
                WHEN NOT MATCHED THEN INSERT (
                    note_id,
                    case_id,
                    scope,
                    owner_id,
                    memory_type,
                    title,
                    summary,
                    detail,
                    freshness,
                    trust_level,
                    required_revalidation,
                    tags_json,
                    source,
                    status,
                    index_text,
                    created_at,
                    updated_at
                ) VALUES (
                    source.note_id,
                    source.case_id,
                    source.scope,
                    source.owner_id,
                    source.memory_type,
                    source.title,
                    source.summary,
                    source.detail,
                    source.freshness,
                    source.trust_level,
                    source.required_revalidation,
                    source.tags_json,
                    source.source,
                    source.status,
                    source.index_text,
                    source.created_at,
                    source.updated_at
                )
                """,
                (
                    normalized["note_id"],
                    case_id,
                    normalized["scope"],
                    normalized["owner_id"],
                    normalized["memory_type"],
                    normalized["title"],
                    normalized["summary"],
                    normalized["detail"],
                    normalized["freshness"],
                    normalized["trust_level"],
                    bool(normalized["required_revalidation"]),
                    _json_dumps(normalized.get("tags") or []),
                    normalized["source"],
                    normalized["status"],
                    normalized["index_text"],
                    normalized["created_at"],
                    normalized["updated_at"],
                ),
            )
            return dict(normalized)
        finally:
            engine.close()

    def _shape_llm_session_row(
        self,
        engine: DuckDBEngine,
        row: dict[str, Any],
        *,
        include_domain_trace: bool = False,
    ) -> dict[str, Any]:
        raw_messages = _json_loads(row.get("messages_json"), [])
        messages: list[dict[str, Any]] = []
        for raw_message in list(raw_messages or []):
            if not isinstance(raw_message, dict):
                continue
            message = dict(raw_message)
            message["skillTrace"] = self._normalize_llm_skill_trace(message.get("skillTrace"))
            raw_versions = message.get("versions")
            if isinstance(raw_versions, list):
                repaired_versions: list[dict[str, Any]] = []
                for raw_version in raw_versions:
                    if not isinstance(raw_version, dict):
                        continue
                    version = dict(raw_version)
                    version["skillTrace"] = self._normalize_llm_skill_trace(version.get("skillTrace"))
                    repaired_versions.append(version)
                message["versions"] = repaired_versions
            messages.append(message)
        document_ids = _json_loads(row.get("document_ids_json"), [])
        subject_ids = _json_loads(row.get("subject_ids_json"), [])
        weak_subject_ids = _json_loads(row.get("weak_subject_ids_json"), [])
        account_keys = _json_loads(row.get("account_keys_json"), [])
        skill_trace = self._normalize_llm_skill_trace(_json_loads(row.get("skill_trace_json"), {}))
        domain_trace = _json_loads(row.get("domain_trace_json"), {})
        stored_binding = self._normalize_session_data_binding(
            dict(skill_trace).get("session_data_binding") if isinstance(skill_trace, dict) else {}
        )
        current_binding = self._build_session_data_binding(
            engine,
            case_id=_trim(row.get("case_id")),
            document_ids=document_ids,
            subject_ids=subject_ids,
            weak_subject_ids=weak_subject_ids,
            account_keys=account_keys,
        )
        data_staleness_state, data_staleness_reason = self._resolve_session_data_staleness(
            stored_binding=stored_binding,
            current_binding=current_binding,
        )
        has_transcript = bool(messages)
        has_context = any([document_ids, subject_ids, weak_subject_ids, account_keys])
        session_kind = "full_session" if has_transcript else ("context_only" if has_context else "memory_only")
        title = _trim(row.get("title")) or "新会话"
        if session_kind == "memory_only" and title in {"自动沉淀会话", "自动沉淀记忆"}:
            title = "自动沉淀记忆"
        shaped = {
            "id": _trim(row.get("session_id")),
            "title": title,
            "createdAt": _trim(row.get("created_at")),
            "updatedAt": _trim(row.get("updated_at")),
            "pinnedAt": _trim(row.get("pinned_at")),
            "documentIds": document_ids,
            "subjectIds": subject_ids,
            "weakSubjectIds": weak_subject_ids,
            "accountKeys": account_keys,
            "lastPrompt": _trim(row.get("last_prompt")),
            "messages": messages,
            "memory": {
                "focus_summary": _trim(row.get("focus_summary")),
                "conclusion_summary": _trim(row.get("conclusion_summary")),
                "next_steps_summary": _trim(row.get("next_steps_summary")),
                "covered_until_turn_id": _trim(row.get("covered_until_turn_id")),
                "covered_message_count": max(0, _as_int(row.get("covered_message_count"))),
                "tags": _json_loads(row.get("tags_json"), []),
            },
            "skillTrace": skill_trace,
            "sessionKind": session_kind,
            "hasTranscript": has_transcript,
            "hasContext": has_context,
            "transcriptMessageCount": len(messages),
            "dataStalenessState": data_staleness_state,
            "dataStalenessReason": data_staleness_reason,
            "boundSourceRevision": _optional_nonnegative_int(stored_binding.get("bound_source_revision")),
            "currentSourceRevision": _optional_nonnegative_int(current_binding.get("bound_source_revision")),
        }
        if include_domain_trace and isinstance(domain_trace, dict) and domain_trace:
            shaped["domainTrace"] = domain_trace
        return shaped

    def list_llm_sessions(
        self,
        case_id: str,
        *,
        keyword: str = "",
        limit: int = 200,
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        def runner(engine: DuckDBEngine) -> list[dict[str, Any]]:
            self.ensure_analysis_schema(engine)
            params: list[Any] = [case_id]
            sql = """
                SELECT *
                  FROM analysis_llm_session
                 WHERE case_id=?
            """
            normalized_keyword = _trim(keyword)
            if normalized_keyword:
                sql += " AND LOWER(COALESCE(index_text, '')) LIKE ?"
                params.append(f"%{normalized_keyword.lower()}%")
            sql += " ORDER BY CASE WHEN NULLIF(TRIM(pinned_at), '') IS NULL THEN 1 ELSE 0 END, pinned_at DESC, updated_at DESC LIMIT ?"
            params.append(max(1, min(_as_int(limit or 200), 500)))
            rows = self._query_dicts(engine, sql, params)
            items: list[dict[str, Any]] = []
            for row in rows:
                items.append(self._shape_llm_session_row(engine, row, include_domain_trace=False))
            return items
        return self._with_case_engine_retry(case_id, runner)

    def get_llm_session(self, case_id: str, session_id: str, *, include_domain_trace: bool = False) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_session_id = _trim(session_id)
        if not normalized_session_id:
            return {}
        def runner(engine: DuckDBEngine) -> dict[str, Any]:
            self.ensure_analysis_schema(engine)
            rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_llm_session WHERE case_id=? AND session_id=? LIMIT 1",
                (case_id, normalized_session_id),
            )
            if not rows:
                return {}
            return self._shape_llm_session_row(engine, rows[0], include_domain_trace=include_domain_trace)
        return self._with_case_engine_retry(case_id, runner)

    def delete_llm_session(self, case_id: str, *, session_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_session_id = _trim(session_id)
        if not normalized_session_id:
            return {
                "session_id": "",
                "deleted": False,
                "deleted_phase_judgment_count": 0,
            }
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            rows = self._query_dicts(
                engine,
                "SELECT session_id FROM analysis_llm_session WHERE case_id=? AND session_id=? LIMIT 1",
                (case_id, normalized_session_id),
            )
            if not rows:
                return {
                    "session_id": normalized_session_id,
                    "deleted": False,
                    "deleted_phase_judgment_count": 0,
                }

            phase_judgment_rows = self._query_dicts(
                engine,
                "SELECT judgment_id FROM analysis_phase_judgment WHERE case_id=? AND session_id=?",
                (case_id, normalized_session_id),
            )
            deleted_phase_judgment_count = len(phase_judgment_rows)
            if deleted_phase_judgment_count:
                engine.execute(
                    "DELETE FROM analysis_phase_judgment WHERE case_id=? AND session_id=?",
                    (case_id, normalized_session_id),
                )

            engine.execute(
                "DELETE FROM analysis_llm_session WHERE case_id=? AND session_id=?",
                (case_id, normalized_session_id),
            )
            return {
                "session_id": normalized_session_id,
                "deleted": True,
                "deleted_phase_judgment_count": deleted_phase_judgment_count,
            }
        finally:
            engine.close()

    def sync_llm_sessions(
        self,
        case_id: str,
        sessions: Sequence[dict[str, Any]],
        *,
        replace_missing: bool = True,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_sessions = [self._normalize_llm_session_payload(item) for item in sessions if isinstance(item, dict)]
        deduped_sessions: list[dict[str, Any]] = []
        seen_session_ids: set[str] = set()
        for item in reversed(normalized_sessions):
            session_id = str(item.get("id") or "").strip()
            if not session_id or session_id in seen_session_ids:
                continue
            seen_session_ids.add(session_id)
            deduped_sessions.append(item)
        normalized_sessions = list(reversed(deduped_sessions))
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing_rows = self._query_dicts(
                engine,
                "SELECT session_id, skill_trace_json, domain_trace_json, covered_until_turn_id, covered_message_count FROM analysis_llm_session WHERE case_id=?",
                (case_id,),
            )
            existing_ids = {_trim(item.get("session_id")) for item in existing_rows if _trim(item.get("session_id"))}
            existing_traces = {
                _trim(item.get("session_id")): _json_loads(item.get("skill_trace_json"), {})
                for item in existing_rows
                if _trim(item.get("session_id"))
            }
            existing_domain_traces = {
                _trim(item.get("session_id")): _json_loads(item.get("domain_trace_json"), {})
                for item in existing_rows
                if _trim(item.get("session_id"))
            }
            existing_memory_coverage = {
                _trim(item.get("session_id")): {
                    "covered_until_turn_id": _trim(item.get("covered_until_turn_id")),
                    "covered_message_count": max(0, _as_int(item.get("covered_message_count"))),
                }
                for item in existing_rows
                if _trim(item.get("session_id"))
            }
            incoming_ids = {str(item.get("id") or "") for item in normalized_sessions if str(item.get("id") or "")}
            if replace_missing:
                for session_id in sorted(existing_ids - incoming_ids):
                    engine.execute("DELETE FROM analysis_llm_session WHERE case_id=? AND session_id=?", (case_id, session_id))
            for item in normalized_sessions:
                memory = item.get("memory") if isinstance(item.get("memory"), dict) else {}
                existing_coverage = existing_memory_coverage.get(str(item.get("id") or "").strip(), {})
                if not _trim(memory.get("covered_until_turn_id")) and _trim(existing_coverage.get("covered_until_turn_id")):
                    memory = {
                        **memory,
                        "covered_until_turn_id": _trim(existing_coverage.get("covered_until_turn_id")),
                    }
                if _as_int(memory.get("covered_message_count")) <= 0 and _as_int(existing_coverage.get("covered_message_count")) > 0:
                    memory = {
                        **memory,
                        "covered_message_count": _as_int(existing_coverage.get("covered_message_count")),
                    }
                session_binding = self._build_session_data_binding(
                    engine,
                    case_id=case_id,
                    document_ids=item.get("documentIds") or [],
                    subject_ids=item.get("subjectIds") or [],
                    weak_subject_ids=item.get("weakSubjectIds") or [],
                    account_keys=item.get("accountKeys") or [],
                )
                skill_trace = self._merge_session_data_binding_into_skill_trace(
                    self._merge_llm_session_skill_trace(
                        existing_traces.get(str(item.get("id") or "").strip()),
                        item.get("skillTrace"),
                    ),
                    binding=session_binding,
                )
                domain_trace = self._merge_llm_session_domain_trace(
                    existing_domain_traces.get(str(item.get("id") or "").strip()),
                    item.get("domainTrace"),
                )
                engine.execute(
                    """
                    INSERT OR REPLACE INTO analysis_llm_session(
                        session_id,
                        case_id,
                        title,
                        pinned_at,
                        last_prompt,
                        document_ids_json,
                        subject_ids_json,
                        weak_subject_ids_json,
                        account_keys_json,
                        messages_json,
                        skill_trace_json,
                        domain_trace_json,
                        focus_summary,
                        conclusion_summary,
                        next_steps_summary,
                        covered_until_turn_id,
                        covered_message_count,
                        tags_json,
                        index_text,
                        created_at,
                        updated_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        item["id"],
                        case_id,
                        item["title"],
                        _trim(item.get("pinnedAt")),
                        item["lastPrompt"],
                        _json_dumps(item.get("documentIds") or []),
                        _json_dumps(item.get("subjectIds") or []),
                        _json_dumps(item.get("weakSubjectIds") or []),
                        _json_dumps(item.get("accountKeys") or []),
                        _json_dumps(item.get("messages") or []),
                        _json_dumps(skill_trace),
                        _json_dumps(domain_trace),
                        _trim(memory.get("focus_summary")),
                        _trim(memory.get("conclusion_summary")),
                        _trim(memory.get("next_steps_summary")),
                        _trim(memory.get("covered_until_turn_id")),
                        max(0, _as_int(memory.get("covered_message_count"))),
                        _json_dumps(memory.get("tags") or []),
                        _trim(item.get("indexText")),
                        item["createdAt"],
                        item["updatedAt"],
                    ),
                )
            return {
                "saved_count": len(normalized_sessions),
                "session_ids": [str(item.get("id") or "") for item in normalized_sessions if str(item.get("id") or "")],
            }
        finally:
            engine.close()

    def save_llm_session_memory(
        self,
        case_id: str,
        *,
        session_id: str,
        memory: dict[str, Any],
        covered_until_turn_id: str = "",
        covered_message_count: int = 0,
        last_prompt: str = "",
        title: str = "",
        base_session: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_session_id = _trim(session_id) or _slug("session")
        normalized_memory = self._normalize_llm_session_memory(memory)
        now_text = _now_text()
        current_session = dict(base_session or {}) if isinstance(base_session, dict) else {}
        if not current_session:
            current_session = self.get_llm_session(case_id, normalized_session_id, include_domain_trace=True)
        existing_memory = dict(current_session.get("memory") or {})
        merged_tags = list(existing_memory.get("tags") or [])
        for tag in list(normalized_memory.get("tags") or []):
            text = _trim(tag)
            if text and text not in merged_tags:
                merged_tags.append(text)
        stored_session = {
            "id": normalized_session_id,
            "title": _trim(title) or _trim(current_session.get("title")) or "自动沉淀记忆",
            "createdAt": _trim(current_session.get("createdAt")) or now_text,
            "updatedAt": now_text,
            "pinnedAt": _trim(current_session.get("pinnedAt")),
            "documentIds": _trim_list(current_session.get("documentIds") or []),
            "subjectIds": _trim_list(current_session.get("subjectIds") or []),
            "weakSubjectIds": _trim_list(current_session.get("weakSubjectIds") or []),
            "accountKeys": _trim_list(current_session.get("accountKeys") or []),
            "lastPrompt": _trim(last_prompt) or _trim(current_session.get("lastPrompt")),
            "messages": self._normalize_llm_messages(current_session.get("messages")),
            "memory": {
                "focus_summary": _trim(normalized_memory.get("focus_summary")) or _trim(existing_memory.get("focus_summary")),
                "conclusion_summary": _trim(normalized_memory.get("conclusion_summary")) or _trim(existing_memory.get("conclusion_summary")),
                "next_steps_summary": _trim(normalized_memory.get("next_steps_summary")) or _trim(existing_memory.get("next_steps_summary")),
                "covered_until_turn_id": (
                    _trim(covered_until_turn_id)
                    or _trim(normalized_memory.get("covered_until_turn_id"))
                    or _trim(existing_memory.get("covered_until_turn_id"))
                ),
                "covered_message_count": max(
                    0,
                    _as_int(covered_message_count)
                    or _as_int(normalized_memory.get("covered_message_count"))
                    or _as_int(existing_memory.get("covered_message_count")),
                ),
                "tags": merged_tags[:8],
            },
            "skillTrace": current_session.get("skillTrace") if isinstance(current_session.get("skillTrace"), dict) else {},
            "domainTrace": current_session.get("domainTrace") if isinstance(current_session.get("domainTrace"), dict) else {},
        }
        stored_session["indexText"] = self._build_llm_session_index_text(
            title=stored_session["title"],
            last_prompt=stored_session["lastPrompt"],
            messages=stored_session["messages"],
            memory=stored_session["memory"],
        )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            stored_session["skillTrace"] = self._merge_session_data_binding_into_skill_trace(
                stored_session.get("skillTrace"),
                binding=self._build_session_data_binding(
                    engine,
                    case_id=case_id,
                    document_ids=stored_session.get("documentIds") or [],
                    subject_ids=stored_session.get("subjectIds") or [],
                    weak_subject_ids=stored_session.get("weakSubjectIds") or [],
                    account_keys=stored_session.get("accountKeys") or [],
                ),
            )
            stored_session["domainTrace"] = self._merge_llm_session_domain_trace(
                stored_session.get("domainTrace"),
                current_session.get("domainTrace"),
            )
            engine.execute(
                """
                MERGE INTO analysis_llm_session AS target
                USING (
                    SELECT
                        ? AS session_id,
                        ? AS case_id,
                        ? AS title,
                        ? AS pinned_at,
                        ? AS last_prompt,
                        ? AS document_ids_json,
                        ? AS subject_ids_json,
                        ? AS weak_subject_ids_json,
                        ? AS account_keys_json,
                        ? AS messages_json,
                        ? AS skill_trace_json,
                        ? AS domain_trace_json,
                        ? AS focus_summary,
                        ? AS conclusion_summary,
                        ? AS next_steps_summary,
                        ? AS covered_until_turn_id,
                        ? AS covered_message_count,
                        ? AS tags_json,
                        ? AS index_text,
                        ? AS created_at,
                        ? AS updated_at
                ) AS source
                ON target.case_id = source.case_id AND target.session_id = source.session_id
                WHEN MATCHED THEN UPDATE SET
                    title = source.title,
                    pinned_at = source.pinned_at,
                    last_prompt = source.last_prompt,
                    document_ids_json = source.document_ids_json,
                    subject_ids_json = source.subject_ids_json,
                    weak_subject_ids_json = source.weak_subject_ids_json,
                    account_keys_json = source.account_keys_json,
                    messages_json = source.messages_json,
                    skill_trace_json = source.skill_trace_json,
                    domain_trace_json = source.domain_trace_json,
                    focus_summary = source.focus_summary,
                    conclusion_summary = source.conclusion_summary,
                    next_steps_summary = source.next_steps_summary,
                    covered_until_turn_id = source.covered_until_turn_id,
                    covered_message_count = source.covered_message_count,
                    tags_json = source.tags_json,
                    index_text = source.index_text,
                    created_at = COALESCE(NULLIF(target.created_at, ''), source.created_at),
                    updated_at = source.updated_at
                WHEN NOT MATCHED THEN INSERT (
                    session_id,
                    case_id,
                    title,
                    pinned_at,
                    last_prompt,
                    document_ids_json,
                    subject_ids_json,
                    weak_subject_ids_json,
                    account_keys_json,
                    messages_json,
                    skill_trace_json,
                    domain_trace_json,
                    focus_summary,
                    conclusion_summary,
                    next_steps_summary,
                    covered_until_turn_id,
                    covered_message_count,
                    tags_json,
                    index_text,
                    created_at,
                    updated_at
                ) VALUES (
                    source.session_id,
                    source.case_id,
                    source.title,
                    source.pinned_at,
                    source.last_prompt,
                    source.document_ids_json,
                    source.subject_ids_json,
                    source.weak_subject_ids_json,
                    source.account_keys_json,
                    source.messages_json,
                    source.skill_trace_json,
                    source.domain_trace_json,
                    source.focus_summary,
                    source.conclusion_summary,
                    source.next_steps_summary,
                    source.covered_until_turn_id,
                    source.covered_message_count,
                    source.tags_json,
                    source.index_text,
                    source.created_at,
                    source.updated_at
                )
                """,
                (
                    stored_session["id"],
                    case_id,
                    stored_session["title"],
                    stored_session["pinnedAt"],
                    stored_session["lastPrompt"],
                    _json_dumps(stored_session["documentIds"]),
                    _json_dumps(stored_session["subjectIds"]),
                    _json_dumps(stored_session["weakSubjectIds"]),
                    _json_dumps(stored_session["accountKeys"]),
                    _json_dumps(stored_session["messages"]),
                    _json_dumps(stored_session["skillTrace"]),
                    _json_dumps(stored_session["domainTrace"]),
                    _trim(stored_session["memory"].get("focus_summary")),
                    _trim(stored_session["memory"].get("conclusion_summary")),
                    _trim(stored_session["memory"].get("next_steps_summary")),
                    _trim(stored_session["memory"].get("covered_until_turn_id")),
                    max(0, _as_int(stored_session["memory"].get("covered_message_count"))),
                    _json_dumps(stored_session["memory"].get("tags") or []),
                    _trim(stored_session["indexText"]),
                    stored_session["createdAt"],
                    stored_session["updatedAt"],
                ),
            )
            return stored_session
        finally:
            engine.close()

    def append_audit_event(self, case_id: str, *, event: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        record = _closed_audit_record(
            normalized_case_id,
            dict(event or {}),
            generate_event_id=True,
        )
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            engine.execute(
                """
                INSERT INTO analysis_audit_event(
                    event_id, case_id, event_type, task_id, run_id, turn_id,
                    artifact_id, approval_id, trace_id, span_id, actor_id, actor_role,
                    tenant_id, request_id, session_id, payload_json, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    record["event_id"],
                    record["case_id"],
                    record["event_type"],
                    record["task_id"],
                    record["run_id"],
                    record["turn_id"],
                    record["artifact_id"],
                    record["approval_id"],
                    record["trace_id"],
                    record["span_id"],
                    record["actor_id"],
                    record["actor_role"],
                    record["tenant_id"],
                    record["request_id"],
                    record["session_id"],
                    _json_dumps(record["payload"]),
                    record["timestamp"],
                ),
            )
            return record
        finally:
            engine.close()

    def append_audit_events(self, case_id: str, *, events: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        normalized_events = [dict(item or {}) for item in list(events or []) if dict(item or {})]
        if not normalized_events:
            return []
        records: list[dict[str, Any]] = []
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            for payload in normalized_events:
                record = _closed_audit_record(
                    normalized_case_id,
                    payload,
                    generate_event_id=True,
                )
                engine.execute(
                    """
                    INSERT INTO analysis_audit_event(
                        event_id, case_id, event_type, task_id, run_id, turn_id,
                        artifact_id, approval_id, trace_id, span_id, actor_id, actor_role,
                        tenant_id, request_id, session_id, payload_json, created_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        record["event_id"],
                        record["case_id"],
                        record["event_type"],
                        record["task_id"],
                        record["run_id"],
                        record["turn_id"],
                        record["artifact_id"],
                        record["approval_id"],
                        record["trace_id"],
                        record["span_id"],
                        record["actor_id"],
                        record["actor_role"],
                        record["tenant_id"],
                        record["request_id"],
                        record["session_id"],
                        _json_dumps(record["payload"]),
                        record["timestamp"],
                    ),
                )
                records.append(record)
            return records
        finally:
            engine.close()

    def append_run_log_event(self, case_id: str, *, event: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        payload = dict(event or {})
        sequence_scope = project_operational_event(
            event=payload,
            run_log=True,
            case_id=normalized_case_id,
        )
        normalized_run_id = sequence_scope["run_id"]
        normalized_turn_id = sequence_scope["turn_id"]
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            sequence_rows = engine.query(
                """
                SELECT COALESCE(MAX(sequence_no), 0)
                  FROM analysis_run_log_event
                 WHERE case_id=? AND run_id=? AND turn_id=?
                """,
                (normalized_case_id, normalized_run_id, normalized_turn_id),
            )
            sequence_no = max(1, _as_int(sequence_rows[0][0] if sequence_rows else 0) + 1)
            record = _closed_run_log_record(
                normalized_case_id,
                payload,
                sequence_no=sequence_no,
                generate_event_id=True,
            )
            engine.execute(
                """
                INSERT INTO analysis_run_log_event(
                    event_id, case_id, task_id, run_id, turn_id, sequence_no,
                    event_stage, event_type, source, payload_json, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    record["event_id"],
                    record["case_id"],
                    record["task_id"],
                    record["run_id"],
                    record["turn_id"],
                    int(record["sequence_no"]),
                    record["event_stage"],
                    record["event_type"],
                    record["source"],
                    _json_dumps(record["payload"]),
                    record["created_at"],
                ),
            )
            return record
        finally:
            engine.close()

    def append_run_log_events(self, case_id: str, *, events: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        normalized_events = [dict(item or {}) for item in list(events or []) if dict(item or {})]
        if not normalized_events:
            return []
        sequence_scope = project_operational_event(
            event=normalized_events[0],
            run_log=True,
            case_id=normalized_case_id,
        )
        run_id = sequence_scope["run_id"]
        turn_id = sequence_scope["turn_id"]
        engine = self.open_case_engine(normalized_case_id)
        records: list[dict[str, Any]] = []
        try:
            self.ensure_analysis_schema(engine)
            sequence_rows = engine.query(
                """
                SELECT COALESCE(MAX(sequence_no), 0)
                  FROM analysis_run_log_event
                 WHERE case_id=? AND run_id=? AND turn_id=?
                """,
                (normalized_case_id, run_id, turn_id),
            )
            next_sequence_no = max(1, _as_int(sequence_rows[0][0] if sequence_rows else 0) + 1)
            for payload in normalized_events:
                record = _closed_run_log_record(
                    normalized_case_id,
                    payload,
                    sequence_no=next_sequence_no,
                    generate_event_id=True,
                )
                next_sequence_no += 1
                engine.execute(
                    """
                    INSERT INTO analysis_run_log_event(
                        event_id, case_id, task_id, run_id, turn_id, sequence_no,
                        event_stage, event_type, source, payload_json, created_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    (
                        record["event_id"],
                        record["case_id"],
                        record["task_id"],
                        record["run_id"],
                        record["turn_id"],
                        int(record["sequence_no"]),
                        record["event_stage"],
                        record["event_type"],
                        record["source"],
                        _json_dumps(record["payload"]),
                        record["created_at"],
                    ),
                )
                records.append(record)
            return records
        finally:
            engine.close()

    def list_audit_events(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
        limit: int = 200,
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        filter_scope = project_operational_event(
            event={"run_id": run_id, "turn_id": turn_id},
            run_log=False,
            case_id=case_id,
        )
        if (_trim(run_id) and not filter_scope["run_id"]) or (
            _trim(turn_id) and not filter_scope["turn_id"]
        ):
            return []
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            params: list[Any] = [_trim(case_id)]
            sql = """
                SELECT *
                  FROM analysis_audit_event
                 WHERE case_id=?
            """
            normalized_run_id = filter_scope["run_id"]
            normalized_turn_id = filter_scope["turn_id"]
            if normalized_run_id:
                sql += " AND run_id=?"
                params.append(normalized_run_id)
            if normalized_turn_id:
                sql += " AND turn_id=?"
                params.append(normalized_turn_id)
            sql += " ORDER BY created_at ASC LIMIT ?"
            params.append(max(1, min(_as_int(limit or 200), 1000)))
            rows = self._query_dicts(engine, sql, params)
            records: list[dict[str, Any]] = []
            for row in rows:
                if _trim(row.get("case_id")) != _trim(case_id):
                    raise RuntimeError("diagnostic_case_binding_mismatch")
                records.append(
                    _closed_audit_record(
                    _trim(case_id),
                    {
                        **row,
                        "timestamp": row.get("created_at"),
                        "payload": _json_loads(row.get("payload_json"), {}),
                    },
                    generate_event_id=False,
                )
                )
            return records
        finally:
            engine.close()

    def list_run_log_events(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
        limit: int = 500,
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        filter_scope = project_operational_event(
            event={"run_id": run_id, "turn_id": turn_id},
            run_log=True,
            case_id=case_id,
        )
        if (_trim(run_id) and not filter_scope["run_id"]) or (
            _trim(turn_id) and not filter_scope["turn_id"]
        ):
            return []
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            params: list[Any] = [_trim(case_id)]
            sql = """
                SELECT *
                  FROM analysis_run_log_event
                 WHERE case_id=?
            """
            normalized_run_id = filter_scope["run_id"]
            normalized_turn_id = filter_scope["turn_id"]
            if normalized_run_id:
                sql += " AND run_id=?"
                params.append(normalized_run_id)
            if normalized_turn_id:
                sql += " AND turn_id=?"
                params.append(normalized_turn_id)
            sql += " ORDER BY sequence_no ASC, created_at ASC LIMIT ?"
            params.append(max(1, min(_as_int(limit or 500), 5000)))
            rows = self._query_dicts(engine, sql, params)
            records: list[dict[str, Any]] = []
            for row in rows:
                if _trim(row.get("case_id")) != _trim(case_id):
                    raise RuntimeError("diagnostic_case_binding_mismatch")
                records.append(
                    _closed_run_log_record(
                    _trim(case_id),
                    {
                        **row,
                        "timestamp": row.get("created_at"),
                        "payload": _json_loads(row.get("payload_json"), {}),
                    },
                    sequence_no=_as_int(row.get("sequence_no")),
                    generate_event_id=False,
                )
                )
            return records
        finally:
            engine.close()

    def ensure_scratchpad_workspace(self, case_id: str, *, workspace: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        payload = dict(workspace or {})
        record = {
            "workspace_id": _trim(payload.get("workspace_id")) or _slug("scratch"),
            "case_id": normalized_case_id,
            "run_id": _trim(payload.get("run_id")),
            "turn_id": _trim(payload.get("turn_id")),
            "owner_type": _trim(payload.get("owner_type")) or "main",
            "owner_id": _trim(payload.get("owner_id")) or "main",
            "workspace_kind": _trim(payload.get("workspace_kind")) or "scratchpad",
            "status": _trim(payload.get("status")) or "active",
            "parent_workspace_id": _trim(payload.get("parent_workspace_id")),
            "inherited_context": dict(payload.get("inherited_context") or {}),
            "retention_state": _trim(payload.get("retention_state")) or "active",
            "created_at": _trim(payload.get("created_at")) or _now_text(),
            "expires_at": _trim(payload.get("expires_at")),
            "updated_at": _trim(payload.get("updated_at")) or _now_text(),
        }
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing = self._query_dicts(
                engine,
                "SELECT workspace_id, created_at FROM analysis_scratchpad_workspace WHERE workspace_id=? LIMIT 1",
                (record["workspace_id"],),
            )
            if existing:
                record["created_at"] = _trim(existing[0].get("created_at")) or record["created_at"]
                engine.execute(
                    """
                    UPDATE analysis_scratchpad_workspace
                       SET case_id=?, run_id=?, turn_id=?, owner_type=?, owner_id=?, workspace_kind=?, status=?,
                           parent_workspace_id=?, inherited_context_json=?, retention_state=?, created_at=?, expires_at=?, updated_at=?
                     WHERE workspace_id=?
                    """,
                    (
                        record["case_id"],
                        record["run_id"],
                        record["turn_id"],
                        record["owner_type"],
                        record["owner_id"],
                        record["workspace_kind"],
                        record["status"],
                        record["parent_workspace_id"],
                        _json_dumps(record["inherited_context"]),
                        record["retention_state"],
                        record["created_at"],
                        record["expires_at"],
                        record["updated_at"],
                        record["workspace_id"],
                    ),
                )
            else:
                engine.execute(
                    """
                    INSERT INTO analysis_scratchpad_workspace(
                        workspace_id, case_id, run_id, turn_id, owner_type, owner_id, workspace_kind, status,
                        parent_workspace_id, inherited_context_json, retention_state, created_at, expires_at, updated_at
                    ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        record["workspace_id"],
                        record["case_id"],
                        record["run_id"],
                        record["turn_id"],
                        record["owner_type"],
                        record["owner_id"],
                        record["workspace_kind"],
                        record["status"],
                        record["parent_workspace_id"],
                        _json_dumps(record["inherited_context"]),
                        record["retention_state"],
                        record["created_at"],
                        record["expires_at"],
                        record["updated_at"],
                    ),
                )
            return {
                **record,
                "inherited_context": dict(record["inherited_context"]),
            }
        finally:
            engine.close()

    def list_scratchpad_workspaces(
        self,
        case_id: str,
        *,
        run_id: str = "",
        turn_id: str = "",
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            sql = """
                SELECT *
                  FROM analysis_scratchpad_workspace
                 WHERE case_id=?
            """
            params: list[Any] = [_trim(case_id)]
            normalized_run_id = _trim(run_id)
            normalized_turn_id = _trim(turn_id)
            if normalized_run_id:
                sql += " AND run_id=?"
                params.append(normalized_run_id)
            if normalized_turn_id:
                sql += " AND turn_id=?"
                params.append(normalized_turn_id)
            sql += " ORDER BY created_at ASC"
            rows = self._query_dicts(engine, sql, params)
            return [
                {
                    "workspace_id": _trim(row.get("workspace_id")),
                    "case_id": _trim(row.get("case_id")),
                    "run_id": _trim(row.get("run_id")),
                    "turn_id": _trim(row.get("turn_id")),
                    "owner_type": _trim(row.get("owner_type")),
                    "owner_id": _trim(row.get("owner_id")),
                    "workspace_kind": _trim(row.get("workspace_kind")),
                    "status": _trim(row.get("status")),
                    "parent_workspace_id": _trim(row.get("parent_workspace_id")),
                    "inherited_context": _json_loads(row.get("inherited_context_json"), {}),
                    "retention_state": _trim(row.get("retention_state")),
                    "created_at": _trim(row.get("created_at")),
                    "expires_at": _trim(row.get("expires_at")),
                    "updated_at": _trim(row.get("updated_at")),
                }
                for row in rows
            ]
        finally:
            engine.close()

    def write_scratchpad_artifact(self, case_id: str, *, artifact: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_case_id = _trim(case_id)
        payload = dict(artifact or {})
        record = {
            "artifact_id": _trim(payload.get("artifact_id")) or _slug("artifact"),
            "case_id": normalized_case_id,
            "workspace_id": _trim(payload.get("workspace_id")),
            "run_id": _trim(payload.get("run_id")),
            "turn_id": _trim(payload.get("turn_id")),
            "artifact_type": _trim(payload.get("artifact_type")) or "scratchpad_note",
            "title": _trim(payload.get("title")) or "未命名 artifact",
            "content_summary": _truncate_text(payload.get("content_summary"), 400),
            "content_md": str(payload.get("content_md") or ""),
            "provenance": dict(payload.get("provenance") or {}),
            "is_formalized": _optional_bool(payload.get("is_formalized")),
            "created_by": _trim(payload.get("created_by")) or "anonymous",
            "retention_state": _trim(payload.get("retention_state")) or "active",
            "created_at": _trim(payload.get("created_at")) or _now_text(),
            "updated_at": _trim(payload.get("updated_at")) or _now_text(),
        }
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing = self._query_dicts(
                engine,
                "SELECT artifact_id, created_at FROM analysis_workspace_artifact WHERE artifact_id=? LIMIT 1",
                (record["artifact_id"],),
            )
            if existing:
                record["created_at"] = _trim(existing[0].get("created_at")) or record["created_at"]
                engine.execute(
                    """
                    UPDATE analysis_workspace_artifact
                       SET case_id=?, workspace_id=?, run_id=?, turn_id=?, artifact_type=?, title=?, content_summary=?,
                           content_md=?, provenance_json=?, is_formalized=?, created_by=?, retention_state=?, created_at=?, updated_at=?
                     WHERE artifact_id=?
                    """,
                    (
                        record["case_id"],
                        record["workspace_id"],
                        record["run_id"],
                        record["turn_id"],
                        record["artifact_type"],
                        record["title"],
                        record["content_summary"],
                        record["content_md"],
                        _json_dumps(record["provenance"]),
                        record["is_formalized"],
                        record["created_by"],
                        record["retention_state"],
                        record["created_at"],
                        record["updated_at"],
                        record["artifact_id"],
                    ),
                )
            else:
                engine.execute(
                    """
                    INSERT INTO analysis_workspace_artifact(
                        artifact_id, case_id, workspace_id, run_id, turn_id, artifact_type, title, content_summary,
                        content_md, provenance_json, is_formalized, created_by, retention_state, created_at, updated_at
                    ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        record["artifact_id"],
                        record["case_id"],
                        record["workspace_id"],
                        record["run_id"],
                        record["turn_id"],
                        record["artifact_type"],
                        record["title"],
                        record["content_summary"],
                        record["content_md"],
                        _json_dumps(record["provenance"]),
                        record["is_formalized"],
                        record["created_by"],
                        record["retention_state"],
                        record["created_at"],
                        record["updated_at"],
                    ),
                )
            return {
                **record,
                "provenance": dict(record["provenance"]),
            }
        finally:
            engine.close()

    def list_scratchpad_artifacts(
        self,
        case_id: str,
        *,
        run_id: str = "",
        workspace_id: str = "",
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            sql = """
                SELECT *
                  FROM analysis_workspace_artifact
                 WHERE case_id=?
            """
            params: list[Any] = [_trim(case_id)]
            normalized_run_id = _trim(run_id)
            normalized_workspace_id = _trim(workspace_id)
            if normalized_run_id:
                sql += " AND run_id=?"
                params.append(normalized_run_id)
            if normalized_workspace_id:
                sql += " AND workspace_id=?"
                params.append(normalized_workspace_id)
            sql += " ORDER BY created_at ASC"
            rows = self._query_dicts(engine, sql, params)
            return [
                {
                    "artifact_id": _trim(row.get("artifact_id")),
                    "case_id": _trim(row.get("case_id")),
                    "workspace_id": _trim(row.get("workspace_id")),
                    "run_id": _trim(row.get("run_id")),
                    "turn_id": _trim(row.get("turn_id")),
                    "artifact_type": _trim(row.get("artifact_type")),
                    "title": _trim(row.get("title")),
                    "content_summary": _trim(row.get("content_summary")),
                    "content_md": str(row.get("content_md") or ""),
                    "provenance": _json_loads(row.get("provenance_json"), {}),
                    "is_formalized": _optional_bool(row.get("is_formalized")),
                    "created_by": _trim(row.get("created_by")),
                    "retention_state": _trim(row.get("retention_state")),
                    "created_at": _trim(row.get("created_at")),
                    "updated_at": _trim(row.get("updated_at")),
                }
                for row in rows
            ]
        finally:
            engine.close()

    def mark_scratchpad_retention(
        self,
        case_id: str,
        *,
        run_id: str,
        status: str,
        retention_state: str,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_run_id = _trim(run_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            updated_at = _now_text()
            engine.execute(
                """
                UPDATE analysis_scratchpad_workspace
                   SET status=?, retention_state=?, updated_at=?
                 WHERE case_id=? AND run_id=?
                """,
                (
                    _trim(status) or "retained",
                    _trim(retention_state) or "retained",
                    updated_at,
                    _trim(case_id),
                    normalized_run_id,
                ),
            )
            engine.execute(
                """
                UPDATE analysis_workspace_artifact
                   SET retention_state=?, updated_at=?
                 WHERE case_id=? AND run_id=?
                """,
                (
                    _trim(retention_state) or "retained",
                    updated_at,
                    _trim(case_id),
                    normalized_run_id,
                ),
            )
            workspace_rows = self._query_dicts(
                engine,
                "SELECT workspace_id, expires_at FROM analysis_scratchpad_workspace WHERE case_id=? AND run_id=? ORDER BY created_at ASC",
                (_trim(case_id), normalized_run_id),
            )
            artifact_rows = self._query_dicts(
                engine,
                "SELECT artifact_id FROM analysis_workspace_artifact WHERE case_id=? AND run_id=? ORDER BY created_at ASC",
                (_trim(case_id), normalized_run_id),
            )
            return {
                "status": _trim(status) or "retained",
                "retention_state": _trim(retention_state) or "retained",
                "workspace_count": len(workspace_rows),
                "artifact_count": len(artifact_rows),
                "workspace_ids": [_trim(item.get("workspace_id")) for item in workspace_rows if _trim(item.get("workspace_id"))],
                "artifact_ids": [_trim(item.get("artifact_id")) for item in artifact_rows if _trim(item.get("artifact_id"))],
                "expires_at": next((item for item in (_trim(row.get("expires_at")) for row in workspace_rows) if item), ""),
                "updated_at": updated_at,
            }
        finally:
            engine.close()

    def build_llm_session_memory_rollups(
        self,
        case_id: str,
        *,
        session_id: str = "",
        limit: int = 4,
    ) -> dict[str, Any]:
        items = self.list_llm_sessions(case_id, limit=max(limit * 3, 12))
        normalized_session_id = _trim(session_id)
        prioritized = sorted(items, key=lambda item: _trim(item.get("updatedAt")), reverse=True)
        prioritized = sorted(prioritized, key=lambda item: 0 if _trim(item.get("pinnedAt")) else 1)
        if normalized_session_id:
            prioritized = sorted(
                prioritized,
                key=lambda item: 0 if _trim(item.get("id")) == normalized_session_id else 1,
            )
        memory_items: list[dict[str, Any]] = []
        for item in prioritized:
            data_staleness_state = _trim(item.get("dataStalenessState") or item.get("data_staleness_state")).lower() or "unknown"
            if data_staleness_state != "fresh":
                continue
            memory = item.get("memory") if isinstance(item.get("memory"), dict) else {}
            focus_summary = _trim(memory.get("focus_summary"))
            conclusion_summary = _trim(memory.get("conclusion_summary"))
            next_steps_summary = _trim(memory.get("next_steps_summary"))
            last_prompt = _trim(item.get("lastPrompt"))
            if not any([focus_summary, conclusion_summary, next_steps_summary, last_prompt]):
                continue
            memory_items.append(
                {
                    "session_id": _trim(item.get("id")),
                    "title": _trim(item.get("title")) or "新会话",
                    "updated_at": _trim(item.get("updatedAt")),
                    "pinned": bool(_trim(item.get("pinnedAt"))),
                    "focus_summary": focus_summary,
                    "conclusion_summary": conclusion_summary,
                    "next_steps_summary": next_steps_summary,
                    "covered_until_turn_id": _trim(memory.get("covered_until_turn_id")),
                    "covered_message_count": max(0, _as_int(memory.get("covered_message_count"))),
                    "last_prompt": last_prompt,
                    "tags": _trim_list(memory.get("tags") or []),
                    "data_staleness_state": data_staleness_state,
                }
            )
            if len(memory_items) >= max(1, min(limit, 8)):
                break
        return {
            "items": memory_items,
            "active_session_id": normalized_session_id,
        }

    def get_latest_llm_phase_judgment(self, case_id: str, *, session_id: str = "") -> dict[str, Any]:
        items = self.list_llm_phase_judgments(case_id, session_id=session_id, limit=1)
        if items:
            return items[0]
        payload = self._default_phase_judgment_payload()
        payload["case_id"] = case_id
        payload["session_id"] = _trim(session_id)
        return payload

    def list_llm_phase_judgments(
        self,
        case_id: str,
        *,
        session_id: str = "",
        limit: int = 6,
    ) -> list[dict[str, Any]]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            normalized_session_id = _trim(session_id)
            params: list[Any] = [case_id]
            sql = "SELECT * FROM analysis_phase_judgment WHERE case_id=?"
            if normalized_session_id:
                sql += " AND session_id=?"
                params.append(normalized_session_id)
            sql += " ORDER BY updated_at DESC, created_at DESC LIMIT ?"
            params.append(max(1, int(limit or 6)))
            rows = self._query_dicts(engine, sql, tuple(params))
            items: list[dict[str, Any]] = []
            for row in rows:
                payload = self._normalize_llm_phase_judgment(
                    {
                        "enabled": bool(row.get("visible") is not None) and bool(
                            _trim(row.get("main_line_summary"))
                            or _trim(row.get("stage_conclusion_summary"))
                            or _trim(row.get("priority_action_summary"))
                        ),
                        "judgment_id": _trim(row.get("judgment_id")),
                        "case_id": _trim(row.get("case_id")) or case_id,
                        "session_id": _trim(row.get("session_id")),
                        "run_id": _trim(row.get("run_id")),
                        "turn_id": _trim(row.get("turn_id")),
                        "main_line_summary": _trim(row.get("main_line_summary")),
                        "stage_conclusion_summary": _trim(row.get("stage_conclusion_summary")),
                        "priority_action_summary": _trim(row.get("priority_action_summary")),
                        "status": _trim(row.get("status")) or "empty",
                        "confidence": _json_loads(row.get("confidence_json"), {}),
                        "tags": _json_loads(row.get("tags_json"), []),
                        "source_query_ids": _json_loads(row.get("source_query_ids_json"), []),
                        "source_evidence_ids": _json_loads(row.get("source_evidence_ids_json"), []),
                        "source_path_ids": _json_loads(row.get("source_path_ids_json"), []),
                        "source_report_ids": _json_loads(row.get("source_report_ids_json"), []),
                        "derived_from": _json_loads(row.get("derived_from_json"), {}),
                        "policy": _json_loads(row.get("policy_json"), {}),
                        "visible": bool(row.get("visible")),
                    }
                )
                payload["enabled"] = bool(
                    payload.get("main_line_summary")
                    or payload.get("stage_conclusion_summary")
                    or payload.get("priority_action_summary")
                    or payload.get("source_query_ids")
                    or payload.get("source_evidence_ids")
                    or payload.get("source_path_ids")
                    or payload.get("source_report_ids")
                )
                payload["created_at"] = _trim(row.get("created_at"))
                payload["updated_at"] = _trim(row.get("updated_at"))
                items.append(payload)
            return items
        finally:
            engine.close()

    def save_llm_phase_judgment(self, case_id: str, judgment: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized = self._normalize_llm_phase_judgment(judgment)
        now_text = _now_text()
        created_at = _trim(judgment.get("created_at")) or now_text
        payload = {
            **normalized,
            "enabled": bool(
                normalized.get("main_line_summary")
                or normalized.get("stage_conclusion_summary")
                or normalized.get("priority_action_summary")
                or normalized.get("source_query_ids")
                or normalized.get("source_evidence_ids")
                or normalized.get("source_path_ids")
                or normalized.get("source_report_ids")
            ),
            "judgment_id": normalized.get("judgment_id") or _slug("phasejudg"),
            "case_id": case_id,
            "created_at": created_at,
            "updated_at": now_text,
        }
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            engine.execute(
                """
                INSERT INTO analysis_phase_judgment(
                    judgment_id,
                    case_id,
                    session_id,
                    run_id,
                    turn_id,
                    main_line_summary,
                    stage_conclusion_summary,
                    priority_action_summary,
                    status,
                    confidence_json,
                    tags_json,
                    source_query_ids_json,
                    source_evidence_ids_json,
                    source_path_ids_json,
                    source_report_ids_json,
                    derived_from_json,
                    policy_json,
                    visible,
                    created_at,
                    updated_at
                ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
                """,
                (
                    payload["judgment_id"],
                    case_id,
                    payload["session_id"],
                    payload["run_id"],
                    payload["turn_id"],
                    payload["main_line_summary"],
                    payload["stage_conclusion_summary"],
                    payload["priority_action_summary"],
                    payload["status"],
                    _json_dumps(payload.get("confidence") or {}),
                    _json_dumps(payload.get("tags") or []),
                    _json_dumps(payload.get("source_query_ids") or []),
                    _json_dumps(payload.get("source_evidence_ids") or []),
                    _json_dumps(payload.get("source_path_ids") or []),
                    _json_dumps(payload.get("source_report_ids") or []),
                    _json_dumps(payload.get("derived_from") or {}),
                    _json_dumps(payload.get("policy") or {}),
                    bool(payload.get("visible")),
                    created_at,
                    now_text,
                ),
            )
            return payload
        finally:
            engine.close()

    def materialize_feature_mart(
        self,
        case_id: str,
        *,
        account_keys: Sequence[str] = (),
        temp_scope_id: str = "",
        source_file_ids: Sequence[str] = (),
        force_refresh: bool = False,
        max_accounts: int = 12,
    ) -> dict[str, Any]:
        del account_keys, temp_scope_id, source_file_ids, force_refresh, max_accounts
        return project_analysis_maintenance_public(
            case_id=case_id,
            operation="feature_mart_materialize",
        )

    def get_case_profile_semantic_view(self, case_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        case_info = self._storage.get_case(case_id)
        if case_info is None:
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            profile = self._profile_row(engine, case_id) or self._case_profile_payload_from_case_info(case_id, case_info)
            scopes = self._scope_rows(engine, case_id)
            hypotheses = self._hypothesis_rows(engine, case_id)
            findings = self._finding_rows(engine, case_id)
            workspace_rows = self._workspace_rows(engine, case_id)
            top_accounts = self._top_accounts(engine, case_id, limit=12)
            effective_scopes = scopes or self._bootstrap_scope_rows_from_top_accounts(
                case_id=case_id,
                top_accounts=top_accounts,
            )
            return {
                "case_id": case_id,
                "case_profile": {
                    "case_name": _trim(profile.get("case_name")),
                    "case_number": _trim(profile.get("case_number")),
                    "case_type": _trim(profile.get("case_type")),
                    "owner": _trim(profile.get("owner")),
                    "tags": _json_loads(profile.get("tags_json"), []),
                    "note": _trim(profile.get("note")),
                },
                "scope_summary": self._build_scope_summary(effective_scopes),
                "scope_targets": self._build_semantic_scope_targets(effective_scopes, top_accounts),
                "progress": {
                    "hypothesis_count": len(hypotheses),
                    "finding_count": len(findings),
                    "workspace_file_count": len(workspace_rows),
                },
                "workspace_projection": self._build_workspace_projection(profile=profile, workspace_rows=workspace_rows),
                "project_docs": self._build_project_docs_projection(case_id),
            }
        finally:
            engine.close()

    def _parse_entity_account(self, entity_id: str) -> str:
        normalized = _trim(entity_id)
        if not normalized:
            return ""
        for prefix in ("acct_", "account:", "account_", "card:", "card_"):
            if normalized.startswith(prefix):
                return normalized[len(prefix):]
        return normalized

    def _parse_entity_person(self, entity_id: str) -> str:
        normalized = _trim(entity_id)
        if not normalized:
            return ""
        for prefix in ("person_", "person:", "pid:", "id:"):
            if normalized.startswith(prefix):
                return normalized[len(prefix):]
        return normalized

    def _counterparty_entity_payload(self, row: dict[str, Any]) -> dict[str, str]:
        account_key = _trim(row.get("counterparty_acct"))
        display_name = _trim(row.get("counterparty_name")) or account_key or _trim(row.get("counterparty_key"))
        if account_key:
            return {
                "entity_id": _build_entity_id("account", account_key),
                "entity_type": "account",
                "entity_key": account_key,
                "display_name": display_name or account_key,
                "bank_name": _trim(row.get("counterparty_bank")),
                "id_no": _trim(row.get("counterparty_id_no")),
            }
        fallback_key = _trim(row.get("counterparty_key"))
        return {
            "entity_id": _build_entity_id("counterparty", fallback_key or display_name or "unknown"),
            "entity_type": "counterparty",
            "entity_key": fallback_key or display_name or "unknown",
            "display_name": display_name or fallback_key or "未知对手",
            "bank_name": _trim(row.get("counterparty_bank")),
            "id_no": _trim(row.get("counterparty_id_no")),
        }

    def _prepare_rule_txn_index_native(self, case_id: str, *, force: bool = False) -> bool:
        result = try_materialize_rule_txn_index(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            force=force,
        )
        if (
            type(result) is not dict
            or set(result) != {
                "ok",
                "case_id",
                "row_count",
                "rebuilt",
                "agg_name",
                "agg_version",
            }
            or result.get("ok") is not True
            or result.get("case_id") != case_id
            or _exact_nonnegative_db_int(result.get("row_count")) is None
            or type(result.get("rebuilt")) is not bool
            or result.get("agg_name") != RULE_TXN_INDEX_CACHE_KEY
            or _exact_nonnegative_db_int(result.get("agg_version")) != RULE_TXN_INDEX_VERSION
        ):
            return False
        return self._verify_rule_txn_index_clean_binding(case_id)

    def _txn_clean_acceptance_predicate(self, columns: set[str], alias: str = "t") -> str:
        if not _TXN_CLEAN_STATE_COLUMNS.issubset(columns):
            return ""
        return " AND ".join(f"{alias}.{column}=0" for column in sorted(_TXN_CLEAN_STATE_COLUMNS))

    def _txn_cleaning_authority_ready(
        self,
        engine: DuckDBEngine,
        case_id: str,
        columns: set[str],
    ) -> bool:
        if not self._txn_clean_acceptance_predicate(columns):
            return False
        try:
            rows = engine.query(
                """
                SELECT COUNT(1)
                  FROM fc_transaction_norm t
                 WHERE t.case_id=?
                   AND (
                        t.clean_invalid IS NULL OR t.clean_invalid NOT IN (0,1)
                     OR t.clean_failed IS NULL OR t.clean_failed NOT IN (0,1)
                     OR t.clean_reversal IS NULL OR t.clean_reversal NOT IN (0,1)
                   )
                """,
                (case_id,),
            )
        except Exception:
            return False
        row = _exact_single_query_row(rows, width=1)
        if row is None:
            return False
        unresolved_count = _exact_nonnegative_db_int(row[0])
        return unresolved_count == 0

    def _rule_txn_index_meta_ready(self, engine: DuckDBEngine, case_id: str) -> bool:
        if (
            not self._table_exists(engine, "analysis_materialization_meta")
            or not self._table_exists(engine, "analysis_rule_txn_idx")
            or not self._table_exists(engine, "fc_transaction_norm")
            or not self._table_exists(engine, "analysis_revision_state")
        ):
            return False
        txn_columns = self._table_columns(engine, "fc_transaction_norm")
        acceptance_predicate = self._txn_clean_acceptance_predicate(txn_columns, alias="t")
        account_key_expr = self._first_text_expr(
            txn_columns,
            "t",
            ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
        )
        if (
            "id" not in txn_columns
            or not acceptance_predicate
            or account_key_expr == "NULL"
            or not self._txn_cleaning_authority_ready(engine, case_id, txn_columns)
        ):
            return False
        try:
            rows = engine.query(
                """
                SELECT agg_version, case_id, source_revision, source_row_count,
                       source_signature, result_signature, row_count
                  FROM analysis_materialization_meta
                 WHERE agg_name=?
                """,
                (RULE_TXN_INDEX_CACHE_KEY,),
            )
        except Exception:
            return False
        row = _exact_single_query_row(rows, width=7)
        if row is None:
            return False
        (
            version,
            bound_case_id,
            source_revision,
            source_row_count,
            source_signature,
            result_signature,
            result_row_count,
        ) = row
        if (
            type(version) is not int
            or version != RULE_TXN_INDEX_VERSION
            or type(bound_case_id) is not str
            or bound_case_id != case_id
            or not _is_lower_sha256_hex(source_signature)
            or not _is_lower_sha256_hex(result_signature)
        ):
            return False
        normalized_source_revision = _exact_nonnegative_db_int(source_revision)
        normalized_source_count = _exact_nonnegative_db_int(source_row_count)
        normalized_result_count = _exact_nonnegative_db_int(result_row_count)
        if not normalized_source_revision or normalized_source_count is None or normalized_result_count is None:
            return False
        try:
            revision_rows = engine.query(
                """
                SELECT COUNT(1), MIN(revision), MAX(revision)
                  FROM analysis_revision_state
                 WHERE revision_key='stats_flow_source'
                """
            )
            identity_rows = engine.query(
                """
                SELECT COUNT(1), COUNT(TRY_CAST(id AS BIGINT)), COUNT(DISTINCT TRY_CAST(id AS BIGINT))
                  FROM fc_transaction_norm
                 WHERE case_id=?
                """,
                (case_id,),
            )
            index_count_rows = engine.query(
                """
                SELECT COUNT(1), COUNT(TRY_CAST(row_id AS BIGINT)),
                       COUNT(DISTINCT TRY_CAST(row_id AS BIGINT))
                  FROM analysis_rule_txn_idx
                 WHERE case_id=?
                """,
                (case_id,),
            )
            accepted_rows = engine.query(
                f"""
                SELECT COUNT(1)
                  FROM fc_transaction_norm AS t
                 WHERE t.case_id=?
                   AND {acceptance_predicate}
                   AND {account_key_expr} IS NOT NULL
                """,
                (case_id,),
            )
            unmatched_rows = engine.query(
                f"""
                SELECT COUNT(1)
                  FROM analysis_rule_txn_idx AS i
                 WHERE i.case_id=?
                   AND NOT EXISTS (
                     SELECT 1
                       FROM fc_transaction_norm AS t
                      WHERE t.case_id=i.case_id
                        AND t.id=TRY_CAST(i.row_id AS BIGINT)
                        AND {acceptance_predicate}
                        AND {account_key_expr} IS NOT NULL
                   )
                """,
                (case_id,),
            )
            if (
                len(revision_rows) != 1
                or len(revision_rows[0]) != 3
                or len(identity_rows) != 1
                or len(identity_rows[0]) != 3
                or len(index_count_rows) != 1
                or len(index_count_rows[0]) != 3
                or len(accepted_rows) != 1
                or len(accepted_rows[0]) != 1
                or len(unmatched_rows) != 1
                or len(unmatched_rows[0]) != 1
            ):
                return False
            revision_count = _exact_nonnegative_db_int(revision_rows[0][0])
            revision_min = _exact_nonnegative_db_int(revision_rows[0][1])
            revision_max = _exact_nonnegative_db_int(revision_rows[0][2])
            source_count = _exact_nonnegative_db_int(identity_rows[0][0])
            cast_id_count = _exact_nonnegative_db_int(identity_rows[0][1])
            distinct_id_count = _exact_nonnegative_db_int(identity_rows[0][2])
            actual_result_count = _exact_nonnegative_db_int(index_count_rows[0][0])
            cast_result_id_count = _exact_nonnegative_db_int(index_count_rows[0][1])
            distinct_result_id_count = _exact_nonnegative_db_int(index_count_rows[0][2])
            accepted_source_count = _exact_nonnegative_db_int(accepted_rows[0][0])
            unmatched_result_count = _exact_nonnegative_db_int(unmatched_rows[0][0])
            if (
                revision_count != 1
                or revision_min != normalized_source_revision
                or revision_max != normalized_source_revision
                or source_count != normalized_source_count
                or cast_id_count != source_count
                or distinct_id_count != source_count
                or actual_result_count != normalized_result_count
                or cast_result_id_count != actual_result_count
                or distinct_result_id_count != actual_result_count
                or accepted_source_count != actual_result_count
                or unmatched_result_count != 0
            ):
                return False
            actual_source_signature = self._canonical_case_table_signature(
                engine,
                table_name="fc_transaction_norm",
                case_id=case_id,
                domain=RULE_TXN_SOURCE_SIGNATURE_DOMAIN,
                order_by="TRY_CAST(r.id AS BIGINT)",
            )
            actual_result_signature = self._canonical_case_table_signature(
                engine,
                table_name="analysis_rule_txn_idx",
                case_id=case_id,
                domain=RULE_TXN_RESULT_SIGNATURE_DOMAIN,
                order_by="TRY_CAST(r.row_id AS BIGINT), r.row_id, r.account_key, r.txn_id",
            )
        except Exception:
            return False
        return source_signature == actual_source_signature and result_signature == actual_result_signature

    def _verify_rule_txn_index_clean_binding(self, case_id: str) -> bool:
        engine = self.open_case_engine(case_id)
        try:
            if not self._table_exists(engine, "analysis_rule_txn_idx") or not self._table_exists(engine, "fc_transaction_norm"):
                return False
            txn_columns = set(self._table_columns(engine, "fc_transaction_norm"))
            acceptance_predicate = self._txn_clean_acceptance_predicate(txn_columns, alias="t")
            if "id" not in txn_columns or not self._txn_cleaning_authority_ready(engine, case_id, txn_columns):
                return False
            if not self._rule_txn_index_meta_ready(engine, case_id):
                return False
            account_key_expr = self._first_text_expr(
                txn_columns,
                "t",
                ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
            )
            if account_key_expr == "NULL":
                return False
            invalid_rows = engine.query(
                f"""
                SELECT COUNT(1)
                  FROM analysis_rule_txn_idx AS i
                 WHERE i.case_id=?
                   AND NOT EXISTS (
                    SELECT 1
                      FROM fc_transaction_norm AS t
                     WHERE t.case_id=i.case_id
                       AND t.id=TRY_CAST(i.row_id AS BIGINT)
                       AND {acceptance_predicate}
                  )
                """,
                (case_id,),
            )
            coverage_rows = engine.query(
                f"""
                SELECT
                  (SELECT COUNT(1)
                     FROM fc_transaction_norm AS t
                    WHERE t.case_id=?
                      AND {acceptance_predicate}
                      AND {account_key_expr} IS NOT NULL),
                  (SELECT COUNT(1)
                     FROM analysis_rule_txn_idx AS i
                    WHERE i.case_id=?)
                """,
                (case_id, case_id),
            )
            invalid_row = _exact_single_query_row(invalid_rows, width=1)
            coverage_row = _exact_single_query_row(coverage_rows, width=2)
            if invalid_row is None or coverage_row is None:
                return False
            invalid_count = _exact_nonnegative_db_int(invalid_row[0])
            accepted_source_count = _exact_nonnegative_db_int(coverage_row[0])
            indexed_result_count = _exact_nonnegative_db_int(coverage_row[1])
            return (
                invalid_count == 0
                and accepted_source_count is not None
                and indexed_result_count is not None
                and accepted_source_count == indexed_result_count
            )
        except Exception:
            return False
        finally:
            engine.close()

    def _native_rule_pattern_param_signature(self, params: Mapping[str, Any]) -> str:
        params = _normalize_rule_params(params)
        payload = {
            "round_amount_unit": _as_float(params.get("round_amount_unit")),
            "round_amount_min_amount": _as_float(params.get("round_amount_min_amount")),
            "round_amount_min_count": _as_int(params.get("round_amount_min_count")),
            "round_amount_min_total_amount": _as_float(params.get("round_amount_min_total_amount")),
            "small_fast_in_out_window_minutes": _as_int(params.get("small_fast_in_out_window_minutes")),
            "small_fast_in_out_ratio": _as_float(params.get("small_fast_in_out_ratio")),
            "small_fast_in_out_min_amount": _as_float(params.get("small_fast_in_out_min_amount")),
            "small_fast_in_out_max_amount": _as_float(params.get("small_fast_in_out_max_amount")),
            "cash_quick_in_out_window_minutes": _as_int(params.get("cash_quick_in_out_window_minutes")),
            "cash_quick_in_out_min_amount": _as_float(params.get("cash_quick_in_out_min_amount")),
            "cash_quick_candidate_window_minutes": _as_int(params.get("cash_quick_candidate_window_minutes")),
            "cash_quick_candidate_min_amount": _as_float(params.get("cash_quick_candidate_min_amount")),
            "cash_quick_candidate_min_out_ratio": _as_float(params.get("cash_quick_candidate_min_out_ratio")),
            "cash_quick_candidate_max_out_ratio": _as_float(params.get("cash_quick_candidate_max_out_ratio")),
            "near_threshold_amount": _as_float(params.get("near_threshold_amount")),
            "near_threshold_lower_rate": _as_float(params.get("near_threshold_lower_rate")),
            "near_threshold_window_minutes": _as_int(params.get("near_threshold_window_minutes")),
            "near_threshold_min_count": _as_int(params.get("near_threshold_min_count")),
            "near_threshold_min_total_amount": _as_float(params.get("near_threshold_min_total_amount")),
            "repeated_amount_window_minutes": _as_int(params.get("repeated_amount_window_minutes")),
            "repeated_amount_min_amount": _as_float(params.get("repeated_amount_min_amount")),
            "repeated_amount_min_count": _as_int(params.get("repeated_amount_min_count")),
            "repeated_amount_min_total_amount": _as_float(params.get("repeated_amount_min_total_amount")),
            "threshold_split_window_minutes": _as_int(params.get("high_freq_window_minutes")),
            "threshold_split_amount": _as_float(params.get("threshold_split_amount")),
            "threshold_split_tolerance_rate": _as_float(params.get("threshold_split_tolerance_rate")),
            "threshold_split_min_count": _as_int(params.get("threshold_split_min_count")),
            "high_freq_small_amount_threshold": _as_float(params.get("small_amount_threshold")),
            "high_freq_window_minutes": _as_int(params.get("high_freq_window_minutes")),
            "high_freq_count_threshold": _as_int(params.get("high_freq_count_threshold")),
            "high_freq_min_total_amount": _as_float(params.get("high_freq_small_min_total_amount")),
            "night_activity_start_hour": _as_int(params.get("night_activity_start_hour")),
            "night_activity_end_hour": _as_int(params.get("night_activity_end_hour")),
            "night_activity_min_count": _as_int(params.get("night_activity_min_count")),
            "night_activity_min_total_amount": _as_float(params.get("night_activity_min_total_amount")),
        }
        return _stable_slug("rule_pattern", _json_dumps(payload))

    def _prepare_rule_pattern_index_native(
        self,
        case_id: str,
        *,
        params: Mapping[str, Any],
        force: bool = False,
    ) -> bool:
        params = _normalize_rule_params(params)
        signature = self._native_rule_pattern_param_signature(params)
        result = try_materialize_rule_pattern_index(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            param_signature=signature,
            round_unit=_as_float(params.get("round_amount_unit")),
            round_min_amount=_as_float(params.get("round_amount_min_amount")),
            round_min_count=_as_int(params.get("round_amount_min_count")),
            round_min_total_amount=_as_float(params.get("round_amount_min_total_amount")),
            small_fast_window_minutes=_as_int(params.get("small_fast_in_out_window_minutes")),
            small_fast_ratio=_as_float(params.get("small_fast_in_out_ratio")),
            small_fast_min_amount=_as_float(params.get("small_fast_in_out_min_amount")),
            small_fast_max_amount=_as_float(params.get("small_fast_in_out_max_amount")),
            cash_quick_window_minutes=_as_int(params.get("cash_quick_in_out_window_minutes")),
            cash_quick_min_amount=_as_float(params.get("cash_quick_in_out_min_amount")),
            cash_candidate_window_minutes=_as_int(params.get("cash_quick_candidate_window_minutes")),
            cash_candidate_min_amount=_as_float(params.get("cash_quick_candidate_min_amount")),
            cash_candidate_min_ratio=_as_float(params.get("cash_quick_candidate_min_out_ratio")),
            cash_candidate_max_ratio=_as_float(params.get("cash_quick_candidate_max_out_ratio")),
            near_threshold_amount=_as_float(params.get("near_threshold_amount")),
            near_threshold_lower_rate=_as_float(params.get("near_threshold_lower_rate")),
            near_threshold_window_minutes=_as_int(params.get("near_threshold_window_minutes")),
            near_threshold_min_count=_as_int(params.get("near_threshold_min_count")),
            near_threshold_min_total_amount=_as_float(params.get("near_threshold_min_total_amount")),
            repeated_amount_window_minutes=_as_int(params.get("repeated_amount_window_minutes")),
            repeated_amount_min_amount=_as_float(params.get("repeated_amount_min_amount")),
            repeated_amount_min_count=_as_int(params.get("repeated_amount_min_count")),
            repeated_amount_min_total_amount=_as_float(params.get("repeated_amount_min_total_amount")),
            threshold_split_window_minutes=_as_int(params.get("high_freq_window_minutes")),
            threshold_split_amount=_as_float(params.get("threshold_split_amount")),
            threshold_split_tolerance_rate=_as_float(params.get("threshold_split_tolerance_rate")),
            threshold_split_min_count=_as_int(params.get("threshold_split_min_count")),
            high_freq_small_amount_threshold=_as_float(params.get("small_amount_threshold")),
            high_freq_window_minutes=_as_int(params.get("high_freq_window_minutes")),
            high_freq_count_threshold=_as_int(params.get("high_freq_count_threshold")),
            high_freq_min_total_amount=_as_float(params.get("high_freq_small_min_total_amount")),
            night_start_hour=_as_int(params.get("night_activity_start_hour")),
            night_end_hour=_as_int(params.get("night_activity_end_hour")),
            night_min_count=_as_int(params.get("night_activity_min_count")),
            night_min_total_amount=_as_float(params.get("night_activity_min_total_amount")),
            force=force,
        )
        if (
            type(result) is not dict
            or set(result) != {
                "ok",
                "case_id",
                "row_count",
                "rebuilt",
                "input_coverage",
                "feature_readiness",
                "agg_name",
                "agg_version",
            }
            or result.get("ok") is not True
            or result.get("case_id") != case_id
            or _exact_nonnegative_db_int(result.get("row_count")) is None
            or type(result.get("rebuilt")) is not bool
            or result.get("agg_name") != RULE_PATTERN_INDEX_CACHE_KEY
            or _exact_nonnegative_db_int(result.get("agg_version")) != RULE_PATTERN_INDEX_VERSION
        ):
            return False
        coverage = result.get("input_coverage")
        expected_coverage_fields = {
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
        if not isinstance(coverage, Mapping) or set(coverage) != expected_coverage_fields:
            return False
        total_rows = _exact_nonnegative_db_int(coverage.get("total_rows"))
        accepted_rows = _exact_nonnegative_db_int(coverage.get("accepted_rows"))
        rejected_rows = _exact_nonnegative_db_int(coverage.get("rejected_rows"))
        if total_rows is None or accepted_rows != total_rows or rejected_rows != 0:
            return False
        if total_rows <= 0 or not all(
            _exact_nonnegative_db_int(coverage.get(field)) == total_rows
            for field in (
                "account_key_covered_rows",
                "transaction_id_covered_rows",
                "transaction_time_covered_rows",
                "amount_covered_rows",
                "direction_covered_rows",
            )
        ):
            return False
        cash_covered_rows = _exact_nonnegative_db_int(coverage.get("cash_covered_rows"))
        cash_unknown_rows = _exact_nonnegative_db_int(coverage.get("cash_unknown_rows"))
        cash_conflict_rows = _exact_nonnegative_db_int(coverage.get("cash_conflict_rows"))
        if (cash_covered_rows, cash_unknown_rows, cash_conflict_rows) != (total_rows, 0, 0):
            return False

        readiness = result.get("feature_readiness")
        if not isinstance(readiness, Mapping) or set(readiness) != {
            "cash_dependent_rules",
            "cash_independent_rules",
        }:
            return False
        cash_dependent = readiness.get("cash_dependent_rules")
        cash_independent = readiness.get("cash_independent_rules")
        if not isinstance(cash_dependent, Mapping) or set(cash_dependent) != {
            "status",
            "requested_rows",
            "eligible_rows",
            "unknown_cash_rows",
            "conflict_cash_rows",
            "blocker",
        }:
            return False
        if not isinstance(cash_independent, Mapping) or set(cash_independent) != {
            "status",
            "requested_rows",
            "eligible_rows",
            "blocker",
        }:
            return False
        return (
            cash_dependent.get("status") == "complete"
            and _exact_nonnegative_db_int(cash_dependent.get("requested_rows")) == total_rows
            and _exact_nonnegative_db_int(cash_dependent.get("eligible_rows")) == total_rows
            and _exact_nonnegative_db_int(cash_dependent.get("unknown_cash_rows")) == 0
            and _exact_nonnegative_db_int(cash_dependent.get("conflict_cash_rows")) == 0
            and cash_dependent.get("blocker") is None
            and cash_independent.get("status") == "complete"
            and _exact_nonnegative_db_int(cash_independent.get("requested_rows")) == total_rows
            and _exact_nonnegative_db_int(cash_independent.get("eligible_rows")) == total_rows
            and cash_independent.get("blocker") is None
        )

    def _load_rule_params_for_native(self, case_id: str) -> dict[str, Any]:
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            return self._load_rule_params(engine, case_id)
        finally:
            engine.close()

    def _load_native_rule_pattern_features(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        params: Mapping[str, Any],
    ) -> Optional[dict[str, Any]]:
        if not self._table_exists(engine, "analysis_rule_pattern_idx"):
            return None
        if not self._table_exists(engine, "analysis_materialization_meta"):
            return None
        if not self._rule_txn_index_meta_ready(engine, case_id):
            return None
        signature = self._native_rule_pattern_param_signature(params)
        try:
            meta_rows = self._query_dicts(
                engine,
                """
                SELECT p.agg_version, p.case_id, p.source_signature, p.source_parameter_signature,
                       p.result_signature, p.row_count,
                       t.agg_version AS source_version, t.case_id AS source_case_id,
                       t.result_signature AS current_source_signature
                  FROM analysis_materialization_meta AS p
                  JOIN analysis_materialization_meta AS t ON t.agg_name=?
                 WHERE p.agg_name=?
                """,
                (RULE_TXN_INDEX_CACHE_KEY, RULE_PATTERN_INDEX_CACHE_KEY),
            )
        except Exception:
            return None
        if type(meta_rows) is not list or len(meta_rows) != 1 or type(meta_rows[0]) is not dict:
            return None
        meta = meta_rows[0]
        result_signature = meta.get("result_signature")
        current_source_signature = meta.get("current_source_signature")
        if (
            _exact_nonnegative_db_int(meta.get("agg_version")) != RULE_PATTERN_INDEX_VERSION
            or meta.get("case_id") != case_id
            or _exact_nonnegative_db_int(meta.get("source_version")) != RULE_TXN_INDEX_VERSION
            or meta.get("source_case_id") != case_id
            or meta.get("source_signature") != current_source_signature
            or meta.get("source_parameter_signature") != signature
            or not _is_lower_sha256_hex(current_source_signature)
            or not _is_lower_sha256_hex(result_signature)
        ):
            return None
        try:
            actual_result_signature = self._canonical_case_table_signature(
                engine,
                table_name="analysis_rule_pattern_idx",
                case_id=case_id,
                domain=RULE_PATTERN_RESULT_SIGNATURE_DOMAIN,
                order_by="r.param_signature, r.feature_code, r.account_key, r.direction, r.first_time, r.last_time",
            )
            count_rows = engine.query(
                "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE case_id=?",
                (case_id,),
            )
            matching_count_rows = engine.query(
                "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE case_id=? AND param_signature=?",
                (case_id, signature),
            )
        except Exception:
            return None
        meta_row_count = _exact_nonnegative_db_int(meta.get("row_count"))
        count_row = _exact_single_query_row(count_rows, width=1)
        matching_count_row = _exact_single_query_row(matching_count_rows, width=1)
        result_row_count = _exact_nonnegative_db_int(count_row[0]) if count_row is not None else None
        matching_row_count = (
            _exact_nonnegative_db_int(matching_count_row[0])
            if matching_count_row is not None
            else None
        )
        if (
            result_signature != actual_result_signature
            or meta_row_count is None
            or result_row_count != meta_row_count
            or matching_row_count != meta_row_count
        ):
            return None
        rows = self._query_dicts(
            engine,
            """
            SELECT *
            FROM analysis_rule_pattern_idx
            WHERE case_id=? AND param_signature=?
            ORDER BY account_key ASC, feature_code ASC, first_time ASC, direction ASC, total_amount DESC
            """,
            (case_id, signature),
        )
        if type(rows) is not list or len(rows) != matching_row_count:
            return None
        by_account: dict[str, dict[str, list[dict[str, Any]]]] = defaultdict(lambda: defaultdict(list))
        for row in rows:
            if type(row) is not dict:
                return None
            account_key_raw = row.get("account_key")
            feature_code_raw = row.get("feature_code")
            if (
                type(account_key_raw) is not str
                or not account_key_raw
                or account_key_raw != account_key_raw.strip()
                or type(feature_code_raw) is not str
                or not feature_code_raw
                or feature_code_raw != feature_code_raw.strip()
                or row.get("case_id") != case_id
                or row.get("param_signature") != signature
            ):
                return None
            account_key = account_key_raw
            feature_code = feature_code_raw
            by_account[account_key][feature_code].append(row)
        return {"ready": True, "param_signature": signature, "by_account": by_account}

    def _load_transaction_rows_from_rule_index(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        file_ids: Optional[Sequence[str]] = None,
    ) -> Optional[list[dict[str, Any]]]:
        if not self._table_exists(engine, "analysis_rule_txn_idx"):
            return None
        normalized_file_ids = _trim_list(file_ids or [])
        where = ["i.case_id=?"]
        params: list[Any] = [case_id]
        if normalized_file_ids:
            placeholders = ",".join(["?"] * len(normalized_file_ids))
            where.append(f"i.file_id IN ({placeholders})")
            params.extend(normalized_file_ids)
        if not self._table_exists(engine, "fc_transaction_norm"):
            return None
        txn_columns = set(self._table_columns(engine, "fc_transaction_norm"))
        if (
            "id" not in txn_columns
            or not self._txn_cleaning_authority_ready(engine, case_id, txn_columns)
            or not self._rule_txn_index_meta_ready(engine, case_id)
        ):
            return None
        acceptance_predicate = self._txn_clean_acceptance_predicate(txn_columns, alias="t")
        where.append(
            "EXISTS ("
            "SELECT 1 FROM fc_transaction_norm AS t "
            "WHERE t.case_id=i.case_id "
            "AND t.id=TRY_CAST(i.row_id AS BIGINT) "
            f"AND {acceptance_predicate}"
            ")"
        )
        rows = self._query_dicts(
            engine,
            f"""
            SELECT
                account_key,
                account_name,
                txn_id,
                row_id,
                txn_time,
                txn_ts_val,
                amount_val,
                balance_val,
                direction_raw,
                success_raw,
                reason_raw,
                opener_id_no,
                ip_addr,
                mac_addr,
                counterparty_acct,
                counterparty_name,
                counterparty_id_no,
                counterparty_bank,
                summary,
                currency,
                branch_name,
                location,
                cash_raw,
                voucher_no,
                receipt_no,
                log_no,
                voucher_type,
                voucher_id,
                teller_no,
                remark,
                txn_type,
                file_id
            FROM analysis_rule_txn_idx AS i
            WHERE {" AND ".join(where)}
            ORDER BY (txn_ts_val IS NULL) ASC, txn_ts_val ASC, COALESCE(txn_id, row_id, account_key) ASC
            """,
            params,
        )
        return self._normalize_transaction_query_rows(case_id, rows)

    def _normalize_transaction_query_rows(
        self,
        case_id: str,
        rows: Sequence[Mapping[str, Any]],
    ) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        for index, row in enumerate(rows, start=1):
            account_key = _trim(row.get("account_key"))
            if not account_key:
                continue
            txn_ts = _coerce_datetime(row.get("txn_ts_val") or row.get("txn_time"))
            txn_time = _datetime_text(row.get("txn_ts_val") or row.get("txn_time"))
            txn_id = _trim(row.get("txn_id"))
            row_id = _trim(row.get("row_id"))
            txn_key = txn_id or (f"row_{row_id}" if row_id else _stable_slug("txnref", case_id, account_key, txn_time, index))
            source_record_id = f"row:{row_id}" if row_id else (f"txn:{txn_id}" if txn_id else "")
            opener_id_no = _trim(row.get("opener_id_no"))
            ip_addr = _trim(row.get("ip_addr"))
            mac_addr = _trim(row.get("mac_addr"))
            ip_key = _normalize_ip(ip_addr)
            mac_key = _normalize_mac(mac_addr)
            device_value = mac_addr or ip_addr
            branch_name = _trim(row.get("branch_name"))
            location = _trim(row.get("location"))
            voucher_no = _trim(row.get("voucher_no"))
            receipt_no = _trim(row.get("receipt_no"))
            log_no = _trim(row.get("log_no"))
            voucher_type = _trim(row.get("voucher_type"))
            teller_no = _trim(row.get("teller_no"))
            branch_key = branch_name or ""
            location_key = location or ""
            voucher_key = ":".join(_unique_texts((voucher_type, voucher_no or receipt_no or log_no)))
            counterparty_key = "|".join(
                _unique_texts(
                    (
                        row.get("counterparty_acct"),
                        row.get("counterparty_name"),
                        row.get("counterparty_bank"),
                    )
                )
            )
            counterparty_payload = self._counterparty_entity_payload(
                {
                    "counterparty_acct": row.get("counterparty_acct"),
                    "counterparty_name": row.get("counterparty_name"),
                    "counterparty_bank": row.get("counterparty_bank"),
                    "counterparty_id_no": row.get("counterparty_id_no"),
                    "counterparty_key": counterparty_key,
                }
            )
            amount_val = _optional_finite_float(row.get("amount_val"))
            balance_val = _optional_finite_float(row.get("balance_val"))
            items.append(
                {
                    "account_key": account_key,
                    "account_name": _trim(row.get("account_name")) or account_key,
                    "account_entity_id": _build_entity_id("account", account_key),
                    "opener_id_no": opener_id_no,
                    "person_entity_id": _build_entity_id("person", opener_id_no) if opener_id_no else "",
                    "txn_id": txn_key,
                    "source_txn_id": txn_id,
                    "source_record_id": source_record_id,
                    "txn_time": txn_time,
                    "txn_ts": txn_ts,
                    "amount_val": round(abs(amount_val), 2) if amount_val is not None else None,
                    "trace_amount_val": round(abs(amount_val), 2) if amount_val is not None else None,
                    "balance_val": round(balance_val, 2) if balance_val is not None else None,
                    "direction_norm": _normalize_direction(row.get("direction_raw")),
                    "success_flag_norm": _normalize_success(row.get("success_raw")),
                    "query_feedback_reason_norm": _normalize_reason(row.get("reason_raw")),
                    "summary": _trim(row.get("summary")),
                    "currency": _trim(row.get("currency")) or None,
                    "ip_addr": ip_addr,
                    "ip_key": ip_key,
                    "mac_addr": mac_addr,
                    "mac_key": mac_key,
                    "device_key": mac_key or ip_key,
                    "device_entity_id": _build_entity_id("device", device_value) if device_value else "",
                    "branch_name": branch_name,
                    "branch_key": branch_key,
                    "branch_entity_id": _build_entity_id("branch", branch_key) if branch_key else "",
                    "location": location,
                    "location_key": location_key,
                    "location_entity_id": _build_entity_id("location", location_key) if location_key else "",
                    "cash_flag_norm": _normalize_bool(row.get("cash_raw")),
                    "voucher_no": voucher_no,
                    "receipt_no": receipt_no,
                    "log_no": log_no,
                    "voucher_type": voucher_type,
                    "voucher_id": _trim(row.get("voucher_id")),
                    "voucher_key": voucher_key,
                    "voucher_entity_id": _build_entity_id("voucher", voucher_key) if voucher_key else "",
                    "teller_no": teller_no,
                    "teller_entity_id": _build_entity_id("teller", teller_no) if teller_no else "",
                    "remark": _trim(row.get("remark")),
                    "txn_type": _trim(row.get("txn_type")),
                    "file_id": _trim(row.get("file_id")),
                    "counterparty_acct": _trim(row.get("counterparty_acct")),
                    "counterparty_name": _trim(row.get("counterparty_name")),
                    "counterparty_id_no": _trim(row.get("counterparty_id_no")),
                    "counterparty_bank": _trim(row.get("counterparty_bank")),
                    "counterparty_key": counterparty_key,
                    "counterparty_entity_id": counterparty_payload["entity_id"],
                    "counterparty_entity_type": counterparty_payload["entity_type"],
                    "counterparty_entity_key": counterparty_payload["entity_key"],
                    "counterparty_display_name": counterparty_payload["display_name"],
                }
            )
        return items

    def _load_transaction_rows(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        file_ids: Optional[Sequence[str]] = None,
        prefer_rule_txn_index: bool = False,
    ) -> list[dict[str, Any]]:
        if prefer_rule_txn_index:
            indexed_rows = self._load_transaction_rows_from_rule_index(engine, case_id, file_ids=file_ids)
            if indexed_rows is not None:
                return indexed_rows
        if not self._table_exists(engine, "fc_transaction_norm"):
            raise TransactionFactSourceUnavailableError()
        columns = self._table_columns(engine, "fc_transaction_norm")
        clean_acceptance_predicate = self._txn_clean_acceptance_predicate(set(columns), alias="t")
        if not self._txn_cleaning_authority_ready(engine, case_id, set(columns)):
            raise TransactionFactSourceUnavailableError()
        account_key_expr = self._first_text_expr(
            columns,
            "t",
            ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
        )
        if account_key_expr == "NULL":
            raise TransactionFactSourceUnavailableError()
        account_name_expr = self._first_text_expr(columns, "t", ("account_open_name",))
        txn_id_expr = self._first_text_expr(columns, "t", ("txn_id",))
        row_id_expr = "CAST(t.id AS VARCHAR)" if "id" in columns else "NULL"
        txn_ts_expr = self._txn_ts_expr(columns, "t")
        txn_time_expr = f"CAST({txn_ts_expr} AS VARCHAR)" if txn_ts_expr != "NULL" else self._first_text_expr(columns, "t", ("txn_time",))
        amount_expr = self._first_num_expr(columns, "t", ("clean_amount", "amount_val", "amount"))
        balance_expr = self._first_num_expr(columns, "t", ("balance_val", "clean_balance", "balance"))
        direction_expr = self._first_text_expr(columns, "t", ("dc_final", "clean_dc_flag", "dc_flag"))
        success_expr = self._first_text_expr(columns, "t", ("success_flag_norm", "is_success", "is_succ"))
        reason_expr = self._first_text_expr(columns, "t", ("query_feedback_reason_norm", "query_feedback_reason"))
        opener_id_expr = self._first_text_expr(columns, "t", ("opener_id_no",))
        ip_expr = self._first_text_expr(columns, "t", ("ip_addr",))
        mac_expr = self._first_text_expr(columns, "t", ("mac_addr",))
        counterparty_acct_expr = self._first_text_expr(columns, "t", ("counterparty_acct_norm", "counterparty_acct"))
        counterparty_name_expr = self._first_text_expr(columns, "t", ("counterparty_name",))
        counterparty_id_expr = self._first_text_expr(columns, "t", ("counterparty_id_no",))
        counterparty_bank_expr = self._first_text_expr(columns, "t", ("counterparty_bank",))
        summary_expr = self._first_text_expr(columns, "t", ("summary",))
        currency_expr = self._first_text_expr(columns, "t", ("currency",))
        branch_expr = self._first_text_expr(columns, "t", ("branch_name",))
        location_expr = self._first_text_expr(columns, "t", ("location", "txn_location"))
        cash_expr = self._first_text_expr(columns, "t", ("cash_flag_norm", "is_cash", "cash_flag"))
        voucher_no_expr = self._first_text_expr(columns, "t", ("voucher_no",))
        receipt_no_expr = self._first_text_expr(columns, "t", ("receipt_no",))
        log_no_expr = self._first_text_expr(columns, "t", ("log_no", "log_id"))
        voucher_type_expr = self._first_text_expr(columns, "t", ("voucher_type",))
        voucher_id_expr = self._first_text_expr(columns, "t", ("voucher_id",))
        teller_no_expr = self._first_text_expr(columns, "t", ("teller_no",))
        remark_expr = self._first_text_expr(columns, "t", ("remark",))
        txn_type_expr = self._first_text_expr(columns, "t", ("txn_type",))
        file_id_expr = self._first_text_expr(columns, "t", ("file_id",))
        order_sql = (
            f"({txn_ts_expr} IS NULL) ASC, {txn_ts_expr} ASC, COALESCE({txn_id_expr}, {row_id_expr}, {account_key_expr}) ASC"
            if txn_ts_expr != "NULL"
            else f"{account_key_expr} ASC, COALESCE({txn_id_expr}, {row_id_expr}) ASC"
        )
        normalized_file_ids = _trim_list(file_ids or [])
        where = [
            "t.case_id=?",
            clean_acceptance_predicate,
            "{account_key_expr} IS NOT NULL".format(account_key_expr=account_key_expr),
        ]
        params: list[Any] = [case_id]
        if normalized_file_ids and file_id_expr != "NULL":
            placeholders = ",".join(["?"] * len(normalized_file_ids))
            where.append(f"{file_id_expr} IN ({placeholders})")
            params.extend(normalized_file_ids)
        rows = self._query_dicts(
            engine,
            f"""
            SELECT
                {account_key_expr} AS account_key,
                COALESCE({account_name_expr}, {account_key_expr}) AS account_name,
                {txn_id_expr} AS txn_id,
                {row_id_expr} AS row_id,
                {txn_time_expr} AS txn_time,
                {txn_ts_expr} AS txn_ts_val,
                ABS({amount_expr}) AS amount_val,
                {balance_expr} AS balance_val,
                {direction_expr} AS direction_raw,
                {success_expr} AS success_raw,
                {reason_expr} AS reason_raw,
                {opener_id_expr} AS opener_id_no,
                {ip_expr} AS ip_addr,
                {mac_expr} AS mac_addr,
                {counterparty_acct_expr} AS counterparty_acct,
                {counterparty_name_expr} AS counterparty_name,
                {counterparty_id_expr} AS counterparty_id_no,
                {counterparty_bank_expr} AS counterparty_bank,
                {summary_expr} AS summary,
                {currency_expr} AS currency,
                {branch_expr} AS branch_name,
                {location_expr} AS location,
                {cash_expr} AS cash_raw,
                {voucher_no_expr} AS voucher_no,
                {receipt_no_expr} AS receipt_no,
                {log_no_expr} AS log_no,
                {voucher_type_expr} AS voucher_type,
                {voucher_id_expr} AS voucher_id,
                {teller_no_expr} AS teller_no,
                {remark_expr} AS remark,
                {txn_type_expr} AS txn_type,
                {file_id_expr} AS file_id
            FROM fc_transaction_norm t
            WHERE {" AND ".join(where)}
            ORDER BY {order_sql}
            """,
            params,
        )
        return self._normalize_transaction_query_rows(case_id, rows)

    def _rebuild_evidence_ref_without_graph_refs(self, engine: DuckDBEngine, case_id: str) -> None:
        tmp_table = f"analysis_evidence_ref_rebuild_{_stable_slug('tmp', case_id)[-12:]}"
        tmp_ident = _quote_identifier(tmp_table)
        columns = (
            "evidence_id",
            "case_id",
            "evidence_type",
            "ref_table",
            "ref_pk",
            "title",
            "snippet",
            "payload_json",
            "created_at",
        )
        column_sql = ", ".join(columns)
        engine.execute(f"DROP TABLE IF EXISTS {tmp_ident}")
        engine.execute(
            f"""
            CREATE TABLE {tmp_ident}(
                evidence_id TEXT PRIMARY KEY,
                case_id TEXT,
                evidence_type TEXT,
                ref_table TEXT,
                ref_pk TEXT,
                title TEXT,
                snippet TEXT,
                payload_json TEXT,
                created_at TEXT
            )
            """
        )
        engine.execute(
            f"""
            INSERT INTO {tmp_ident}({column_sql})
            SELECT {column_sql}
              FROM analysis_evidence_ref
             WHERE NOT (
                case_id=?
                AND ref_table IN ('analysis_entity_node', 'analysis_relation_edge')
             )
            """,
            (case_id,),
        )
        engine.execute("DROP TABLE IF EXISTS analysis_evidence_ref")
        engine.execute(f"ALTER TABLE {tmp_ident} RENAME TO analysis_evidence_ref")
        self._ensure_indexes(engine)

    def _rebuild_evidence_ref_without_rule_hit_refs(self, engine: DuckDBEngine, case_id: str) -> None:
        tmp_table = f"analysis_evidence_ref_rule_rebuild_{_stable_slug('tmp', case_id)[-12:]}"
        tmp_ident = _quote_identifier(tmp_table)
        columns = (
            "evidence_id",
            "case_id",
            "evidence_type",
            "ref_table",
            "ref_pk",
            "title",
            "snippet",
            "payload_json",
            "created_at",
        )
        column_sql = ", ".join(columns)
        engine.execute(f"DROP TABLE IF EXISTS {tmp_ident}")
        engine.execute(
            f"""
            CREATE TABLE {tmp_ident}(
                evidence_id TEXT PRIMARY KEY,
                case_id TEXT,
                evidence_type TEXT,
                ref_table TEXT,
                ref_pk TEXT,
                title TEXT,
                snippet TEXT,
                payload_json TEXT,
                created_at TEXT
            )
            """
        )
        engine.execute(
            f"""
            INSERT INTO {tmp_ident}({column_sql})
            SELECT {column_sql}
              FROM analysis_evidence_ref
             WHERE case_id IS DISTINCT FROM ?
                OR ref_table IS DISTINCT FROM 'analysis_rule_hit'
            """,
            (case_id,),
        )
        engine.execute("DROP TABLE IF EXISTS analysis_evidence_ref")
        engine.execute(f"ALTER TABLE {tmp_ident} RENAME TO analysis_evidence_ref")
        self._ensure_indexes(engine)

    def _rebuild_rule_hit_without_case(self, engine: DuckDBEngine, case_id: str) -> None:
        tmp_table = f"analysis_rule_hit_rebuild_{_stable_slug('tmp', case_id)[-12:]}"
        tmp_ident = _quote_identifier(tmp_table)
        columns = (
            "rule_hit_id",
            "case_id",
            "rule_code",
            "risk_type",
            "severity",
            "score",
            "entity_ids_json",
            "txn_ids_json",
            "evidence_ids_json",
            "summary",
            "detail_json",
            "created_at",
        )
        column_sql = ", ".join(columns)
        engine.execute(f"DROP TABLE IF EXISTS {tmp_ident}")
        engine.execute(
            f"""
            CREATE TABLE {tmp_ident}(
                rule_hit_id TEXT PRIMARY KEY,
                case_id TEXT,
                rule_code TEXT,
                risk_type TEXT,
                severity TEXT,
                score DOUBLE,
                entity_ids_json TEXT,
                txn_ids_json TEXT,
                evidence_ids_json TEXT,
                summary TEXT,
                detail_json TEXT,
                created_at TEXT
            )
            """
        )
        engine.execute(
            f"""
            INSERT INTO {tmp_ident}({column_sql})
            SELECT {column_sql}
              FROM analysis_rule_hit
             WHERE case_id IS DISTINCT FROM ?
            """,
            (case_id,),
        )
        engine.execute("DROP TABLE IF EXISTS analysis_rule_hit")
        engine.execute(f"ALTER TABLE {tmp_ident} RENAME TO analysis_rule_hit")
        self._ensure_indexes(engine)

    def _reset_entity_graph_tables(self, engine: DuckDBEngine, case_id: str) -> None:
        self._rebuild_evidence_ref_without_graph_refs(engine, case_id)
        # Entity graph tables are derived materializations; rebuild them atomically from cleaned transactions.
        engine.execute("DROP TABLE IF EXISTS analysis_relation_edge")
        engine.execute("DROP TABLE IF EXISTS analysis_entity_node")
        self.ensure_analysis_schema(engine)

    def _trace_txn_source_signature(self, txn_rows: Sequence[dict[str, Any]]) -> str:
        if not txn_rows:
            return _stable_slug("trace_sig", "empty")
        amount_entries = []
        for row in txn_rows:
            amount = _trace_amount_value(row)
            amount_entries.append(
                {
                    "txn_id": _trim(row.get("txn_id")),
                    "amount_state": "known" if amount is not None else "unresolved",
                    "amount": amount,
                }
            )
        return _stable_slug("trace_sig", _json_dumps(amount_entries))

    def _materialized_detail_scope(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        account_keys: Optional[Sequence[str]] = None,
        txn_ids: Optional[Sequence[str]] = None,
        file_ids: Optional[Sequence[str]] = None,
    ) -> tuple[str, list[Any]]:
        self._daily_agg.ensure_materialized(case_id, engine=engine)
        if not self._table_exists(engine, "analysis_txn_detail_idx"):
            return "1=0", []
        where = ["1=1"]
        params: list[Any] = []
        normalized_account_keys = _trim_list(account_keys or [])
        normalized_txn_ids = _trim_list(txn_ids or [])
        normalized_file_ids = _trim_list(file_ids or [])
        if normalized_account_keys:
            where.append(f"acct_key IN ({','.join(['?'] * len(normalized_account_keys))})")
            params.extend(normalized_account_keys)
        if normalized_txn_ids:
            where.append(f"txn_id IN ({','.join(['?'] * len(normalized_txn_ids))})")
            params.extend(normalized_txn_ids)
        if normalized_file_ids and "file_id" in self._table_columns(engine, "analysis_txn_detail_idx"):
            where.append(f"file_id IN ({','.join(['?'] * len(normalized_file_ids))})")
            params.extend(normalized_file_ids)
        return " AND ".join(where), params

    def _scope_stats_from_materialized(
        self,
        engine: DuckDBEngine,
        *,
        where_sql: str,
        params: Sequence[Any],
    ) -> dict[str, Any]:
        rows = engine.query(
            f"""
            SELECT
              COUNT(1) AS txn_count,
              COUNT(DISTINCT acct_key) AS account_count,
              COUNT(DISTINCT COALESCE(NULLIF(cp_key, ''), NULLIF(cp_name, ''), NULLIF(cp_raw, ''))) AS counterparty_count,
              MAX(COALESCE(account_open_name, '')) AS account_open_name,
              MIN(CAST(txn_ts AS VARCHAR)) AS first_txn_at,
              MAX(CAST(txn_ts AS VARCHAR)) AS last_txn_at,
              SUM(CASE WHEN dc_val='进' AND amount IS NOT NULL THEN ABS(amount) END) AS in_amount,
              SUM(CASE WHEN dc_val='出' AND amount IS NOT NULL THEN ABS(amount) END) AS out_amount,
              SUM(CASE WHEN dc_val='进' THEN 1 ELSE 0 END) AS in_txn_count,
              SUM(CASE WHEN dc_val='出' THEN 1 ELSE 0 END) AS out_txn_count,
              SUM(CASE WHEN dc_val IN ('进','出') THEN 1 ELSE 0 END) AS directed_txn_count,
              SUM(CASE WHEN dc_val NOT IN ('进','出') OR dc_val IS NULL THEN 1 ELSE 0 END) AS undirected_txn_count,
              AVG(CASE WHEN COALESCE(NULLIF(cp_key, ''), NULLIF(cp_name, ''), NULLIF(cp_raw, '')) IS NOT NULL THEN 1 ELSE 0 END) AS counterparty_rate,
              AVG(CASE WHEN branch_name IS NOT NULL AND TRIM(branch_name) <> '' THEN 1 ELSE 0 END) AS branch_rate,
              AVG(CASE WHEN location IS NOT NULL AND TRIM(location) <> '' THEN 1 ELSE 0 END) AS location_rate,
              AVG(CASE WHEN summary IS NOT NULL AND TRIM(summary) <> '' THEN 1 ELSE 0 END) AS summary_rate,
              AVG(CASE WHEN remark IS NOT NULL AND TRIM(remark) <> '' THEN 1 ELSE 0 END) AS remark_rate,
              AVG(CASE WHEN is_success IS NOT NULL AND TRIM(is_success) <> '' THEN 1 ELSE 0 END) AS success_rate,
              AVG(CASE WHEN ip_addr IS NOT NULL AND TRIM(ip_addr) <> '' THEN 1 ELSE 0 END) AS ip_rate,
              AVG(CASE WHEN mac_addr IS NOT NULL AND TRIM(mac_addr) <> '' THEN 1 ELSE 0 END) AS mac_rate,
              AVG(CASE WHEN cash_flag IS NOT NULL AND TRIM(cash_flag) <> '' THEN 1 ELSE 0 END) AS cash_flag_rate,
              AVG(CASE WHEN file_id IS NOT NULL AND TRIM(CAST(file_id AS VARCHAR)) <> '' THEN 1 ELSE 0 END) AS file_rate,
              SUM(CASE WHEN dc_val='进' AND amount IS NOT NULL THEN 1 ELSE 0 END) AS in_amount_present_count,
              SUM(CASE WHEN dc_val='出' AND amount IS NOT NULL THEN 1 ELSE 0 END) AS out_amount_present_count
            FROM analysis_txn_detail_idx
            WHERE {where_sql}
            """,
            tuple(params),
        )
        row = rows[0] if rows else ()
        txn_count = _optional_nonnegative_int(row[0]) if len(row) > 0 else None
        account_count = _optional_nonnegative_int(row[1]) if len(row) > 1 else None
        counterparty_count = _optional_nonnegative_int(row[2]) if len(row) > 2 else None
        in_txn_count = _optional_nonnegative_int(row[8]) if len(row) > 8 else None
        out_txn_count = _optional_nonnegative_int(row[9]) if len(row) > 9 else None
        directed_txn_count = _optional_nonnegative_int(row[10]) if len(row) > 10 else None
        undirected_txn_count = _optional_nonnegative_int(row[11]) if len(row) > 11 else None
        in_amount_present_count = _optional_nonnegative_int(row[22]) if len(row) > 22 else None
        out_amount_present_count = _optional_nonnegative_int(row[23]) if len(row) > 23 else None
        in_amount = _complete_aggregate_amount(
            row[6] if len(row) > 6 else None,
            expected_count=in_txn_count,
            present_count=in_amount_present_count,
        )
        out_amount = _complete_aggregate_amount(
            row[7] if len(row) > 7 else None,
            expected_count=out_txn_count,
            present_count=out_amount_present_count,
        )
        amount_coverage_complete = (
            directed_txn_count is not None
            and in_amount_present_count is not None
            and out_amount_present_count is not None
            and directed_txn_count == in_amount_present_count + out_amount_present_count
        )
        if txn_count is None:
            coverage_status = "unresolved"
        elif txn_count == 0:
            coverage_status = "empty_unverified"
        elif not amount_coverage_complete or directed_txn_count != txn_count:
            coverage_status = "partial"
        else:
            coverage_status = "complete"
        return {
            "txn_count": txn_count,
            "account_count": account_count,
            "counterparty_count": counterparty_count,
            "account_open_name": _trim(row[3] if len(row) > 3 else ""),
            "first_txn_at": _trim(row[4] if len(row) > 4 else ""),
            "last_txn_at": _trim(row[5] if len(row) > 5 else ""),
            "in_amount": in_amount,
            "out_amount": out_amount,
            "in_txn_count": in_txn_count,
            "out_txn_count": out_txn_count,
            "directed_txn_count": directed_txn_count,
            "undirected_txn_count": undirected_txn_count,
            "in_amount_present_count": in_amount_present_count,
            "out_amount_present_count": out_amount_present_count,
            "turnover_total": _complete_amount_sum((in_amount, out_amount)),
            "field_coverage": {
                "counterparty": _optional_confidence(row[12]) if len(row) > 12 else None,
                "branch_name": _optional_confidence(row[13]) if len(row) > 13 else None,
                "location": _optional_confidence(row[14]) if len(row) > 14 else None,
                "summary": _optional_confidence(row[15]) if len(row) > 15 else None,
                "remark": _optional_confidence(row[16]) if len(row) > 16 else None,
                "success_status": _optional_confidence(row[17]) if len(row) > 17 else None,
                "ip_addr": _optional_confidence(row[18]) if len(row) > 18 else None,
                "mac_addr": _optional_confidence(row[19]) if len(row) > 19 else None,
                "cash_flag": _optional_confidence(row[20]) if len(row) > 20 else None,
                "file_id": _optional_confidence(row[21]) if len(row) > 21 else None,
            },
            "coverage_status": coverage_status,
        }

    def _materialized_filtered_detail_scope(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        account_keys: Optional[Sequence[str]] = None,
        txn_ids: Optional[Sequence[str]] = None,
        file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
    ) -> tuple[str, list[Any]]:
        where_sql, params = self._materialized_detail_scope(
            engine,
            case_id=case_id,
            account_keys=account_keys,
            txn_ids=txn_ids,
            file_ids=file_ids,
        )
        where_parts = [where_sql] if where_sql else ["1=1"]
        normalized_date_start = _trim(date_start)
        normalized_date_end = _trim(date_end)
        normalized_direction = _trim(direction_mode).lower()
        normalized_success = _trim(success_filter).lower()
        normalized_cash = _trim(cash_filter).lower()

        if normalized_date_start:
            start_dt = _coerce_datetime(normalized_date_start)
            if start_dt is not None:
                where_parts.append("txn_ts >= ?")
                params.append(start_dt)
        if normalized_date_end:
            end_dt = _coerce_datetime(normalized_date_end)
            if end_dt is not None:
                where_parts.append("txn_ts <= ?")
                params.append(end_dt)
        if normalized_direction == "in":
            where_parts.append("dc_val='进'")
        elif normalized_direction == "out":
            where_parts.append("dc_val='出'")
        if normalized_success == "success_only":
            where_parts.append(
                "("
                "LOWER(COALESCE(is_success, '')) IN ('1','true','t','y','yes','ok','succ','success') "
                "OR COALESCE(is_success, '') IN ('成功','通过','是') "
                "OR COALESCE(is_success, '') LIKE '%成功%' "
                "OR COALESCE(is_success, '') LIKE '%通过%'"
                ")"
            )
        if normalized_cash == "cash_only":
            where_parts.append(
                "("
                "LOWER(COALESCE(cash_flag, '')) IN ('1','true','t','y','yes','cash') "
                "OR COALESCE(cash_flag, '') IN ('现金','是') "
                "OR COALESCE(cash_flag, '') LIKE '%现金%'"
                ")"
            )
        elif normalized_cash == "non_cash_only":
            where_parts.append(
                "("
                "LOWER(COALESCE(cash_flag, '')) IN ('0','false','f','n','no','non-cash','nocash') "
                "OR COALESCE(cash_flag, '') IN ('非现金','否') "
                "OR COALESCE(cash_flag, '') LIKE '%非现金%'"
                ")"
            )
        return " AND ".join(where_parts), params

    def _resolve_scope_account_keys(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        account_keys: Optional[Sequence[str]] = None,
        txn_ids: Optional[Sequence[str]] = None,
        file_ids: Optional[Sequence[str]] = None,
    ) -> list[str]:
        normalized_account_keys = _trim_list(account_keys or [])
        if normalized_account_keys:
            return normalized_account_keys
        where_sql, params = self._materialized_detail_scope(
            engine,
            case_id=case_id,
            txn_ids=txn_ids,
            file_ids=file_ids,
        )
        rows = engine.query(
            f"""
            SELECT DISTINCT acct_key
            FROM analysis_txn_detail_idx
            WHERE {where_sql}
              AND acct_key IS NOT NULL
              AND TRIM(acct_key) <> ''
            ORDER BY acct_key ASC
            LIMIT 64
            """,
            tuple(params),
        )
        return _trim_list(row[0] for row in rows)

    def _temp_scope_source_refs(self, *, case_id: str, source_ids: Sequence[str]) -> list[str]:
        return _unique_texts(
            opaque_case_bound_ref(prefix="tempsrc_v1", case_id=case_id, value=source_id)
            for source_id in _trim_list(source_ids or [])
        )

    def _resolve_temp_scope_file_ids(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        stored_refs: Any,
    ) -> list[str]:
        stored_values = _trim_list(_json_loads(stored_refs, []) if type(stored_refs) is str else stored_refs or [])
        if not stored_values or not self._table_exists(engine, "import_file_log"):
            return []
        opaque_refs = {value for value in stored_values if re.fullmatch(r"tempsrc_v1_[a-f0-9]{64}", value)}
        legacy_ids = {value for value in stored_values if value not in opaque_refs}
        rows = self._query_dicts(
            engine,
            "SELECT file_id FROM import_file_log WHERE case_id=? ORDER BY file_id ASC",
            (case_id,),
        )
        resolved: list[str] = []
        for row in rows:
            file_id = _trim(row.get("file_id"))
            if not file_id:
                continue
            source_ref = opaque_case_bound_ref(prefix="tempsrc_v1", case_id=case_id, value=file_id)
            if file_id in legacy_ids or source_ref in opaque_refs:
                resolved.append(file_id)
        return _unique_texts(resolved)

    def _resolve_ready_temp_scope_file_ids(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        stored_refs: Any,
    ) -> list[str]:
        stored_values = _trim_list(_json_loads(stored_refs, []) if type(stored_refs) is str else stored_refs or [])
        if (
            not stored_values
            or any(re.fullmatch(r"tempsrc_v1_[a-f0-9]{64}", value) is None for value in stored_values)
            or len(set(stored_values)) != len(stored_values)
        ):
            return []
        candidate_ids = self._resolve_temp_scope_file_ids(
            engine,
            case_id=case_id,
            stored_refs=stored_values,
        )
        rows = self._query_import_file_rows(
            engine,
            case_id=case_id,
            file_ids=candidate_ids,
        )
        ready: list[str] = []
        for file_id in candidate_ids:
            row = dict(rows.get(file_id) or {})
            if self._temp_transaction_import_ready(
                engine,
                case_id=case_id,
                expected_file_id=file_id,
                row=row,
            ):
                ready.append(file_id)
        return _unique_texts(ready)

    def _validated_active_temp_scope_contract(
        self,
        row: Mapping[str, Any],
        *,
        case_id: str,
        scope_id: str,
    ) -> tuple[dict[str, Any], dict[str, Any], list[str], list[str]]:
        signature = _trim(row.get("scope_signature"))
        if (
            _trim(row.get("case_id")) != case_id
            or _trim(row.get("scope_id")) != scope_id
            or not re.fullmatch(r"temp_scope_v1_[a-f0-9]{64}", scope_id)
            or not re.fullmatch(r"temp_scope_v2_[a-f0-9]{16}", signature)
            or scope_id
            != opaque_case_bound_ref(
                prefix="temp_scope_v1",
                case_id=case_id,
                value=signature,
            )
            or _trim(row.get("scope_type")) != "file_scope"
            or _trim(row.get("source_kind")) not in _TEMP_SCOPE_SOURCE_KINDS
            or _trim(row.get("status")).lower() != "active"
        ):
            raise ValueError("temp_scope_invalid")

        source_revision = row.get("source_revision")
        if type(source_revision) is not int or source_revision < 0:
            raise ValueError("temp_scope_invalid")

        source_refs = _json_loads(row.get("source_file_ids_json"), None)
        document_refs = _json_loads(row.get("document_ids_json"), None)
        if (
            not isinstance(source_refs, list)
            or not source_refs
            or any(
                type(value) is not str
                or re.fullmatch(r"tempsrc_v1_[a-f0-9]{64}", value) is None
                for value in source_refs
            )
            or len(set(source_refs)) != len(source_refs)
            or not isinstance(document_refs, list)
            or any(
                type(value) is not str
                or re.fullmatch(r"tempdoc_v1_[a-f0-9]{64}", value) is None
                for value in document_refs
            )
            or len(set(document_refs)) != len(document_refs)
        ):
            raise ValueError("temp_scope_invalid")

        stats = _json_loads(row.get("stats_json"), None)
        audit = _json_loads(row.get("audit_json"), None)
        expected_stats_keys = {
            "contract",
            "requested_source_count",
            "resolved_source_count",
            "failure_count",
            "failures",
            "lifecycle_warning_codes",
            "fact_answer_allowed",
            "raw_details_exposed",
            "lifecycle",
        }
        if (
            not isinstance(stats, dict)
            or set(stats) != expected_stats_keys
            or stats.get("contract") != "TempScopeOperationalStatsV2"
            or stats.get("fact_answer_allowed") is not False
            or stats.get("raw_details_exposed") is not False
        ):
            raise ValueError("temp_scope_invalid")
        requested_count = stats.get("requested_source_count")
        resolved_count = stats.get("resolved_source_count")
        failure_count = stats.get("failure_count")
        if (
            type(requested_count) is not int
            or requested_count < 0
            or type(resolved_count) is not int
            or resolved_count < 0
            or resolved_count != len(source_refs)
            or type(failure_count) is not int
            or failure_count < 0
        ):
            raise ValueError("temp_scope_invalid")
        failures = stats.get("failures")
        if not isinstance(failures, list) or failure_count != len(failures):
            raise ValueError("temp_scope_invalid")
        for failure in failures:
            if (
                not isinstance(failure, dict)
                or set(failure)
                != {"source_ref", "failure_code", "retryable", "fact_answer_allowed"}
                or type(failure.get("source_ref")) is not str
                or re.fullmatch(r"tempsrc_v1_[a-f0-9]{64}", failure["source_ref"])
                is None
                or failure.get("failure_code") not in _TEMP_SCOPE_FAILURE_CODES
                or failure.get("retryable")
                is not (failure.get("failure_code") in _TEMP_SCOPE_RETRYABLE_FAILURE_CODES)
                or failure.get("fact_answer_allowed") is not False
            ):
                raise ValueError("temp_scope_invalid")
        warning_codes = stats.get("lifecycle_warning_codes")
        if (
            not isinstance(warning_codes, list)
            or len(warning_codes) != len(set(warning_codes))
            or any(
                type(code) is not str or code not in _TEMP_SCOPE_WARNING_CODES
                for code in warning_codes
            )
        ):
            raise ValueError("temp_scope_invalid")
        lifecycle = stats.get("lifecycle")
        if (
            not isinstance(lifecycle, dict)
            or set(lifecycle)
            != {"status", "ttl_hours", "expires_at", "retention_until", "active_limit"}
            or lifecycle.get("status") != "active"
            or lifecycle.get("ttl_hours") != TEMP_SCOPE_TTL_HOURS
            or lifecycle.get("active_limit") != TEMP_SCOPE_ACTIVE_LIMIT
            or lifecycle.get("expires_at") != row.get("expires_at")
            or lifecycle.get("retention_until") != row.get("retention_until")
        ):
            raise ValueError("temp_scope_invalid")

        expected_audit_keys = {
            "contract",
            "status",
            "requested_source_count",
            "resolved_source_count",
            "failure_count",
            "cleanup_policy",
            "events",
            "restricted_details_withheld",
        }
        events = audit.get("events") if isinstance(audit, dict) else None
        if (
            not isinstance(audit, dict)
            or set(audit) != expected_audit_keys
            or audit.get("contract") != "TempScopeAuditV2"
            or audit.get("status") != "active"
            or audit.get("requested_source_count") != requested_count
            or audit.get("resolved_source_count") != resolved_count
            or audit.get("failure_count") != failure_count
            or audit.get("cleanup_policy")
            != {
                "ttl_hours": TEMP_SCOPE_TTL_HOURS,
                "audit_retention_days": TEMP_SCOPE_AUDIT_RETENTION_DAYS,
                "active_limit": TEMP_SCOPE_ACTIVE_LIMIT,
            }
            or audit.get("restricted_details_withheld") is not True
            or not isinstance(events, list)
            or len(events) != 1
            or not isinstance(events[0], dict)
            or set(events[0]) != {"action", "at"}
            or events[0].get("action") != "created_or_updated"
            or events[0].get("at") != row.get("updated_at")
        ):
            raise ValueError("temp_scope_invalid")
        if any(
            _coerce_datetime(row.get(column)) is None
            for column in (
                "expires_at",
                "retention_until",
                "last_accessed_at",
                "created_at",
                "updated_at",
            )
        ):
            raise ValueError("temp_scope_invalid")
        return stats, audit, source_refs, document_refs

    def _resolve_active_temp_scope_or_raise(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        temp_scope_id: str,
        caller_source_file_ids: Optional[Sequence[str]] = None,
    ) -> tuple[dict[str, Any], list[str]]:
        normalized_scope_id = _trim(temp_scope_id)
        if not re.fullmatch(r"temp_scope_v1_[a-f0-9]{64}", normalized_scope_id):
            raise ValueError("temp_scope_invalid")
        rows = self._query_dicts(
            engine,
            "SELECT * FROM analysis_temp_scope WHERE case_id=? AND scope_id=? LIMIT 2",
            (case_id, normalized_scope_id),
        )
        if len(rows) != 1:
            raise ValueError("temp_scope_unavailable")
        row = rows[0]
        if _trim(row.get("case_id")) != case_id or _trim(row.get("scope_id")) != normalized_scope_id:
            raise ValueError("temp_scope_unavailable")
        if _trim(row.get("status")).lower() != "active":
            raise ValueError("temp_scope_inactive")
        expires_at = _coerce_datetime(row.get("expires_at"))
        if expires_at is None or expires_at <= datetime.now():
            raise ValueError("temp_scope_expired")
        stats, _, raw_source_refs, _ = self._validated_active_temp_scope_contract(
            row,
            case_id=case_id,
            scope_id=normalized_scope_id,
        )
        if stats.get("resolved_source_count") != len(raw_source_refs):
            raise ValueError("temp_scope_source_unavailable")
        if row.get("source_revision") != self._stats_source_revision(engine):
            raise ValueError("temp_scope_stale")

        resolved_file_ids = self._resolve_ready_temp_scope_file_ids(
            engine,
            case_id=case_id,
            stored_refs=raw_source_refs,
        )
        if not resolved_file_ids or len(resolved_file_ids) != len(raw_source_refs):
            raise ValueError("temp_scope_source_unavailable")
        caller_file_ids = _trim_list(caller_source_file_ids or [])
        normalized_caller_file_ids = _unique_texts(caller_file_ids)
        if len(caller_file_ids) != len(normalized_caller_file_ids):
            raise ValueError("temp_scope_source_mismatch")
        if normalized_caller_file_ids and set(normalized_caller_file_ids) != set(resolved_file_ids):
            raise ValueError("temp_scope_source_mismatch")
        self._touch_temp_scope(engine, case_id=case_id, scope_id=normalized_scope_id)
        return row, resolved_file_ids

    def _temp_scope_lifecycle(self, *, now_text: str) -> tuple[str, str]:
        base_dt = _coerce_datetime(now_text) or datetime.now()
        expires_at = (base_dt + timedelta(hours=TEMP_SCOPE_TTL_HOURS)).strftime("%Y-%m-%d %H:%M:%S")
        retention_until = (base_dt + timedelta(days=TEMP_SCOPE_AUDIT_RETENTION_DAYS)).strftime("%Y-%m-%d %H:%M:%S")
        return expires_at, retention_until

    def _touch_temp_scope(self, engine: DuckDBEngine, *, case_id: str, scope_id: str) -> None:
        if not _trim(scope_id):
            return
        now_text = _now_text()
        engine.execute(
            """
            UPDATE analysis_temp_scope
               SET last_accessed_at=?
             WHERE case_id=? AND scope_id=? AND COALESCE(status, 'active')='active'
            """,
            (now_text, case_id, _trim(scope_id)),
        )

    def _purge_temp_file_ids(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        file_ids: Sequence[str],
    ) -> None:
        normalized_file_ids = [item for item in _trim_list(file_ids) if item.startswith("temp_fc_")]
        if not normalized_file_ids:
            return
        placeholders = ",".join(["?"] * len(normalized_file_ids))
        for table_name in ("fc_transaction_raw", "fc_transaction_norm", "fc_account", "cleaning_log", "import_file_log"):
            if not self._table_exists(engine, table_name):
                continue
            columns = self._table_columns(engine, table_name)
            if "file_id" not in columns:
                continue
            if "case_id" in columns:
                engine.execute(
                    f"DELETE FROM {table_name} WHERE case_id=? AND file_id IN ({placeholders})",
                    (case_id, *normalized_file_ids),
                )
            else:
                engine.execute(
                    f"DELETE FROM {table_name} WHERE file_id IN ({placeholders})",
                    tuple(normalized_file_ids),
                )

    def _invalidate_temp_scope_materializations(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
    ) -> None:
        for table_name in (
            "analysis_txn_daily_agg",
            "analysis_txn_detail_idx",
            "analysis_txn_keyword_idx",
            "analysis_account_dim",
        ):
            if not self._table_exists(engine, table_name):
                continue
            columns = self._table_columns(engine, table_name)
            if "case_id" in columns:
                engine.execute(f"DELETE FROM {table_name} WHERE case_id=?", (case_id,))
            else:
                # Every active database is admitted for exactly one case. A
                # legacy materialization without case_id must therefore be
                # emptied rather than risk serving stale temp facts.
                engine.execute(f"DELETE FROM {table_name}")
        if self._table_exists(engine, "analysis_materialization_meta"):
            engine.execute("DELETE FROM analysis_materialization_meta")

    def _expire_temp_scope_rows(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        scope_rows: Sequence[dict[str, Any]],
        reason: str,
    ) -> list[dict[str, Any]]:
        expired_items: list[dict[str, Any]] = []
        if not scope_rows:
            return expired_items
        now_text = _now_text()
        cleanup_reason = reason if reason in {"ttl_expired", "quota_trimmed"} else "lifecycle_cleanup"
        expiration_plans: list[dict[str, Any]] = []
        for row in scope_rows:
            scope_id = _trim(row.get("scope_id"))
            if not scope_id:
                raise ValueError("temp_scope_invalid")
            stats, _, _, _ = self._validated_active_temp_scope_contract(
                row,
                case_id=case_id,
                scope_id=scope_id,
            )
            source_file_ids = self._resolve_temp_scope_file_ids(
                engine,
                case_id=case_id,
                stored_refs=row.get("source_file_ids_json"),
            )
            synthetic_file_ids = [item for item in source_file_ids if item.startswith("temp_fc_")]
            expired_stats = dict(stats)
            expired_lifecycle = dict(stats["lifecycle"])
            expired_lifecycle["status"] = "expired"
            expired_stats["lifecycle"] = expired_lifecycle
            audit_payload = {
                "contract": "TempScopeAuditV2",
                "status": "expired",
                "cleanup_reason": cleanup_reason,
                "cleanup_at": now_text,
                "events": [{"action": "expired", "reason": cleanup_reason, "at": now_text}],
                "restricted_details_withheld": True,
            }
            expiration_plans.append(
                {
                    "scope_id": scope_id,
                    "scope_signature": _trim(row.get("scope_signature")),
                    "synthetic_file_ids": synthetic_file_ids,
                    "stats_json": _json_dumps(expired_stats),
                    "audit_json": _json_dumps(audit_payload),
                }
            )

        need_rebuild = any(plan["synthetic_file_ids"] for plan in expiration_plans)
        transaction_started = False
        try:
            engine.execute("BEGIN TRANSACTION")
            transaction_started = True
            for plan in expiration_plans:
                synthetic_file_ids = plan["synthetic_file_ids"]
                if synthetic_file_ids:
                    self._purge_temp_file_ids(
                        engine,
                        case_id=case_id,
                        file_ids=synthetic_file_ids,
                    )
                engine.execute(
                    """
                    UPDATE analysis_temp_scope
                       SET status='expired', stats_json=?, audit_json=?, updated_at=?,
                           last_accessed_at=COALESCE(last_accessed_at, ?)
                     WHERE case_id=? AND scope_id=? AND status='active'
                    """,
                    (
                        plan["stats_json"],
                        plan["audit_json"],
                        now_text,
                        now_text,
                        case_id,
                        plan["scope_id"],
                    ),
                )
            if need_rebuild:
                bump_stats_flow_source_revision(engine, reason="source_revision_changed")
                self._invalidate_temp_scope_materializations(engine, case_id=case_id)
            engine.execute("COMMIT")
            transaction_started = False
        except Exception:
            if transaction_started:
                try:
                    engine.execute("ROLLBACK")
                except Exception:
                    pass
            raise

        expired_items.extend(
            {
                "scope_id": plan["scope_id"],
                "scope_signature": plan["scope_signature"],
                "synthetic_file_ids": list(plan["synthetic_file_ids"]),
            }
            for plan in expiration_plans
        )
        if need_rebuild:
            if not self._daily_agg.ensure_materialized(case_id, engine=engine, force=True):
                raise RuntimeError("temp_scope_materialization_rebuild_failed")
        return expired_items

    def _cleanup_temp_scope_lifecycle(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        keep_scope_id: str = "",
    ) -> list[dict[str, Any]]:
        lifecycle_warnings: list[dict[str, Any]] = []
        now_text = _now_text()
        active_expired_rows = self._query_dicts(
            engine,
            """
            SELECT *
            FROM analysis_temp_scope
            WHERE case_id=? AND COALESCE(status, 'active')='active' AND COALESCE(expires_at, '')<>'' AND expires_at<=?
            """,
            (case_id, now_text),
        )
        expired_items = self._expire_temp_scope_rows(
            engine,
            case_id=case_id,
            scope_rows=active_expired_rows,
            reason="ttl_expired",
        )
        if expired_items:
            lifecycle_warnings.append(
                {
                    "code": "TEMP_SCOPE_TTL_EXPIRED",
                    "message": f"已回收 {len(expired_items)} 个超时 temp_scope，并清理其 synthetic temp file_id。",
                    "severity": "info",
                }
            )
        active_rows = self._query_dicts(
            engine,
            """
            SELECT *
            FROM analysis_temp_scope
            WHERE case_id=? AND COALESCE(status, 'active')='active'
            ORDER BY COALESCE(last_accessed_at, updated_at, created_at) ASC, scope_id ASC
            """,
            (case_id,),
        )
        keep_scope = _trim(keep_scope_id)
        overflow_rows = []
        if len(active_rows) > TEMP_SCOPE_ACTIVE_LIMIT:
            for row in active_rows:
                if keep_scope and _trim(row.get("scope_id")) == keep_scope:
                    continue
                overflow_rows.append(row)
                if len(active_rows) - len(overflow_rows) <= TEMP_SCOPE_ACTIVE_LIMIT:
                    break
        quota_expired = self._expire_temp_scope_rows(
            engine,
            case_id=case_id,
            scope_rows=overflow_rows,
            reason="quota_trimmed",
        )
        if quota_expired:
            lifecycle_warnings.append(
                {
                    "code": "TEMP_SCOPE_QUOTA_TRIMMED",
                    "message": f"为控制活跃 temp_scope 配额，已回收 {len(quota_expired)} 个较旧范围。",
                    "severity": "warning",
                }
            )
        hard_delete_rows = self._query_dicts(
            engine,
            """
            SELECT scope_id, scope_signature
            FROM analysis_temp_scope
            WHERE case_id=? AND COALESCE(status, 'active')='expired' AND COALESCE(retention_until, '')<>'' AND retention_until<=?
            """,
            (case_id, now_text),
        )
        if hard_delete_rows:
            scope_ids = [_trim(row.get("scope_id")) for row in hard_delete_rows if _trim(row.get("scope_id"))]
            if scope_ids:
                placeholders = ",".join(["?"] * len(scope_ids))
                engine.execute(
                    f"DELETE FROM analysis_temp_scope WHERE case_id=? AND scope_id IN ({placeholders})",
                    (case_id, *scope_ids),
                )
            lifecycle_warnings.append(
                {
                    "code": "TEMP_SCOPE_AUDIT_PURGED",
                    "message": f"已清理 {len(hard_delete_rows)} 个超过审计保留期的 temp_scope 记录。",
                    "severity": "info",
                }
            )
        return lifecycle_warnings

    def _cleanup_workspace_retention(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
    ) -> dict[str, Any]:
        summary = WorkspaceJanitorSummary()
        now_text = _now_text()
        now_dt = _coerce_datetime(now_text) or datetime.now()
        delete_grace = timedelta(hours=max(1, int(DEFAULT_RUNTIME_RETENTION_POLICY.scratchpad_delete_grace_hours or 72)))
        workspace_rows = self._query_dicts(
            engine,
            """
            SELECT *
            FROM analysis_scratchpad_workspace
            WHERE case_id=?
            ORDER BY created_at ASC, workspace_id ASC
            """,
            (case_id,),
        )
        summary.scanned_count = len(workspace_rows)
        for row in workspace_rows:
            workspace_id = _trim(row.get("workspace_id"))
            if not workspace_id:
                summary.add_skip("missing_workspace_id")
                continue
            retention_state = _trim(row.get("retention_state")) or "active"
            status = _trim(row.get("status")) or "active"
            expires_at = _coerce_datetime(row.get("expires_at"))
            if retention_state == "retained" or status == "retained":
                summary.add_skip("retained_workspace")
                continue
            if expires_at is None or expires_at > now_dt:
                summary.add_skip("not_expired")
                continue
            summary.stale_count += 1
            if workspace_id not in summary.expired_workspace_ids:
                summary.expired_workspace_ids.append(workspace_id)
            if status != "expired" or retention_state != "expired":
                engine.execute(
                    """
                    UPDATE analysis_scratchpad_workspace
                       SET status='expired', retention_state='expired', updated_at=?
                     WHERE case_id=? AND workspace_id=?
                    """,
                    (now_text, case_id, workspace_id),
                )
            artifact_rows = self._query_dicts(
                engine,
                """
                SELECT artifact_id, retention_state, is_formalized
                FROM analysis_workspace_artifact
                WHERE case_id=? AND workspace_id=?
                ORDER BY created_at ASC, artifact_id ASC
                """,
                (case_id, workspace_id),
            )
            summary.artifact_scanned_count += len(artifact_rows)
            protected_artifacts = False
            for artifact_row in artifact_rows:
                artifact_id = _trim(artifact_row.get("artifact_id"))
                artifact_retention = _trim(artifact_row.get("retention_state")) or "active"
                if bool(artifact_row.get("is_formalized")) or artifact_retention == "retained":
                    protected_artifacts = True
                    summary.add_skip("protected_artifact")
                    continue
                if artifact_id:
                    engine.execute(
                        "DELETE FROM analysis_workspace_artifact WHERE case_id=? AND artifact_id=?",
                        (case_id, artifact_id),
                    )
                    summary.deleted_count += 1
                    summary.deleted_artifact_ids.append(artifact_id)
            if protected_artifacts:
                continue
            if expires_at + delete_grace <= now_dt:
                engine.execute(
                    "DELETE FROM analysis_scratchpad_workspace WHERE case_id=? AND workspace_id=?",
                    (case_id, workspace_id),
                )
                summary.deleted_count += 1
                summary.deleted_workspace_ids.append(workspace_id)
        return summary.to_payload()

    def _cleanup_cache_retention(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
    ) -> dict[str, Any]:
        summary = CacheJanitorSummary()
        del case_id
        self._invalidate_legacy_fact_caches(engine)
        summary.add_skip("legacy_fact_cache_tables_absent")
        return summary.to_payload()

    def _cleanup_memory_retention(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
    ) -> dict[str, Any]:
        summary = MemoryJanitorSummary()
        summary.add_skip("no_memory_retention_targets")
        return summary.to_payload()

    def _cleanup_run_log_retention(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
    ) -> dict[str, Any]:
        summary = RunLogRetentionSummary()
        now_dt = _coerce_datetime(_now_text()) or datetime.now()
        cutoff = now_dt - timedelta(days=max(1, int(DEFAULT_RUNTIME_RETENTION_POLICY.run_log_ttl_days or 7)))
        rows = self._query_dicts(
            engine,
            """
            SELECT *
            FROM analysis_run_log_event
            WHERE case_id=?
            ORDER BY run_id ASC, turn_id ASC, sequence_no ASC, created_at ASC
            """,
            (case_id,),
        )
        summary.scanned_count = len(rows)
        if not rows:
            summary.add_skip("missing_run_log")
            return summary.to_payload()
        first_last_ids: dict[tuple[str, str], list[str]] = {}
        for row in rows:
            key = (_trim(row.get("run_id")), _trim(row.get("turn_id")))
            event_id = _trim(row.get("event_id"))
            if not event_id:
                continue
            bucket = first_last_ids.setdefault(key, [])
            if not bucket:
                bucket.append(event_id)
            if len(bucket) == 1:
                bucket.append(event_id)
            else:
                bucket[1] = event_id
        preserved_boundary_ids = {
            event_id
            for values in first_last_ids.values()
            for event_id in values
            if _trim(event_id)
        }
        for row in rows:
            event_id = _trim(row.get("event_id"))
            event_stage = _trim(row.get("event_stage"))
            event_type = _trim(row.get("event_type"))
            payload = _json_loads(row.get("payload_json"), {})
            if event_id in preserved_boundary_ids:
                summary.add_skip("boundary_event")
                summary.preserved_anchor_count += 1
                continue
            if is_run_log_anchor(event_stage=event_stage, event_type=event_type, payload=payload):
                summary.add_skip("anchor_event")
                summary.preserved_anchor_count += 1
                continue
            created_dt = _coerce_datetime(row.get("created_at"))
            if created_dt is None:
                summary.add_skip("missing_created_at")
                continue
            if created_dt > cutoff:
                summary.add_skip("fresh_run_log")
                continue
            summary.add_stale("ttl_expired")
            if event_id:
                engine.execute(
                    "DELETE FROM analysis_run_log_event WHERE case_id=? AND event_id=?",
                    (case_id, event_id),
                )
                summary.deleted_count += 1
                summary.deleted_event_ids.append(event_id)
        return summary.to_payload()

    def run_runtime_retention_janitor(self, *, case_id: str = "") -> dict[str, Any]:
        requested_case_id = _trim(case_id)
        if requested_case_id and not self.case_exists(requested_case_id):
            raise KeyError(requested_case_id)
        case_ids: list[str]
        if requested_case_id:
            case_ids = [requested_case_id]
        else:
            case_ids = [
                _trim(getattr(case, "case_id", ""))
                for case in self._storage.list_cases(include_deleted=False)
                if _trim(getattr(case, "case_id", ""))
            ]
        items: list[dict[str, Any]] = []
        case_count = 0
        affected_case_count = 0
        scanned_count = 0
        stale_count = 0
        deleted_count = 0
        skipped_count = 0
        for current_case_id in case_ids:
            if not current_case_id:
                continue
            case_count += 1
            engine = self.open_case_engine(current_case_id)
            try:
                self.ensure_analysis_schema(engine)
                workspace_summary = self._cleanup_workspace_retention(engine, case_id=current_case_id)
                cache_summary = self._cleanup_cache_retention(engine, case_id=current_case_id)
                memory_summary = self._cleanup_memory_retention(engine, case_id=current_case_id)
                run_log_summary = self._cleanup_run_log_retention(engine, case_id=current_case_id)
            finally:
                engine.close()
            case_scanned = (
                int(workspace_summary.get("scanned_count") or 0)
                + int(cache_summary.get("scanned_count") or 0)
                + int(memory_summary.get("scanned_count") or 0)
                + int(run_log_summary.get("scanned_count") or 0)
            )
            case_stale = (
                int(workspace_summary.get("stale_count") or 0)
                + int(cache_summary.get("stale_count") or 0)
                + int(memory_summary.get("stale_count") or 0)
                + int(run_log_summary.get("stale_count") or 0)
            )
            case_deleted = (
                int(workspace_summary.get("deleted_count") or 0)
                + int(cache_summary.get("deleted_count") or 0)
                + int(memory_summary.get("deleted_count") or 0)
                + int(run_log_summary.get("deleted_count") or 0)
            )
            case_skipped = (
                int(workspace_summary.get("skipped_count") or 0)
                + int(cache_summary.get("skipped_count") or 0)
                + int(memory_summary.get("skipped_count") or 0)
                + int(run_log_summary.get("skipped_count") or 0)
            )
            scanned_count += case_scanned
            stale_count += case_stale
            deleted_count += case_deleted
            skipped_count += case_skipped
            if case_stale > 0 or case_deleted > 0:
                affected_case_count += 1
            items.append(
                {
                    "case_id": current_case_id,
                    "policy": DEFAULT_RUNTIME_RETENTION_POLICY.to_payload(),
                    "workspace": workspace_summary,
                    "cache": cache_summary,
                    "memory": memory_summary,
                    "run_log": run_log_summary,
                    "scanned_count": case_scanned,
                    "stale_count": case_stale,
                    "deleted_count": case_deleted,
                    "skipped_count": case_skipped,
                }
            )
        affected_item_count = stale_count + deleted_count
        return {
            "job_kind": "runtime_retention_janitor",
            "case_count": case_count,
            "affected_case_count": affected_case_count,
            "affected_scope_count": affected_item_count,
            "affected_item_count": affected_item_count,
            "scanned_count": scanned_count,
            "stale_count": stale_count,
            "deleted_count": deleted_count,
            "skipped_count": skipped_count,
            "items": items,
        }

    def run_temp_scope_janitor(self) -> dict[str, Any]:
        results: list[dict[str, Any]] = []
        affected_scope_count = 0
        case_count = 0
        for case in self._storage.list_cases(include_deleted=False):
            case_id = _trim(getattr(case, "case_id", ""))
            if not case_id:
                continue
            case_count += 1
            engine = self.open_case_engine(case_id)
            try:
                self.ensure_analysis_schema(engine)
                warnings = self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
                if warnings:
                    affected_scope_count += len(warnings)
                    results.append({"case_id": case_id, "warnings": warnings})
            finally:
                engine.close()
        return {
            "contract": "TempScopeJanitorPublicV2",
            "case_count": case_count,
            "affected_case_count": len(results),
            "affected_scope_count": affected_scope_count,
            "items": [],
            "raw_details_exposed": False,
        }

    def _query_import_file_rows(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        file_ids: Sequence[str],
    ) -> dict[str, dict[str, Any]]:
        normalized_file_ids = _trim_list(file_ids or [])
        if not normalized_file_ids or not self._table_exists(engine, "import_file_log"):
            return {}
        rows = self._query_dicts(
            engine,
            f"""
            SELECT case_id, file_id, kind, filename, display_path, stored_path, file_type, size, md5, sha256,
                   rows_total, rows_imported_raw, rows_imported_norm, import_counts_version, status, finished_at
            FROM import_file_log
            WHERE case_id=?
              AND file_id IN ({','.join(['?'] * len(normalized_file_ids))})
            """,
            (case_id, *normalized_file_ids),
        )
        result: dict[str, dict[str, Any]] = {}
        for row in rows:
            file_id = _trim(row.get("file_id"))
            if file_id:
                result[file_id] = row
        return result

    def _temp_transaction_import_ready(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        expected_file_id: str,
        row: Mapping[str, Any],
    ) -> bool:
        """Accept an existing temp-scope source only with exact persisted coverage."""

        file_id = _trim(row.get("file_id"))
        rows_total = known_public_import_count(row.get("rows_total"))
        rows_imported_raw = known_public_import_count(row.get("rows_imported_raw"))
        rows_imported_norm = known_public_import_count(row.get("rows_imported_norm"))
        source_sha256 = _trim(row.get("sha256")).lower()
        if (
            not file_id
            or file_id != _trim(expected_file_id)
            or _trim(row.get("case_id")) != _trim(case_id)
            or re.fullmatch(r"temp_fc_[a-f0-9]{64}", file_id) is not None
            or _trim(row.get("kind")) != "fc_transaction"
            or not _trim(row.get("stored_path"))
            or re.fullmatch(r"[a-f0-9]{64}", source_sha256) is None
            or not is_import_success_status(row.get("status"))
            or not _trim(row.get("finished_at"))
            or type(row.get("import_counts_version")) is not int
            or row.get("import_counts_version") != IMPORT_COUNTS_VERSION
            or rows_total is None
            or rows_imported_raw is None
            or rows_imported_norm is None
            or rows_total <= 0
            or rows_imported_raw <= 0
            or rows_imported_norm <= 0
            or not (rows_imported_norm <= rows_imported_raw <= rows_total)
        ):
            return False
        try:
            if not self._table_exists(engine, "fc_transaction_raw") or not self._table_exists(engine, "fc_transaction_norm"):
                return False
            rows = engine.query(
                """
                SELECT
                    (SELECT COUNT(1) FROM fc_transaction_raw WHERE case_id=? AND file_id=?),
                    (SELECT COUNT(1) FROM fc_transaction_norm WHERE case_id=? AND file_id=?)
                """,
                (case_id, file_id, case_id, file_id),
            )
        except Exception:
            return False
        return (
            len(rows) == 1
            and len(rows[0]) == 2
            and known_public_import_count(rows[0][0]) == rows_imported_raw
            and known_public_import_count(rows[0][1]) == rows_imported_norm
        )

    def _query_document_asset_rows(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        document_ids: Sequence[str],
    ) -> dict[str, dict[str, Any]]:
        normalized_document_ids = _trim_list(document_ids or [])
        if not normalized_document_ids or not self._table_exists(engine, "document_assets"):
            return {}
        rows = self._query_dicts(
            engine,
            f"""
            SELECT document_id, file_id, kind, filename, display_path, stored_path, file_type, size, md5, sha256, content_path
            FROM document_assets
            WHERE case_id=?
              AND document_id IN ({','.join(['?'] * len(normalized_document_ids))})
            """,
            (case_id, *normalized_document_ids),
        )
        result: dict[str, dict[str, Any]] = {}
        for row in rows:
            document_id = _trim(row.get("document_id"))
            if document_id:
                result[document_id] = row
        return result

    def _ingest_temp_transaction_files(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        source_file_ids: Sequence[str],
        document_ids: Sequence[str],
    ) -> dict[str, Any]:
        normalized_source_file_ids = _trim_list(source_file_ids or [])
        normalized_document_ids = _trim_list(document_ids or [])
        import_rows = self._query_import_file_rows(
            engine,
            case_id=case_id,
            file_ids=normalized_source_file_ids,
        )
        document_rows = self._query_document_asset_rows(
            engine,
            case_id=case_id,
            document_ids=normalized_document_ids,
        )
        resolved_file_ids: list[str] = []
        skipped_sources: list[dict[str, Any]] = []

        def reject(source_token: str, failure_code: str) -> None:
            skipped_sources.append(
                project_temp_scope_failure(
                    case_id=case_id,
                    source_token=source_token,
                    failure_code=failure_code,
                )
            )

        for source_file_id in normalized_source_file_ids:
            row = dict(import_rows.get(source_file_id) or {})
            if self._temp_transaction_import_ready(
                engine,
                case_id=case_id,
                expected_file_id=source_file_id,
                row=row,
            ):
                resolved_file_ids.append(source_file_id)
            else:
                reject(
                    source_file_id,
                    "temp_import_failed" if row else "source_not_registered",
                )

        for document_id in normalized_document_ids:
            document_row = dict(document_rows.get(document_id) or {})
            original_file_id = _trim(document_row.get("file_id"))
            import_row = {}
            if original_file_id:
                import_row = dict(
                    import_rows.get(original_file_id)
                    or self._query_import_file_rows(
                        engine,
                        case_id=case_id,
                        file_ids=[original_file_id],
                    ).get(original_file_id)
                    or {}
                )
            if original_file_id and self._temp_transaction_import_ready(
                engine,
                case_id=case_id,
                expected_file_id=original_file_id,
                row=import_row,
            ):
                resolved_file_ids.append(original_file_id)
            else:
                reject(
                    document_id,
                    "temp_import_failed" if document_row else "source_not_registered",
                )

        return {
            "resolved_file_ids": _unique_texts(resolved_file_ids),
            "imported_file_ids": [],
            "imported_documents": [],
            "skipped_sources": skipped_sources,
            "warnings": [],
        }
    def _rule_param_rows(self, engine: DuckDBEngine, case_id: str) -> list[dict[str, Any]]:
        return self._query_dicts(
            engine,
            """
            SELECT * FROM analysis_rule_config
             WHERE (scope_type='system' AND scope_key='global')
                OR (scope_type='case' AND scope_key=?)
             ORDER BY CASE WHEN scope_type='system' THEN 0 ELSE 1 END, updated_at ASC
            """,
            (case_id,),
        )

    def _load_rule_params(self, engine: DuckDBEngine, case_id: str) -> dict[str, Any]:
        scoped = _rule_param_scope_overrides(self._rule_param_rows(engine, case_id), case_id=case_id)
        overrides = {**scoped["system"], **scoped["case"]}
        return _normalize_rule_params(overrides)

    def get_rule_params(self, case_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            case_params = {
                _trim(row.get("param_key")): _json_loads(row.get("param_value_json"), None)
                for row in self._query_dicts(
                    engine,
                    "SELECT * FROM analysis_rule_config WHERE scope_type='case' AND scope_key=? ORDER BY updated_at ASC",
                    (case_id,),
                )
                if _trim(row.get("param_key"))
            }
            system_params = {
                _trim(row.get("param_key")): _json_loads(row.get("param_value_json"), None)
                for row in self._query_dicts(
                    engine,
                    "SELECT * FROM analysis_rule_config WHERE scope_type='system' AND scope_key='global' ORDER BY updated_at ASC",
                )
                if _trim(row.get("param_key"))
            }
            return {
                "case_id": case_id,
                "effective_params": self._load_rule_params(engine, case_id),
                "case_params": case_params,
                "system_params": system_params,
            }
        finally:
            engine.close()

    def update_rule_params(self, case_id: str, *, scope_type: str, params: dict[str, Any]) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_scope = _trim(scope_type).lower()
        if normalized_scope not in {"case", "system"}:
            raise ValueError("rule_parameter_scope_invalid")
        validated_updates = _validate_rule_param_overrides(params)
        scope_key = "global" if normalized_scope == "system" else case_id
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            scoped = _rule_param_scope_overrides(self._rule_param_rows(engine, case_id), case_id=case_id)
            scoped[normalized_scope].update(validated_updates)
            effective_params = _normalize_rule_params({**scoped["system"], **scoped["case"]})
            updated_at = _now_text()
            updated_keys: list[str] = []
            transaction_started = False
            try:
                engine.execute("BEGIN TRANSACTION")
                transaction_started = True
                for key, value in validated_updates.items():
                    config_key = f"{normalized_scope}:{scope_key}:{key}"
                    value_json = _json_dumps(value)
                    rows = self._query_dicts(
                        engine,
                        "SELECT config_key FROM analysis_rule_config WHERE config_key=? LIMIT 1",
                        (config_key,),
                    )
                    if rows:
                        engine.execute(
                            """
                            UPDATE analysis_rule_config
                               SET param_value_json=?, updated_at=?
                             WHERE config_key=?
                            """,
                            (value_json, updated_at, config_key),
                        )
                    else:
                        engine.execute(
                            """
                            INSERT INTO analysis_rule_config(
                                config_key, scope_type, scope_key, param_key, param_value_json, updated_at
                            ) VALUES (?,?,?,?,?,?)
                            """,
                            (config_key, normalized_scope, scope_key, key, value_json, updated_at),
                        )
                    updated_keys.append(key)
                engine.execute("COMMIT")
                transaction_started = False
            except Exception:
                if transaction_started:
                    try:
                        engine.execute("ROLLBACK")
                    except Exception:
                        pass
                raise
            return {
                "case_id": case_id,
                "scope_type": normalized_scope,
                "updated_keys": updated_keys,
                "effective_params": effective_params,
            }
        finally:
            engine.close()

    def _build_rule_hit(
        self,
        case_id: str,
        *,
        rule_code: str,
        entity_ids: Sequence[str],
        txn_ids: Sequence[str],
        summary: str,
        detail_json: dict[str, Any],
        score: float,
        severity: str = "",
    ) -> dict[str, Any]:
        definition = RULE_DEFINITIONS.get(rule_code, {})
        normalized_entities = _unique_texts(entity_ids)
        normalized_txns = _clip_items(txn_ids, 24)
        effective_severity = _trim(severity) or definition.get("severity", "medium")
        signature = json.dumps(
            {
                "rule_code": rule_code,
                "entity_ids": normalized_entities,
                "txn_ids": normalized_txns,
                "detail_json": detail_json,
            },
            ensure_ascii=False,
            sort_keys=True,
        )
        return {
            "rule_hit_id": _stable_slug("rh", case_id, rule_code, signature),
            "case_id": case_id,
            "rule_code": rule_code,
            "title": definition.get("title", rule_code),
            "risk_type": definition.get("risk_type", "unknown"),
            "severity": effective_severity,
            "score": _clamp_score(score),
            "entity_ids": normalized_entities,
            "txn_ids": normalized_txns,
            "summary": _trim(summary),
            "detail_json": detail_json,
        }

    def _compute_rule_hits(
        self,
        case_id: str,
        txn_rows: Sequence[dict[str, Any]],
        *,
        params: Optional[dict[str, Any]] = None,
        native_pattern_features: Optional[Mapping[str, Any]] = None,
    ) -> list[dict[str, Any]]:
        if _rule_input_coverage(txn_rows).get("status") != "complete":
            return []
        hits: list[dict[str, Any]] = []
        config = _normalize_rule_params(params)
        cash_input_coverage = _cash_rule_input_coverage(txn_rows)
        cash_rules_ready = cash_input_coverage.get("status") == "complete"
        # Native pattern rows are optimization artifacts, not host evidence. Until
        # they carry the same field-complete receipt binding, deterministic rules
        # must derive candidates from the verified transaction rows themselves.
        native_patterns_ready = False
        native_patterns_by_account: dict[str, Any] = {}
        rows_by_account: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in txn_rows:
            account_key = _trim(row.get("account_key"))
            if account_key:
                rows_by_account[account_key].append(row)

        rule_engine_version = _trim(config.get("rule_engine_version")) or "economic_crime_fund_rules"
        fast_window = timedelta(minutes=max(1, _as_int(config.get("fast_in_out_window_minutes"))))
        fast_ratio_threshold = max(0.1, _as_float(config.get("fast_in_out_ratio")))
        fast_min_amount = max(0.0, _as_float(config.get("fast_in_out_min_amount")))
        fast_strong_amount = max(fast_min_amount, _as_float(config.get("fast_in_out_strong_amount")))
        small_fast_window = timedelta(minutes=max(1, _as_int(config.get("small_fast_in_out_window_minutes"))))
        small_fast_ratio = max(0.1, _as_float(config.get("small_fast_in_out_ratio")))
        small_fast_min_amount = max(0.0, _as_float(config.get("small_fast_in_out_min_amount")))
        small_fast_max_amount = max(small_fast_min_amount, _as_float(config.get("small_fast_in_out_max_amount")))
        small_fast_min_event_count = max(2, _as_int(config.get("small_fast_in_out_min_event_count")))
        small_fast_min_total_amount = max(0.0, _as_float(config.get("small_fast_in_out_min_total_amount")))
        single_fast_window = timedelta(minutes=max(1, _as_int(config.get("single_fast_in_out_candidate_window_minutes"))))
        single_fast_min_amount = max(0.0, _as_float(config.get("single_fast_in_out_candidate_min_amount")))
        single_fast_max_amount = max(single_fast_min_amount, _as_float(config.get("single_fast_in_out_candidate_max_amount")))
        single_fast_min_ratio = max(0.1, _as_float(config.get("single_fast_in_out_candidate_min_out_ratio")))
        single_fast_max_ratio = max(single_fast_min_ratio, _as_float(config.get("single_fast_in_out_candidate_max_out_ratio")))
        single_fast_max_event_count = max(1, _as_int(config.get("single_fast_in_out_candidate_max_event_count")))
        near_threshold = max(0.0, _as_float(config.get("near_threshold_amount")))
        near_threshold_lower_rate = max(0.0, min(0.99, _as_float(config.get("near_threshold_lower_rate"))))
        near_threshold_window = timedelta(minutes=max(1, _as_int(config.get("near_threshold_window_minutes"))))
        near_threshold_min_count = max(2, _as_int(config.get("near_threshold_min_count")))
        near_threshold_min_total = max(0.0, _as_float(config.get("near_threshold_min_total_amount")))
        cash_quick_window = timedelta(minutes=max(1, _as_int(config.get("cash_quick_in_out_window_minutes"))))
        cash_quick_min_amount = max(0.0, _as_float(config.get("cash_quick_in_out_min_amount")))
        cash_quick_min_event_count = max(2, _as_int(config.get("cash_quick_in_out_min_event_count")))
        cash_quick_min_total = max(0.0, _as_float(config.get("cash_quick_in_out_min_total_amount")))
        cash_candidate_window = timedelta(minutes=max(1, _as_int(config.get("cash_quick_candidate_window_minutes"))))
        cash_candidate_min_amount = max(0.0, _as_float(config.get("cash_quick_candidate_min_amount")))
        cash_candidate_min_ratio = max(0.1, _as_float(config.get("cash_quick_candidate_min_out_ratio")))
        cash_candidate_max_ratio = max(cash_candidate_min_ratio, _as_float(config.get("cash_quick_candidate_max_out_ratio")))
        cash_candidate_max_event_count = max(1, _as_int(config.get("cash_quick_candidate_max_event_count")))

        def _row_text_contains(row: dict[str, Any], markers: Sequence[str]) -> bool:
            text = " ".join(
                _trim(row.get(field))
                for field in (
                    "summary",
                    "txn_type",
                    "remark",
                    "counterparty_name",
                    "counterparty_bank",
                    "voucher_type",
                    "branch_name",
                    "location",
                )
            )
            upper_text = text.upper()
            return any(_trim(marker) and _trim(marker).upper() in upper_text for marker in markers)

        def _is_night_or_offhour(ts: datetime) -> bool:
            start_hour = max(0, min(23, _as_int(config.get("night_activity_start_hour"))))
            end_hour = max(0, min(23, _as_int(config.get("night_activity_end_hour"))))
            hour = int(ts.hour)
            if start_hour > end_hour:
                return hour >= start_hour or hour < end_hour
            return start_hour <= hour < end_hour

        def _same_name_counterparty(row: dict[str, Any]) -> bool:
            account_name = _trim(row.get("account_name"))
            counterparty_name = _trim(row.get("counterparty_name"))
            return bool(account_name and counterparty_name and account_name == counterparty_name)

        for account_key, rows in rows_by_account.items():
            ordered = [row for row in rows if row.get("txn_ts")]
            ordered.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
            txn_by_id = {_trim(row.get("txn_id")): row for row in ordered if _trim(row.get("txn_id"))}
            native_account_patterns = dict(native_patterns_by_account.get(account_key) or {})

            def _native_event_rows(feature_code: str) -> list[dict[str, Any]]:
                events: list[dict[str, Any]] = []
                for feature in native_account_patterns.get(feature_code, []):
                    detail = _json_loads(feature.get("detail_json"), {})
                    in_row = txn_by_id.get(_trim(detail.get("in_txn_id")))
                    out_rows_native = [
                        txn_by_id[txn_id]
                        for txn_id in _trim_list(detail.get("out_txn_ids") or [])
                        if txn_id in txn_by_id
                    ]
                    if in_row is None or not out_rows_native:
                        continue
                    events.append(
                        {
                            "in_row": in_row,
                            "out_rows": out_rows_native,
                            "in_amount": round(_as_float(detail.get("in_amount")), 2),
                            "out_amount": round(_as_float(detail.get("out_amount")), 2),
                            "out_ratio": round(_as_float(detail.get("out_ratio")), 4),
                            "duration_minutes": round(_as_float(detail.get("duration_minutes")), 2),
                        }
                    )
                return events

            def _native_feature_txn_rows(feature: Mapping[str, Any]) -> list[dict[str, Any]]:
                txn_ids = _trim_list(_json_loads(feature.get("txn_ids_json"), []))
                if not txn_ids:
                    return []
                rows_for_feature = [txn_by_id[txn_id] for txn_id in txn_ids if txn_id in txn_by_id]
                return rows_for_feature if len(rows_for_feature) == len(txn_ids) else []

            out_rows = [row for row in ordered if row.get("direction_norm") == "out"]
            last_hit_end: Optional[datetime] = None
            for in_row in ordered:
                if in_row.get("direction_norm") != "in" or in_row.get("txn_ts") is None or in_row.get("amount_val", 0) <= 0:
                    continue
                in_amount = _as_float(in_row.get("amount_val"))
                if in_amount < fast_min_amount:
                    continue
                if last_hit_end and in_row["txn_ts"] <= last_hit_end:
                    continue
                matching_outs = [
                    row
                    for row in out_rows
                    if in_row["txn_ts"] < row["txn_ts"] <= in_row["txn_ts"] + fast_window
                ]
                if not matching_outs:
                    continue
                out_amount = round(sum(_as_float(row.get("amount_val")) for row in matching_outs), 2)
                out_ratio = out_amount / max(in_amount, 0.01)
                if out_ratio < fast_ratio_threshold:
                    continue
                end_balance = _optional_rounded_float(matching_outs[-1].get("balance_val"), 2)
                start_balance = _optional_rounded_float(in_row.get("balance_val"), 2)
                balance_return_ratio = (
                    max(0.0, min(1.0, (start_balance - end_balance) / max(in_amount, 1.0)))
                    if start_balance is not None and end_balance is not None
                    else None
                )
                hit_rows = [in_row, *matching_outs]
                fast_severity = "high" if in_amount >= fast_strong_amount or out_ratio >= 0.95 else "medium"
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="FAST_IN_FAST_OUT",
                        entity_ids=[in_row.get("account_entity_id")],
                        txn_ids=[row.get("txn_id") for row in hit_rows],
                        summary=(
                            f"账户 {account_key} 在 {int(fast_window.total_seconds() // 60)} 分钟内流入 {format(in_amount, '.2f')} 元，"
                            f"随后分 {len(matching_outs)} 笔流出 {format(out_amount, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "window_minutes": int(fast_window.total_seconds() // 60),
                            "in_amount": round(in_amount, 2),
                            "out_amount": out_amount,
                            "out_ratio": round(out_ratio, 4),
                            "out_count": len(matching_outs),
                            "rule_params": {
                                "min_amount": fast_min_amount,
                                "strong_amount": fast_strong_amount,
                                "min_out_ratio": fast_ratio_threshold,
                                "window_minutes": int(fast_window.total_seconds() // 60),
                            },
                            "regulatory_reference": {
                                "large_cash_rmb": 50000.0,
                                "legacy_large_individual_transfer_rmb": 200000.0,
                                "current_natural_person_domestic_transfer_rmb": 500000.0,
                            },
                            "start_time": _trim(in_row.get("txn_time")),
                            "end_time": _trim(matching_outs[-1].get("txn_time")),
                            "start_balance": start_balance,
                            "end_balance": end_balance,
                            "balance_return_ratio": (
                                round(balance_return_ratio, 4)
                                if balance_return_ratio is not None
                                else None
                            ),
                        },
                        score=(
                            0.76
                            + min(out_ratio, 1.2) * 0.13
                            + (balance_return_ratio * 0.08 if balance_return_ratio is not None else 0.0)
                        ),
                        severity=fast_severity,
                    )
                )
                last_hit_end = matching_outs[-1]["txn_ts"]

            small_amount_threshold = max(0.0, _as_float(config.get("small_amount_threshold")))
            high_freq_window = timedelta(minutes=max(1, _as_int(config.get("high_freq_window_minutes"))))
            high_freq_threshold = max(2, _as_int(config.get("high_freq_count_threshold")))
            high_freq_min_total = max(0.0, _as_float(config.get("high_freq_small_min_total_amount")))
            if native_patterns_ready and "HIGH_FREQ_SMALL_OUT" in native_account_patterns:
                for feature in native_account_patterns.get("HIGH_FREQ_SMALL_OUT", []):
                    window_rows = _native_feature_txn_rows(feature)
                    if len(window_rows) < high_freq_threshold:
                        continue
                    total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                    if total_amount < high_freq_min_total:
                        continue
                    severity = "high" if len(window_rows) >= max(high_freq_threshold + 4, 12) else "medium"
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="HIGH_FREQ_SMALL_OUT",
                            entity_ids=[window_rows[0].get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in window_rows],
                            summary=(
                                f"账户 {account_key} 在 {int(high_freq_window.total_seconds() // 60)} 分钟内连续 {len(window_rows)} 笔小额转出，"
                                f"累计金额 {format(total_amount, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "window_minutes": int(high_freq_window.total_seconds() // 60),
                                "txn_count": len(window_rows),
                                "small_amount_threshold": small_amount_threshold,
                                "min_total_amount": high_freq_min_total,
                                "total_out_amount": total_amount,
                                "start_time": _trim(window_rows[0].get("txn_time")),
                                "end_time": _trim(window_rows[-1].get("txn_time")),
                            },
                            score=0.58 + min(len(window_rows) / max(high_freq_threshold + 4, 12), 1.3) * 0.22,
                            severity=severity,
                        )
                    )

            small_fast_events: list[dict[str, Any]] = []
            if native_patterns_ready and "SMALL_FAST_EVENT" in native_account_patterns:
                small_fast_events = _native_event_rows("SMALL_FAST_EVENT")
            else:
                small_fast_in_rows = [
                    row
                    for row in ordered
                    if cash_rules_ready
                    and row.get("direction_norm") == "in"
                    and row.get("txn_ts") is not None
                    and _cash_txn_state(row) == "non_cash"
                ]
                for index, in_row in enumerate(small_fast_in_rows):
                    in_amount = _as_float(in_row.get("amount_val"))
                    if in_amount < small_fast_min_amount or in_amount >= small_fast_max_amount:
                        continue
                    next_in_ts = (
                        small_fast_in_rows[index + 1].get("txn_ts")
                        if index + 1 < len(small_fast_in_rows)
                        else None
                    )
                    window_end = in_row["txn_ts"] + small_fast_window
                    if isinstance(next_in_ts, datetime) and next_in_ts < window_end:
                        window_end = next_in_ts
                    matching_outs = [
                        row
                        for row in out_rows
                        if in_row["txn_ts"] < row["txn_ts"] <= window_end
                        and small_fast_min_amount <= _as_float(row.get("amount_val")) < small_fast_max_amount
                        and _cash_txn_state(row) == "non_cash"
                    ]
                    if not matching_outs:
                        continue
                    out_amount = round(sum(_as_float(row.get("amount_val")) for row in matching_outs), 2)
                    out_ratio = out_amount / max(in_amount, 0.01)
                    if out_ratio < small_fast_ratio:
                        continue
                    small_fast_events.append(
                        {
                            "in_row": in_row,
                            "out_rows": matching_outs,
                            "in_amount": round(in_amount, 2),
                            "out_amount": out_amount,
                            "out_ratio": round(out_ratio, 4),
                            "duration_minutes": round(
                                (matching_outs[-1]["txn_ts"] - in_row["txn_ts"]).total_seconds() / 60,
                                2,
                            ),
                        }
                    )
            small_fast_total = round(sum(_as_float(item.get("in_amount")) for item in small_fast_events), 2)
            has_formal_small_fast = (
                len(small_fast_events) >= small_fast_min_event_count
                and small_fast_total >= small_fast_min_total_amount
            )
            if has_formal_small_fast:
                event_rows: list[dict[str, Any]] = []
                for event in small_fast_events:
                    in_row = event.get("in_row")
                    if isinstance(in_row, dict):
                        event_rows.append(in_row)
                    event_rows.extend(row for row in list(event.get("out_rows") or []) if isinstance(row, dict))
                event_rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
                total_out = round(sum(_as_float(item.get("out_amount")) for item in small_fast_events), 2)
                max_ratio = max(_as_float(item.get("out_ratio")) for item in small_fast_events)
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="SMALL_FAST_IN_OUT",
                        entity_ids=[event_rows[0].get("account_entity_id") if event_rows else None],
                        txn_ids=[row.get("txn_id") for row in event_rows],
                        summary=(
                            f"账户 {account_key} 出现 {len(small_fast_events)} 组小额入账后短时转出，"
                            f"累计入账 {format(small_fast_total, '.2f')} 元、转出 {format(total_out, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "event_count": len(small_fast_events),
                            "window_minutes": int(small_fast_window.total_seconds() // 60),
                            "amount_range": {
                                "min": small_fast_min_amount,
                                "max_exclusive": small_fast_max_amount,
                            },
                            "min_event_count": small_fast_min_event_count,
                            "min_total_amount": small_fast_min_total_amount,
                            "min_out_ratio": small_fast_ratio,
                            "total_in_amount": small_fast_total,
                            "total_out_amount": total_out,
                            "max_out_ratio": round(max_ratio, 4),
                            "events": [
                                {
                                    "in_txn_id": _trim(dict(event.get("in_row") or {}).get("txn_id")),
                                    "in_amount": event.get("in_amount"),
                                    "out_amount": event.get("out_amount"),
                                    "out_ratio": event.get("out_ratio"),
                                    "out_txn_ids": [
                                        _trim(row.get("txn_id"))
                                        for row in list(event.get("out_rows") or [])
                                    ],
                                }
                                for event in small_fast_events[:20]
                            ],
                        },
                        score=0.64 + min(len(small_fast_events) / 6, 1.0) * 0.18 + min(max_ratio, 1.2) * 0.1,
                        severity="high" if small_fast_total >= fast_min_amount or len(small_fast_events) >= 6 else "medium",
                    )
                )
            elif small_fast_events:
                candidate_events = [
                    event
                    for event in small_fast_events
                    if single_fast_min_amount <= _as_float(event.get("in_amount")) < single_fast_max_amount
                    and single_fast_min_ratio <= _as_float(event.get("out_ratio")) <= single_fast_max_ratio
                    and _as_float(event.get("duration_minutes")) <= int(single_fast_window.total_seconds() // 60)
                ][:single_fast_max_event_count]
                if candidate_events:
                    candidate_rows: list[dict[str, Any]] = []
                    for event in candidate_events:
                        in_row = event.get("in_row")
                        if isinstance(in_row, dict):
                            candidate_rows.append(in_row)
                        candidate_rows.extend(row for row in list(event.get("out_rows") or []) if isinstance(row, dict))
                    candidate_rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
                    first_event = dict(candidate_events[0])
                    first_in_row = dict(first_event.get("in_row") or {})
                    first_out_rows = [row for row in list(first_event.get("out_rows") or []) if isinstance(row, dict)]
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="SINGLE_FAST_IN_OUT_CANDIDATE",
                            entity_ids=[candidate_rows[0].get("account_entity_id") if candidate_rows else None],
                            txn_ids=[row.get("txn_id") for row in candidate_rows],
                            summary=(
                                f"账户 {account_key} 出现单组小额入账后快速转出候选："
                                f"入账 {format(_as_float(first_event.get('in_amount')), '.2f')} 元，"
                                f"{format(_as_float(first_event.get('duration_minutes')), '.1f')} 分钟内转出 "
                                f"{format(_as_float(first_event.get('out_amount')), '.2f')} 元，"
                                f"转出比例 {format(_as_float(first_event.get('out_ratio')) * 100, '.1f')}%。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "assertion_level": "candidate",
                                "candidate_signal": True,
                                "candidate_reason": "单组小额资金近全额快速转出，不满足正式小额分散快进快出次数/累计金额条件，需结合重复次数、对手、设备、时间和资金用途复核。",
                                "event_count": len(candidate_events),
                                "observed_small_fast_event_count": len(small_fast_events),
                                "window_minutes": int(single_fast_window.total_seconds() // 60),
                                "formal_rule": "SMALL_FAST_IN_OUT",
                                "formal_min_event_count": small_fast_min_event_count,
                                "formal_min_total_amount": small_fast_min_total_amount,
                                "rule_params": {
                                    "min_amount": single_fast_min_amount,
                                    "max_amount_exclusive": single_fast_max_amount,
                                    "min_out_ratio": single_fast_min_ratio,
                                    "max_out_ratio": single_fast_max_ratio,
                                    "window_minutes": int(single_fast_window.total_seconds() // 60),
                                    "max_candidate_event_count": single_fast_max_event_count,
                                },
                                "in_txn_id": _trim(first_in_row.get("txn_id")),
                                "out_txn_ids": [_trim(row.get("txn_id")) for row in first_out_rows],
                                "in_amount": round(_as_float(first_event.get("in_amount")), 2),
                                "out_amount": round(_as_float(first_event.get("out_amount")), 2),
                                "out_ratio": round(_as_float(first_event.get("out_ratio")), 4),
                                "duration_minutes": round(_as_float(first_event.get("duration_minutes")), 2),
                                "start_time": _trim(first_in_row.get("txn_time")),
                                "end_time": _trim(first_out_rows[-1].get("txn_time")) if first_out_rows else "",
                                "promotion_criteria": {
                                    "repeat_event_count_at_least": small_fast_min_event_count,
                                    "repeat_total_amount_at_least": small_fast_min_total_amount,
                                    "or_combined_with": [
                                        "same_counterparty_or_device",
                                        "night_offhour_activity",
                                        "cash_quick_in_out",
                                        "near_threshold_or_repeated_amount",
                                    ],
                                },
                            },
                            score=0.44
                            + min(_as_float(first_event.get("out_ratio")), 1.0) * 0.12
                            + min(_as_float(first_event.get("in_amount")) / max(small_fast_min_total_amount, 1.0), 1.0) * 0.08,
                            severity="low",
                        )
                    )

            if near_threshold > 0:
                near_lower = near_threshold * near_threshold_lower_rate
                if native_patterns_ready and "NEAR_THRESHOLD_STRUCTURING" in native_account_patterns:
                    for feature in native_account_patterns.get("NEAR_THRESHOLD_STRUCTURING", []):
                        window_rows = _native_feature_txn_rows(feature)
                        if len(window_rows) < near_threshold_min_count:
                            continue
                        direction = _trim(feature.get("direction")) or "unknown"
                        total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                        if total_amount < near_threshold_min_total:
                            continue
                        amounts = [_as_float(row.get("amount_val")) for row in window_rows]
                        avg_gap = sum(near_threshold - amount for amount in amounts) / max(len(amounts), 1)
                        hits.append(
                            self._build_rule_hit(
                                case_id,
                                rule_code="NEAR_THRESHOLD_STRUCTURING",
                                entity_ids=[window_rows[0].get("account_entity_id")],
                                txn_ids=[row.get("txn_id") for row in window_rows],
                                summary=(
                                    f"账户 {account_key} 在 {int(near_threshold_window.total_seconds() // 60)} 分钟内"
                                    f"{len(window_rows)} 笔{_direction_label(direction)}低于 {format(near_threshold, '.0f')} 元临界值，"
                                    f"累计 {format(total_amount, '.2f')} 元。"
                                ),
                                detail_json={
                                    "rule_engine_version": rule_engine_version,
                                    "account_key": account_key,
                                    "direction": direction,
                                    "window_minutes": int(near_threshold_window.total_seconds() // 60),
                                    "txn_count": len(window_rows),
                                    "threshold": near_threshold,
                                    "lower_rate": near_threshold_lower_rate,
                                    "lower_amount": round(near_lower, 2),
                                    "min_count": near_threshold_min_count,
                                    "min_total_amount": near_threshold_min_total,
                                    "amount_min": round(min(amounts), 2),
                                    "amount_max": round(max(amounts), 2),
                                    "start_time": _trim(window_rows[0].get("txn_time")),
                                    "end_time": _trim(window_rows[-1].get("txn_time")),
                                },
                                score=0.67
                                + min(len(window_rows) / 5, 1.0) * 0.16
                                + max(0.0, 1 - (avg_gap / max(near_threshold * (1 - near_threshold_lower_rate), 1.0))) * 0.12,
                                severity="high" if len(window_rows) >= 3 or total_amount >= fast_strong_amount else "medium",
                            )
                        )
                else:
                    near_candidates = [
                        row
                        for row in ordered
                        if row.get("txn_ts") is not None
                        and row.get("direction_norm") in {"in", "out"}
                        and near_lower <= _as_float(row.get("amount_val")) < near_threshold
                    ]
                    near_by_direction: dict[str, list[dict[str, Any]]] = defaultdict(list)
                    for row in near_candidates:
                        near_by_direction[row.get("direction_norm") or "unknown"].append(row)
                    for direction, direction_rows in near_by_direction.items():
                        start = 0
                        while start < len(direction_rows):
                            end = start
                            while (
                                end < len(direction_rows)
                                and direction_rows[end]["txn_ts"] <= direction_rows[start]["txn_ts"] + near_threshold_window
                            ):
                                end += 1
                            window_rows = direction_rows[start:end]
                            if len(window_rows) >= near_threshold_min_count:
                                total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                                if total_amount < near_threshold_min_total:
                                    start += 1
                                    continue
                                amounts = [_as_float(row.get("amount_val")) for row in window_rows]
                                avg_gap = sum(near_threshold - amount for amount in amounts) / max(len(amounts), 1)
                                hits.append(
                                    self._build_rule_hit(
                                        case_id,
                                        rule_code="NEAR_THRESHOLD_STRUCTURING",
                                        entity_ids=[window_rows[0].get("account_entity_id")],
                                        txn_ids=[row.get("txn_id") for row in window_rows],
                                        summary=(
                                            f"账户 {account_key} 在 {int(near_threshold_window.total_seconds() // 60)} 分钟内"
                                            f"{len(window_rows)} 笔{_direction_label(direction)}低于 {format(near_threshold, '.0f')} 元临界值，"
                                            f"累计 {format(total_amount, '.2f')} 元。"
                                        ),
                                        detail_json={
                                            "rule_engine_version": rule_engine_version,
                                            "account_key": account_key,
                                            "direction": direction,
                                            "window_minutes": int(near_threshold_window.total_seconds() // 60),
                                            "txn_count": len(window_rows),
                                            "threshold": near_threshold,
                                            "lower_rate": near_threshold_lower_rate,
                                            "lower_amount": round(near_lower, 2),
                                            "min_count": near_threshold_min_count,
                                            "min_total_amount": near_threshold_min_total,
                                            "amount_min": round(min(amounts), 2),
                                            "amount_max": round(max(amounts), 2),
                                            "start_time": _trim(window_rows[0].get("txn_time")),
                                            "end_time": _trim(window_rows[-1].get("txn_time")),
                                        },
                                        score=0.67
                                        + min(len(window_rows) / 5, 1.0) * 0.16
                                        + max(0.0, 1 - (avg_gap / max(near_threshold * (1 - near_threshold_lower_rate), 1.0))) * 0.12,
                                        severity="high" if len(window_rows) >= 3 or total_amount >= fast_strong_amount else "medium",
                                    )
                                )
                                start = end
                                continue
                            start += 1

            cash_events: list[dict[str, Any]] = []
            if native_patterns_ready and "CASH_QUICK_EVENT" in native_account_patterns:
                cash_events = _native_event_rows("CASH_QUICK_EVENT")
            else:
                cash_in_rows = [
                    row
                    for row in ordered
                    if cash_rules_ready
                    and row.get("direction_norm") == "in"
                    and row.get("txn_ts") is not None
                    and _as_float(row.get("amount_val")) >= cash_quick_min_amount
                    and _cash_txn_state(row) == "cash"
                ]
                cash_out_rows = [
                    row
                    for row in ordered
                    if cash_rules_ready
                    and row.get("direction_norm") == "out"
                    and row.get("txn_ts") is not None
                    and _as_float(row.get("amount_val")) >= cash_quick_min_amount
                    and _cash_txn_state(row) == "cash"
                ]
                for index, in_row in enumerate(cash_in_rows):
                    next_cash_in_ts = (
                        cash_in_rows[index + 1].get("txn_ts")
                        if index + 1 < len(cash_in_rows)
                        else None
                    )
                    window_end = in_row["txn_ts"] + cash_quick_window
                    if isinstance(next_cash_in_ts, datetime) and next_cash_in_ts < window_end:
                        window_end = next_cash_in_ts
                    matching_outs = [
                        row
                        for row in cash_out_rows
                        if in_row["txn_ts"] < row["txn_ts"] <= window_end
                    ]
                    if not matching_outs:
                        continue
                    out_amount = round(sum(_as_float(row.get("amount_val")) for row in matching_outs), 2)
                    cash_events.append(
                        {
                            "in_row": in_row,
                            "out_rows": matching_outs,
                            "in_amount": round(_as_float(in_row.get("amount_val")), 2),
                            "out_amount": out_amount,
                            "out_ratio": round(out_amount / max(_as_float(in_row.get("amount_val")), 0.01), 4),
                            "duration_minutes": round(
                                (matching_outs[-1]["txn_ts"] - in_row["txn_ts"]).total_seconds() / 60,
                                2,
                            ),
                        }
                    )
            cash_total_in = round(sum(_as_float(item.get("in_amount")) for item in cash_events), 2)
            has_formal_cash_quick = len(cash_events) >= cash_quick_min_event_count and cash_total_in >= cash_quick_min_total
            if has_formal_cash_quick:
                cash_rows: list[dict[str, Any]] = []
                for event in cash_events:
                    in_row = event.get("in_row")
                    if isinstance(in_row, dict):
                        cash_rows.append(in_row)
                    cash_rows.extend(row for row in list(event.get("out_rows") or []) if isinstance(row, dict))
                cash_rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
                cash_total_out = round(sum(_as_float(item.get("out_amount")) for item in cash_events), 2)
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="CASH_QUICK_IN_OUT",
                        entity_ids=[cash_rows[0].get("account_entity_id") if cash_rows else None],
                        txn_ids=[row.get("txn_id") for row in cash_rows],
                        summary=(
                            f"账户 {account_key} 出现 {len(cash_events)} 组现金存入后短时现金支取/转出，"
                            f"现金流入 {format(cash_total_in, '.2f')} 元、流出 {format(cash_total_out, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "event_count": len(cash_events),
                            "window_minutes": int(cash_quick_window.total_seconds() // 60),
                            "min_amount": cash_quick_min_amount,
                            "min_event_count": cash_quick_min_event_count,
                            "min_total_amount": cash_quick_min_total,
                            "total_in_amount": cash_total_in,
                            "total_out_amount": cash_total_out,
                            "events": [
                                {
                                    "in_txn_id": _trim(dict(event.get("in_row") or {}).get("txn_id")),
                                    "in_amount": event.get("in_amount"),
                                    "out_amount": event.get("out_amount"),
                                    "out_txn_ids": [
                                        _trim(row.get("txn_id"))
                                        for row in list(event.get("out_rows") or [])
                                    ],
                                }
                                for event in cash_events[:20]
                            ],
                        },
                        score=0.63 + min(len(cash_events) / 4, 1.0) * 0.16 + min(cash_total_in / max(fast_min_amount, 1.0), 1.0) * 0.13,
                        severity="high" if cash_total_in >= fast_min_amount or len(cash_events) >= 4 else "medium",
                    )
                )
            else:
                candidate_cash_events: list[dict[str, Any]] = []
                if native_patterns_ready and "CASH_QUICK_CANDIDATE_EVENT" in native_account_patterns:
                    candidate_cash_events = _native_event_rows("CASH_QUICK_CANDIDATE_EVENT")[:cash_candidate_max_event_count]
                else:
                    candidate_cash_in_rows = [
                        row
                        for row in ordered
                        if cash_rules_ready
                        and row.get("direction_norm") == "in"
                        and row.get("txn_ts") is not None
                        and _as_float(row.get("amount_val")) >= cash_candidate_min_amount
                        and _cash_txn_state(row) == "cash"
                    ]
                    candidate_cash_out_rows = [
                        row
                        for row in ordered
                        if cash_rules_ready
                        and row.get("direction_norm") == "out"
                        and row.get("txn_ts") is not None
                        and _as_float(row.get("amount_val")) >= cash_candidate_min_amount
                        and _cash_txn_state(row) == "cash"
                    ]
                    for index, in_row in enumerate(candidate_cash_in_rows):
                        next_cash_in_ts = (
                            candidate_cash_in_rows[index + 1].get("txn_ts")
                            if index + 1 < len(candidate_cash_in_rows)
                            else None
                        )
                        window_end = in_row["txn_ts"] + cash_candidate_window
                        if isinstance(next_cash_in_ts, datetime) and next_cash_in_ts < window_end:
                            window_end = next_cash_in_ts
                        matching_outs = [
                            row
                            for row in candidate_cash_out_rows
                            if in_row["txn_ts"] < row["txn_ts"] <= window_end
                        ]
                        if not matching_outs:
                            continue
                        in_amount = _as_float(in_row.get("amount_val"))
                        out_amount = round(sum(_as_float(row.get("amount_val")) for row in matching_outs), 2)
                        out_ratio = out_amount / max(in_amount, 0.01)
                        if not (cash_candidate_min_ratio <= out_ratio <= cash_candidate_max_ratio):
                            continue
                        candidate_cash_events.append(
                            {
                                "in_row": in_row,
                                "out_rows": matching_outs,
                                "in_amount": round(in_amount, 2),
                                "out_amount": out_amount,
                                "out_ratio": round(out_ratio, 4),
                                "duration_minutes": round(
                                    (matching_outs[-1]["txn_ts"] - in_row["txn_ts"]).total_seconds() / 60,
                                    2,
                                ),
                            }
                        )
                        if len(candidate_cash_events) >= cash_candidate_max_event_count:
                            break
                if candidate_cash_events:
                    cash_candidate_rows: list[dict[str, Any]] = []
                    for event in candidate_cash_events:
                        in_row = event.get("in_row")
                        if isinstance(in_row, dict):
                            cash_candidate_rows.append(in_row)
                        cash_candidate_rows.extend(row for row in list(event.get("out_rows") or []) if isinstance(row, dict))
                    cash_candidate_rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
                    first_event = dict(candidate_cash_events[0])
                    first_in_row = dict(first_event.get("in_row") or {})
                    first_out_rows = [row for row in list(first_event.get("out_rows") or []) if isinstance(row, dict)]
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="SINGLE_CASH_QUICK_IN_OUT_CANDIDATE",
                            entity_ids=[cash_candidate_rows[0].get("account_entity_id") if cash_candidate_rows else None],
                            txn_ids=[row.get("txn_id") for row in cash_candidate_rows],
                            summary=(
                                f"账户 {account_key} 出现单组现金快存快取候选：现金入账 "
                                f"{format(_as_float(first_event.get('in_amount')), '.2f')} 元，"
                                f"{format(_as_float(first_event.get('duration_minutes')), '.1f')} 分钟内现金/柜面转出 "
                                f"{format(_as_float(first_event.get('out_amount')), '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "assertion_level": "candidate",
                                "candidate_signal": True,
                                "candidate_reason": "单组现金或柜面资金近全额快速取出/转出，尚未达到现金快存快取正式次数或累计金额条件，需结合柜面凭证、取现用途、对手和重复性复核。",
                                "event_count": len(candidate_cash_events),
                                "formal_rule": "CASH_QUICK_IN_OUT",
                                "formal_min_event_count": cash_quick_min_event_count,
                                "formal_min_total_amount": cash_quick_min_total,
                                "rule_params": {
                                    "min_amount": cash_candidate_min_amount,
                                    "min_out_ratio": cash_candidate_min_ratio,
                                    "max_out_ratio": cash_candidate_max_ratio,
                                    "window_minutes": int(cash_candidate_window.total_seconds() // 60),
                                    "max_candidate_event_count": cash_candidate_max_event_count,
                                },
                                "in_txn_id": _trim(first_in_row.get("txn_id")),
                                "out_txn_ids": [_trim(row.get("txn_id")) for row in first_out_rows],
                                "in_amount": round(_as_float(first_event.get("in_amount")), 2),
                                "out_amount": round(_as_float(first_event.get("out_amount")), 2),
                                "out_ratio": round(_as_float(first_event.get("out_ratio")), 4),
                                "duration_minutes": round(_as_float(first_event.get("duration_minutes")), 2),
                                "start_time": _trim(first_in_row.get("txn_time")),
                                "end_time": _trim(first_out_rows[-1].get("txn_time")) if first_out_rows else "",
                                "promotion_criteria": {
                                    "repeat_event_count_at_least": cash_quick_min_event_count,
                                    "repeat_total_amount_at_least": cash_quick_min_total,
                                    "or_combined_with": [
                                        "shared_teller_multi_account",
                                        "property_or_lifestyle_cash_use",
                                        "same_counterparty_or_device",
                                        "night_offhour_activity",
                                    ],
                                },
                            },
                            score=0.42
                            + min(_as_float(first_event.get("out_ratio")), 1.0) * 0.12
                            + min(_as_float(first_event.get("in_amount")) / max(cash_quick_min_total, 1.0), 1.0) * 0.08,
                            severity="low",
                        )
                    )

            repeated_window = timedelta(minutes=max(1, _as_int(config.get("repeated_amount_window_minutes"))))
            repeated_min_amount = max(0.0, _as_float(config.get("repeated_amount_min_amount")))
            repeated_min_count = max(2, _as_int(config.get("repeated_amount_min_count")))
            repeated_min_total = max(0.0, _as_float(config.get("repeated_amount_min_total_amount")))
            if native_patterns_ready and "REPEATED_AMOUNT_PATTERN" in native_account_patterns:
                for feature in native_account_patterns.get("REPEATED_AMOUNT_PATTERN", []):
                    window_rows = _native_feature_txn_rows(feature)
                    if len(window_rows) < repeated_min_count:
                        continue
                    direction = _trim(feature.get("direction")) or "unknown"
                    amounts = _json_loads(feature.get("amounts_json"), [])
                    amount_bucket = round(
                        _as_float(amounts[0] if isinstance(amounts, list) and amounts else 0.0),
                        2,
                    )
                    total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                    if total_amount < repeated_min_total:
                        continue
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="REPEATED_AMOUNT_PATTERN",
                            entity_ids=[window_rows[0].get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in window_rows],
                            summary=(
                                f"账户 {account_key} 在 {int(repeated_window.total_seconds() // 60)} 分钟内"
                                f"{len(window_rows)} 笔{_direction_label(direction)}金额均为 {format(amount_bucket, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "direction": direction,
                                "amount": amount_bucket,
                                "txn_count": len(window_rows),
                                "total_amount": total_amount,
                                "window_minutes": int(repeated_window.total_seconds() // 60),
                                "min_count": repeated_min_count,
                                "min_total_amount": repeated_min_total,
                                "start_time": _trim(window_rows[0].get("txn_time")),
                                "end_time": _trim(window_rows[-1].get("txn_time")),
                            },
                            score=0.58 + min(len(window_rows) / 8, 1.0) * 0.18 + min(total_amount / max(fast_min_amount, 1.0), 1.0) * 0.1,
                            severity="high" if len(window_rows) >= repeated_min_count + 3 or total_amount >= fast_strong_amount else "medium",
                        )
                    )
            else:
                repeated_candidates = [
                    row
                    for row in ordered
                    if row.get("txn_ts") is not None
                    and row.get("direction_norm") in {"in", "out"}
                    and _as_float(row.get("amount_val")) >= repeated_min_amount
                ]
                repeated_groups: dict[tuple[str, float], list[dict[str, Any]]] = defaultdict(list)
                for row in repeated_candidates:
                    repeated_groups[(row.get("direction_norm") or "unknown", round(_as_float(row.get("amount_val")), 2))].append(row)
                for (direction, amount_bucket), group_rows in repeated_groups.items():
                    if len(group_rows) < repeated_min_count:
                        continue
                    start = 0
                    while start < len(group_rows):
                        end = start
                        while end < len(group_rows) and group_rows[end]["txn_ts"] <= group_rows[start]["txn_ts"] + repeated_window:
                            end += 1
                        window_rows = group_rows[start:end]
                        total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                        if len(window_rows) >= repeated_min_count and total_amount >= repeated_min_total:
                            hits.append(
                                self._build_rule_hit(
                                    case_id,
                                    rule_code="REPEATED_AMOUNT_PATTERN",
                                    entity_ids=[window_rows[0].get("account_entity_id")],
                                    txn_ids=[row.get("txn_id") for row in window_rows],
                                    summary=(
                                        f"账户 {account_key} 在 {int(repeated_window.total_seconds() // 60)} 分钟内"
                                        f"{len(window_rows)} 笔{_direction_label(direction)}金额均为 {format(amount_bucket, '.2f')} 元。"
                                    ),
                                    detail_json={
                                        "rule_engine_version": rule_engine_version,
                                        "account_key": account_key,
                                        "direction": direction,
                                        "amount": amount_bucket,
                                        "txn_count": len(window_rows),
                                        "total_amount": total_amount,
                                        "window_minutes": int(repeated_window.total_seconds() // 60),
                                        "min_count": repeated_min_count,
                                        "min_total_amount": repeated_min_total,
                                        "start_time": _trim(window_rows[0].get("txn_time")),
                                        "end_time": _trim(window_rows[-1].get("txn_time")),
                                    },
                                    score=0.58 + min(len(window_rows) / 8, 1.0) * 0.18 + min(total_amount / max(fast_min_amount, 1.0), 1.0) * 0.1,
                                    severity="high" if len(window_rows) >= repeated_min_count + 3 or total_amount >= fast_strong_amount else "medium",
                                )
                            )
                            break
                        start += 1

            round_unit = max(1.0, _as_float(config.get("round_amount_unit")))
            round_min_amount = max(0.0, _as_float(config.get("round_amount_min_amount")))
            round_min_count = max(2, _as_int(config.get("round_amount_min_count")))
            round_min_total = max(0.0, _as_float(config.get("round_amount_min_total_amount")))
            if native_patterns_ready:
                for feature in native_account_patterns.get("ROUND_AMOUNT_PATTERN", []):
                    direction = _trim(feature.get("direction")) or "unknown"
                    total_amount = round(_as_float(feature.get("total_amount")), 2)
                    txn_count = _as_int(feature.get("txn_count"))
                    txn_ids = _json_loads(feature.get("txn_ids_json"), [])
                    amounts = _json_loads(feature.get("amounts_json"), [])
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="ROUND_AMOUNT_PATTERN",
                            entity_ids=[_build_entity_id("account", account_key)],
                            txn_ids=txn_ids,
                            summary=(
                                f"账户 {account_key} 出现 {txn_count} 笔{_direction_label(direction)}整数金额交易，"
                                f"累计 {format(total_amount, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "direction": direction,
                                "round_unit": round(_as_float(feature.get("round_unit")) or round_unit, 2),
                                "min_amount": round(_as_float(feature.get("min_amount")) or round_min_amount, 2),
                                "txn_count": txn_count,
                                "total_amount": total_amount,
                                "amounts": amounts,
                            },
                            score=0.54 + min(txn_count / 10, 1.0) * 0.16 + min(total_amount / max(fast_strong_amount, 1.0), 1.0) * 0.1,
                            severity="high" if total_amount >= fast_strong_amount and txn_count >= round_min_count + 2 else "medium",
                        )
                    )
            else:
                round_rows = [
                    row
                    for row in ordered
                    if row.get("txn_ts") is not None
                    and row.get("direction_norm") in {"in", "out"}
                    and _as_float(row.get("amount_val")) >= round_min_amount
                    and abs(_as_float(row.get("amount_val")) % round_unit) < 0.01
                ]
                round_by_direction: dict[str, list[dict[str, Any]]] = defaultdict(list)
                for row in round_rows:
                    round_by_direction[row.get("direction_norm") or "unknown"].append(row)
                for direction, direction_rows in round_by_direction.items():
                    total_amount = round(sum(_as_float(row.get("amount_val")) for row in direction_rows), 2)
                    if len(direction_rows) < round_min_count or total_amount < round_min_total:
                        continue
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="ROUND_AMOUNT_PATTERN",
                            entity_ids=[direction_rows[0].get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in direction_rows[:20]],
                            summary=(
                                f"账户 {account_key} 出现 {len(direction_rows)} 笔{_direction_label(direction)}整数金额交易，"
                                f"累计 {format(total_amount, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "direction": direction,
                                "round_unit": round_unit,
                                "min_amount": round_min_amount,
                                "txn_count": len(direction_rows),
                                "total_amount": total_amount,
                                "amounts": sorted({_as_float(row.get("amount_val")) for row in direction_rows})[:20],
                            },
                            score=0.54 + min(len(direction_rows) / 10, 1.0) * 0.16 + min(total_amount / max(fast_strong_amount, 1.0), 1.0) * 0.1,
                            severity="high" if total_amount >= fast_strong_amount and len(direction_rows) >= round_min_count + 2 else "medium",
                        )
                    )

            night_min_count = max(2, _as_int(config.get("night_activity_min_count")))
            night_min_total = max(0.0, _as_float(config.get("night_activity_min_total_amount")))
            if native_patterns_ready:
                for feature in native_account_patterns.get("NIGHT_OFFHOUR_ACTIVITY", []):
                    night_total = round(_as_float(feature.get("total_amount")), 2)
                    txn_count = _as_int(feature.get("txn_count"))
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="NIGHT_OFFHOUR_ACTIVITY",
                            entity_ids=[_build_entity_id("account", account_key)],
                            txn_ids=_json_loads(feature.get("txn_ids_json"), []),
                            summary=(
                                f"账户 {account_key} 出现 {txn_count} 笔夜间/非营业时间交易，"
                                f"累计 {format(night_total, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "start_hour": max(0, min(23, _as_int(config.get("night_activity_start_hour")))),
                                "end_hour": max(0, min(23, _as_int(config.get("night_activity_end_hour")))),
                                "txn_count": txn_count,
                                "total_amount": night_total,
                                "min_count": night_min_count,
                                "min_total_amount": night_min_total,
                                "first_time": _trim(feature.get("first_time")),
                                "last_time": _trim(feature.get("last_time")),
                            },
                            score=0.56 + min(txn_count / 12, 1.0) * 0.16 + min(night_total / max(fast_min_amount, 1.0), 1.0) * 0.1,
                            severity="high" if night_total >= fast_strong_amount or txn_count >= night_min_count + 6 else "medium",
                        )
                    )
            else:
                night_rows = [
                    row
                    for row in ordered
                    if isinstance(row.get("txn_ts"), datetime)
                    and row.get("direction_norm") in {"in", "out"}
                    and _is_night_or_offhour(row["txn_ts"])
                ]
                night_total = round(sum(_as_float(row.get("amount_val")) for row in night_rows), 2)
                if len(night_rows) >= night_min_count and night_total >= night_min_total:
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="NIGHT_OFFHOUR_ACTIVITY",
                            entity_ids=[night_rows[0].get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in night_rows[:20]],
                            summary=(
                                f"账户 {account_key} 出现 {len(night_rows)} 笔夜间/非营业时间交易，"
                                f"累计 {format(night_total, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "start_hour": max(0, min(23, _as_int(config.get("night_activity_start_hour")))),
                                "end_hour": max(0, min(23, _as_int(config.get("night_activity_end_hour")))),
                                "txn_count": len(night_rows),
                                "total_amount": night_total,
                                "min_count": night_min_count,
                                "min_total_amount": night_min_total,
                                "first_time": _trim(night_rows[0].get("txn_time")),
                                "last_time": _trim(night_rows[-1].get("txn_time")),
                            },
                            score=0.56 + min(len(night_rows) / 12, 1.0) * 0.16 + min(night_total / max(fast_min_amount, 1.0), 1.0) * 0.1,
                            severity="high" if night_total >= fast_strong_amount or len(night_rows) >= night_min_count + 6 else "medium",
                        )
                    )

            same_name_rows = [
                row
                for row in ordered
                if row.get("direction_norm") in {"in", "out"} and _same_name_counterparty(row)
            ]
            same_name_min_count = max(2, _as_int(config.get("same_name_transfer_min_count")))
            same_name_min_total = max(0.0, _as_float(config.get("same_name_transfer_min_total_amount")))
            same_name_total = round(sum(_as_float(row.get("amount_val")) for row in same_name_rows), 2)
            if len(same_name_rows) >= same_name_min_count and same_name_total >= same_name_min_total:
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="SAME_NAME_TRANSFER_CLUSTER",
                        entity_ids=[same_name_rows[0].get("account_entity_id")],
                        txn_ids=[row.get("txn_id") for row in same_name_rows[:20]],
                        summary=(
                            f"账户 {account_key} 与同名对手发生 {len(same_name_rows)} 笔往来，"
                            f"合计 {format(same_name_total, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "account_name": _trim(same_name_rows[0].get("account_name")),
                            "txn_count": len(same_name_rows),
                            "total_amount": same_name_total,
                            "counterparty_accounts": _unique_texts(row.get("counterparty_acct") for row in same_name_rows)[:12],
                        },
                        score=0.6 + min(len(same_name_rows) / 8, 1.0) * 0.14 + min(same_name_total / max(fast_min_amount, 1.0), 1.0) * 0.12,
                        severity="high" if same_name_total >= fast_strong_amount else "medium",
                    )
                )

            fan_window = timedelta(minutes=max(1, _as_int(config.get("fan_in_out_window_minutes"))))
            fan_min_unique_in = max(2, _as_int(config.get("fan_in_out_min_unique_in")))
            fan_min_unique_out = max(2, _as_int(config.get("fan_in_out_min_unique_out")))
            fan_min_total = max(0.0, _as_float(config.get("fan_in_out_min_total_amount")))
            window_start = 0
            while window_start < len(ordered):
                anchor = ordered[window_start]
                if anchor.get("txn_ts") is None:
                    window_start += 1
                    continue
                window_end = window_start
                while window_end < len(ordered) and ordered[window_end]["txn_ts"] <= anchor["txn_ts"] + fan_window:
                    window_end += 1
                window_rows = ordered[window_start:window_end]
                in_rows = [row for row in window_rows if row.get("direction_norm") == "in"]
                out_rows_window = [row for row in window_rows if row.get("direction_norm") == "out"]
                unique_in = _unique_texts(row.get("counterparty_key") or row.get("counterparty_acct") for row in in_rows)
                unique_out = _unique_texts(row.get("counterparty_key") or row.get("counterparty_acct") for row in out_rows_window)
                total_in = round(sum(_as_float(row.get("amount_val")) for row in in_rows), 2)
                total_out = round(sum(_as_float(row.get("amount_val")) for row in out_rows_window), 2)
                if (
                    len(unique_in) >= fan_min_unique_in
                    and len(unique_out) >= fan_min_unique_out
                    and min(total_in, total_out) >= fan_min_total
                ):
                    fan_rows = [*in_rows[:20], *out_rows_window[:20]]
                    fan_rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, item.get("txn_id") or ""))
                    balance_ratio = min(total_in, total_out) / max(max(total_in, total_out), 0.01)
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="FAN_IN_FAN_OUT",
                            entity_ids=[anchor.get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in fan_rows],
                            summary=(
                                f"账户 {account_key} 在 {int(fan_window.total_seconds() // 60)} 分钟内呈现"
                                f"{len(unique_in)} 个来源流入、{len(unique_out)} 个去向流出，"
                                f"流入 {format(total_in, '.2f')} 元、流出 {format(total_out, '.2f')} 元。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "window_minutes": int(fan_window.total_seconds() // 60),
                                "unique_in_counterparty_count": len(unique_in),
                                "unique_out_counterparty_count": len(unique_out),
                                "total_in_amount": total_in,
                                "total_out_amount": total_out,
                                "balance_ratio": round(balance_ratio, 4),
                                "start_time": _trim(window_rows[0].get("txn_time")),
                                "end_time": _trim(window_rows[-1].get("txn_time")),
                            },
                            score=0.68 + min((len(unique_in) + len(unique_out)) / 12, 1.0) * 0.14 + min(balance_ratio, 1.0) * 0.12,
                            severity="high",
                        )
                    )
                    break
                window_start += 1

            asset_rows = [
                row for row in ordered
                if row.get("direction_norm") == "out"
                and _as_float(row.get("amount_val")) >= max(0.0, _as_float(config.get("asset_purchase_min_amount")))
                and _row_text_contains(row, _ASSET_PURCHASE_MARKERS)
            ]
            if asset_rows:
                asset_total = round(sum(_as_float(row.get("amount_val")) for row in asset_rows), 2)
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="ASSET_PURCHASE_OUTFLOW",
                        entity_ids=[asset_rows[0].get("account_entity_id")],
                        txn_ids=[row.get("txn_id") for row in asset_rows[:20]],
                        summary=(
                            f"账户 {account_key} 出现 {len(asset_rows)} 笔房产/车辆等大额资产购置语义出账，"
                            f"累计 {format(asset_total, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "txn_count": len(asset_rows),
                            "total_amount": asset_total,
                            "min_amount": max(0.0, _as_float(config.get("asset_purchase_min_amount"))),
                            "markers": list(_ASSET_PURCHASE_MARKERS),
                        },
                        score=0.6 + min(asset_total / max(fast_strong_amount, 1.0), 1.0) * 0.18,
                        severity="high" if asset_total >= fast_strong_amount else "medium",
                    )
                )

            high_consumption_min_amount = max(0.0, _as_float(config.get("high_consumption_min_amount")))
            high_consumption_window = timedelta(minutes=max(1, _as_int(config.get("high_consumption_window_minutes"))))
            high_consumption_min_total = max(0.0, _as_float(config.get("high_consumption_min_total_amount")))
            consumption_rows = [
                row for row in ordered
                if row.get("txn_ts") is not None
                and row.get("direction_norm") == "out"
                and _as_float(row.get("amount_val")) >= high_consumption_min_amount
                and _row_text_contains(row, _HIGH_CONSUMPTION_MARKERS)
            ]
            if consumption_rows:
                start = 0
                while start < len(consumption_rows):
                    end = start
                    while end < len(consumption_rows) and consumption_rows[end]["txn_ts"] <= consumption_rows[start]["txn_ts"] + high_consumption_window:
                        end += 1
                    window_rows = consumption_rows[start:end]
                    total_amount = round(sum(_as_float(row.get("amount_val")) for row in window_rows), 2)
                    if total_amount >= high_consumption_min_total:
                        hits.append(
                            self._build_rule_hit(
                                case_id,
                                rule_code="HIGH_CONSUMPTION_LIFESTYLE",
                                entity_ids=[window_rows[0].get("account_entity_id")],
                                txn_ids=[row.get("txn_id") for row in window_rows[:20]],
                                summary=(
                                    f"账户 {account_key} 出现 {len(window_rows)} 笔高消费语义出账，"
                                    f"累计 {format(total_amount, '.2f')} 元。"
                                ),
                                detail_json={
                                    "rule_engine_version": rule_engine_version,
                                    "account_key": account_key,
                                    "txn_count": len(window_rows),
                                    "total_amount": total_amount,
                                    "window_minutes": int(high_consumption_window.total_seconds() // 60),
                                    "min_amount": high_consumption_min_amount,
                                    "min_total_amount": high_consumption_min_total,
                                    "markers": list(_HIGH_CONSUMPTION_MARKERS),
                                },
                                score=0.55 + min(total_amount / max(fast_strong_amount, 1.0), 1.0) * 0.14,
                                severity="high" if total_amount >= fast_strong_amount else "medium",
                            )
                        )
                        break
                    start += 1

            property_rows = [
                row for row in ordered
                if row.get("direction_norm") == "out"
                and _row_text_contains(row, _PROPERTY_PARKING_MARKERS)
            ]
            property_min_count = max(1, _as_int(config.get("property_parking_min_count")))
            property_min_total = max(0.0, _as_float(config.get("property_parking_min_total_amount")))
            property_total = round(sum(_as_float(row.get("amount_val")) for row in property_rows), 2)
            if len(property_rows) >= property_min_count and property_total >= property_min_total:
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="PROPERTY_PARKING_PAYMENT",
                        entity_ids=[property_rows[0].get("account_entity_id")],
                        txn_ids=[row.get("txn_id") for row in property_rows[:20]],
                        summary=(
                            f"账户 {account_key} 出现 {len(property_rows)} 笔物业/停车/车位语义支付，"
                            f"累计 {format(property_total, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "account_key": account_key,
                            "txn_count": len(property_rows),
                            "total_amount": property_total,
                            "min_count": property_min_count,
                            "min_total_amount": property_min_total,
                            "markers": list(_PROPERTY_PARKING_MARKERS),
                        },
                        score=0.42 + min(property_total / max(fast_min_amount, 1.0), 1.0) * 0.12,
                        severity="low",
                    )
                )

            threshold_window = timedelta(minutes=max(1, _as_int(config.get("high_freq_window_minutes"))))
            threshold = max(0.0, _as_float(config.get("threshold_split_amount")))
            tolerance_rate = max(0.0, _as_float(config.get("threshold_split_tolerance_rate")))
            threshold_split_min_count = max(2, _as_int(config.get("threshold_split_min_count")))
            lower = threshold * (1 - tolerance_rate)
            upper = threshold * (1 + tolerance_rate)
            if native_patterns_ready and "THRESHOLD_SPLIT" in native_account_patterns:
                for feature in native_account_patterns.get("THRESHOLD_SPLIT", []):
                    window_rows = _native_feature_txn_rows(feature)
                    if len(window_rows) < threshold_split_min_count:
                        continue
                    direction = _trim(feature.get("direction")) or "unknown"
                    amounts = [_as_float(row.get("amount_val")) for row in window_rows]
                    avg_gap = sum(abs(amount - threshold) for amount in amounts) / max(len(amounts), 1)
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="THRESHOLD_SPLIT",
                            entity_ids=[window_rows[0].get("account_entity_id")],
                            txn_ids=[row.get("txn_id") for row in window_rows],
                            summary=(
                                f"账户 {account_key} 在 {int(threshold_window.total_seconds() // 60)} 分钟内连续 {len(window_rows)} 笔{_direction_label(direction)}"
                                f"金额落在 {format(lower, '.0f')}-{format(upper, '.0f')} 元敏感区间。"
                            ),
                            detail_json={
                                "rule_engine_version": rule_engine_version,
                                "account_key": account_key,
                                "direction": direction,
                                "window_minutes": int(threshold_window.total_seconds() // 60),
                                "txn_count": len(window_rows),
                                "threshold": threshold,
                                "tolerance_rate": tolerance_rate,
                                "min_count": threshold_split_min_count,
                                "amount_min": round(min(amounts), 2),
                                "amount_max": round(max(amounts), 2),
                                "start_time": _trim(window_rows[0].get("txn_time")),
                                "end_time": _trim(window_rows[-1].get("txn_time")),
                            },
                            score=0.74 + min(len(window_rows) / 5, 1.1) * 0.14 + max(0.0, 1 - (avg_gap / max(threshold * tolerance_rate, 1.0))) * 0.08,
                            severity="high",
                        )
                    )
            else:
                split_candidates = [
                    row
                    for row in ordered
                    if row.get("txn_ts") is not None and lower <= _as_float(row.get("amount_val")) <= upper and row.get("direction_norm") in {"in", "out"}
                ]
                split_by_direction: dict[str, list[dict[str, Any]]] = defaultdict(list)
                for row in split_candidates:
                    split_by_direction[row.get("direction_norm") or "unknown"].append(row)
                for direction, direction_rows in split_by_direction.items():
                    start = 0
                    while start < len(direction_rows):
                        end = start
                        while end < len(direction_rows) and direction_rows[end]["txn_ts"] <= direction_rows[start]["txn_ts"] + threshold_window:
                            end += 1
                        window_rows = direction_rows[start:end]
                        if len(window_rows) >= threshold_split_min_count:
                            amounts = [_as_float(row.get("amount_val")) for row in window_rows]
                            avg_gap = sum(abs(amount - threshold) for amount in amounts) / max(len(amounts), 1)
                            hits.append(
                                self._build_rule_hit(
                                    case_id,
                                    rule_code="THRESHOLD_SPLIT",
                                    entity_ids=[window_rows[0].get("account_entity_id")],
                                    txn_ids=[row.get("txn_id") for row in window_rows],
                                    summary=(
                                        f"账户 {account_key} 在 {int(threshold_window.total_seconds() // 60)} 分钟内连续 {len(window_rows)} 笔{_direction_label(direction)}"
                                        f"金额落在 {format(lower, '.0f')}-{format(upper, '.0f')} 元敏感区间。"
                                    ),
                                    detail_json={
                                        "rule_engine_version": rule_engine_version,
                                        "account_key": account_key,
                                        "direction": direction,
                                        "window_minutes": int(threshold_window.total_seconds() // 60),
                                        "txn_count": len(window_rows),
                                        "threshold": threshold,
                                        "tolerance_rate": tolerance_rate,
                                        "min_count": threshold_split_min_count,
                                        "amount_min": round(min(amounts), 2),
                                        "amount_max": round(max(amounts), 2),
                                        "start_time": _trim(window_rows[0].get("txn_time")),
                                        "end_time": _trim(window_rows[-1].get("txn_time")),
                                    },
                                    score=0.74 + min(len(window_rows) / 5, 1.1) * 0.14 + max(0.0, 1 - (avg_gap / max(threshold * tolerance_rate, 1.0))) * 0.08,
                                    severity="high",
                                )
                            )
                            start = end
                            continue
                        start += 1

            counterparty_rows = [row for row in ordered if _trim(row.get("counterparty_key"))]
            counterparty_stats: dict[str, dict[str, Any]] = {}
            for row in counterparty_rows:
                key = _trim(row.get("counterparty_key"))
                bucket = counterparty_stats.setdefault(
                    key,
                    {"amount": 0.0, "txn_count": 0, "txn_ids": [], "display_name": _trim(row.get("counterparty_name")) or _trim(row.get("counterparty_acct")) or key},
                )
                bucket["amount"] += _as_float(row.get("amount_val"))
                bucket["txn_count"] += 1
                bucket["txn_ids"].append(_trim(row.get("txn_id")))
            total_counterparty_amount = round(sum(item["amount"] for item in counterparty_stats.values()), 2)
            ranked_counterparties = sorted(counterparty_stats.items(), key=lambda item: (-item[1]["amount"], -item[1]["txn_count"], item[0]))
            top_three = ranked_counterparties[:3]
            top_three_amount = round(sum(item[1]["amount"] for item in top_three), 2)
            top_three_ratio = top_three_amount / max(total_counterparty_amount, 0.01)
            counterparty_top3_ratio = max(0.0, _as_float(config.get("counterparty_top3_ratio")))
            if len(counterparty_stats) >= 3 and len(counterparty_rows) >= 6 and top_three_ratio >= counterparty_top3_ratio:
                severity = "high" if top_three_ratio >= 0.85 else "medium"
                top_counterparties = [
                    {
                        "counterparty_key": key,
                        "display_name": stats["display_name"],
                        "amount": round(stats["amount"], 2),
                        "txn_count": int(stats["txn_count"]),
                    }
                    for key, stats in top_three
                ]
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="COUNTERPARTY_CONCENTRATION",
                        entity_ids=[ordered[0].get("account_entity_id")],
                        txn_ids=[txn_id for _, stats in top_three for txn_id in stats["txn_ids"]],
                        summary=(
                            f"账户 {account_key} 的前 3 个主要对手占交易金额 {format(top_three_ratio * 100, '.1f')}%，"
                            f"集中度明显偏高。"
                        ),
                        detail_json={
                            "account_key": account_key,
                            "unique_counterparty_count": len(counterparty_stats),
                            "top3_ratio": round(top_three_ratio, 4),
                            "total_amount": total_counterparty_amount,
                            "top_counterparties": top_counterparties,
                        },
                        score=0.6 + min(top_three_ratio, 1.0) * 0.25 + min(len(counterparty_rows) / 12, 1.0) * 0.08,
                        severity=severity,
                    )
                )

            retry_rows = [row for row in ordered if row.get("txn_ts") is not None and row.get("success_flag_norm") is not None]
            retry_window = timedelta(minutes=max(1, _as_int(config.get("failed_retry_window_minutes"))))
            retry_count_threshold = max(2, _as_int(config.get("failed_retry_count_threshold")))
            index = 0
            while index < len(retry_rows):
                anchor = retry_rows[index]
                if anchor.get("success_flag_norm") is not False:
                    index += 1
                    continue
                failures: list[dict[str, Any]] = []
                success_row: Optional[dict[str, Any]] = None
                anchor_reason = _trim(anchor.get("query_feedback_reason_norm"))
                anchor_device = _trim(anchor.get("device_key"))
                scan = index
                while scan < len(retry_rows) and retry_rows[scan]["txn_ts"] <= anchor["txn_ts"] + retry_window:
                    candidate = retry_rows[scan]
                    candidate_device = _trim(candidate.get("device_key"))
                    candidate_reason = _trim(candidate.get("query_feedback_reason_norm"))
                    same_device = not anchor_device or not candidate_device or candidate_device == anchor_device
                    same_reason = not anchor_reason or not candidate_reason or candidate_reason == anchor_reason
                    if not same_device or not same_reason:
                        scan += 1
                        continue
                    if candidate.get("success_flag_norm") is False:
                        failures.append(candidate)
                    elif candidate.get("success_flag_norm") is True and len(failures) >= retry_count_threshold:
                        success_row = candidate
                        break
                    scan += 1
                if success_row and len(failures) >= retry_count_threshold:
                    entity_ids = [anchor.get("account_entity_id")]
                    if anchor.get("device_entity_id"):
                        entity_ids.append(anchor.get("device_entity_id"))
                    severity = "high"
                    hits.append(
                        self._build_rule_hit(
                            case_id,
                            rule_code="FAILED_RETRY_PROBE",
                            entity_ids=entity_ids,
                            txn_ids=[row.get("txn_id") for row in [*failures, success_row]],
                            summary=(
                                f"账户 {account_key} 在 {int(retry_window.total_seconds() // 60)} 分钟内连续失败 {len(failures)} 次后出现成功交易，"
                                f"疑似存在试探或撞库行为。"
                            ),
                            detail_json={
                                "account_key": account_key,
                                "failed_count": len(failures),
                                "window_minutes": int(retry_window.total_seconds() // 60),
                                "reason": anchor_reason,
                                "device_key": anchor_device,
                                "failed_txn_ids": [row.get("txn_id") for row in failures],
                                "success_txn_id": success_row.get("txn_id"),
                                "success_time": _trim(success_row.get("txn_time")),
                            },
                            score=0.75 + min(len(failures) / 5, 1.0) * 0.14,
                            severity=severity,
                        )
                    )
                    index = scan + 1
                    continue
                index += 1

        for field_name, rule_code, threshold in (
            ("ip_key", "SHARED_IP_MULTI_ACCOUNT", max(2, _as_int(config.get("shared_ip_account_threshold")))),
            ("mac_key", "SHARED_MAC_MULTI_ACCOUNT", max(2, _as_int(config.get("shared_mac_account_threshold")))),
        ):
            grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
            for row in txn_rows:
                group_key = _trim(row.get(field_name))
                if group_key:
                    grouped[group_key].append(row)
            for group_key, rows in grouped.items():
                unique_accounts = _unique_texts(row.get("account_key") for row in rows)
                unique_persons = _unique_texts(row.get("opener_id_no") for row in rows)
                if len(unique_accounts) < threshold and len(unique_persons) < threshold:
                    continue
                label = "IP" if field_name == "ip_key" else "MAC"
                raw_value = _trim(rows[0].get("ip_addr" if field_name == "ip_key" else "mac_addr")) or group_key
                severity = "high" if len(unique_accounts) >= threshold + 2 or len(unique_persons) >= threshold else RULE_DEFINITIONS[rule_code]["severity"]
                device_entity_id = _build_entity_id("device", raw_value)
                entity_ids = [device_entity_id]
                entity_ids.extend(_build_entity_id("account", account) for account in unique_accounts[:8])
                entity_ids.extend(_build_entity_id("person", person) for person in unique_persons[:4])
                txn_ids = [row.get("txn_id") for row in rows[:12]]
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code=rule_code,
                        entity_ids=entity_ids,
                        txn_ids=txn_ids,
                        summary=(
                            f"同一 {label} {raw_value} 关联 {len(unique_accounts)} 个账户"
                            f"{f'、{len(unique_persons)} 个开户人' if unique_persons else ''}。"
                        ),
                        detail_json={
                            "device_value": raw_value,
                            "device_label": label,
                            "unique_account_count": len(unique_accounts),
                            "unique_accounts": unique_accounts[:12],
                            "unique_person_count": len(unique_persons),
                            "unique_persons": unique_persons[:12],
                            "txn_count": len(rows),
                            "first_seen_at": _trim(rows[0].get("txn_time")),
                            "last_seen_at": _trim(rows[-1].get("txn_time")),
                        },
                        score=0.55 + min(len(unique_accounts) / max(threshold + 2, 1), 1.2) * 0.2 + min(len(rows) / 10, 1.0) * 0.08,
                        severity=severity,
                    )
                )

        teller_threshold = max(2, _as_int(config.get("shared_teller_account_threshold")))
        teller_groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in txn_rows:
            teller_key = _trim(row.get("teller_no"))
            branch_key = _trim(row.get("branch_name"))
            if not teller_key:
                continue
            group_key = "|".join(_unique_texts((branch_key, teller_key))) or teller_key
            teller_groups[group_key].append(row)
        for group_key, rows in teller_groups.items():
            unique_accounts = _unique_texts(row.get("account_key") for row in rows)
            if len(unique_accounts) < teller_threshold:
                continue
            total_amount = round(sum(_as_float(row.get("amount_val")) for row in rows), 2)
            raw_teller = _trim(rows[0].get("teller_no")) or group_key
            raw_branch = _trim(rows[0].get("branch_name"))
            entity_ids = [_build_entity_id("teller", raw_teller)]
            if raw_branch:
                entity_ids.append(_build_entity_id("branch", raw_branch))
            entity_ids.extend(_build_entity_id("account", account) for account in unique_accounts[:8])
            hits.append(
                self._build_rule_hit(
                    case_id,
                    rule_code="SHARED_TELLER_MULTI_ACCOUNT",
                    entity_ids=entity_ids,
                    txn_ids=[row.get("txn_id") for row in rows[:20]],
                    summary=(
                        f"柜员/柜面 {raw_teller} 关联 {len(unique_accounts)} 个账户交易，"
                        f"累计 {format(total_amount, '.2f')} 元。"
                    ),
                    detail_json={
                        "rule_engine_version": rule_engine_version,
                        "teller_no": raw_teller,
                        "branch_name": raw_branch,
                        "unique_account_count": len(unique_accounts),
                        "unique_accounts": unique_accounts[:20],
                        "txn_count": len(rows),
                        "total_amount": total_amount,
                    },
                    score=0.52 + min(len(unique_accounts) / max(teller_threshold + 3, 1), 1.0) * 0.18 + min(total_amount / max(fast_strong_amount, 1.0), 1.0) * 0.08,
                    severity="high" if len(unique_accounts) >= teller_threshold + 3 or total_amount >= fast_strong_amount else "medium",
                )
            )

        circular_min_amount = max(0.0, _as_float(config.get("circular_flow_min_amount")))
        circular_window = timedelta(minutes=max(1, _as_int(config.get("circular_flow_window_minutes"))))
        circular_tolerance = max(0.0, _as_float(config.get("circular_flow_tolerance_rate")))
        outgoing_rows = [
            row for row in txn_rows
            if row.get("direction_norm") == "out"
            and row.get("txn_ts") is not None
            and _as_float(row.get("amount_val")) >= circular_min_amount
            and _trim(row.get("account_key"))
            and _trim(row.get("counterparty_acct"))
        ]
        outgoing_by_pair: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
        for row in outgoing_rows:
            outgoing_by_pair[(_trim(row.get("account_key")), _trim(row.get("counterparty_acct")))].append(row)
        circular_pairs_seen: set[tuple[str, str]] = set()
        for (source_account, target_account), source_rows in outgoing_by_pair.items():
            reverse_key = (target_account, source_account)
            reverse_rows = outgoing_by_pair.get(reverse_key) or []
            if not reverse_rows:
                continue
            pair_key = tuple(sorted((source_account, target_account)))
            if pair_key in circular_pairs_seen:
                continue
            for source_row in source_rows:
                source_ts = source_row.get("txn_ts")
                source_amount = _as_float(source_row.get("amount_val"))
                matched_reverse = next(
                    (
                        row
                        for row in reverse_rows
                        if row.get("txn_ts") is not None
                        and source_ts < row["txn_ts"] <= source_ts + circular_window
                        and abs(_as_float(row.get("amount_val")) - source_amount) <= max(source_amount * circular_tolerance, 1.0)
                    ),
                    None,
                )
                if matched_reverse is None:
                    continue
                reverse_amount = _as_float(matched_reverse.get("amount_val"))
                circular_pairs_seen.add(pair_key)
                hits.append(
                    self._build_rule_hit(
                        case_id,
                        rule_code="CIRCULAR_FLOW_RETURN",
                        entity_ids=[
                            _build_entity_id("account", source_account),
                            _build_entity_id("account", target_account),
                        ],
                        txn_ids=[source_row.get("txn_id"), matched_reverse.get("txn_id")],
                        summary=(
                            f"账户 {source_account} 与 {target_account} 在 {int(circular_window.total_seconds() // 60)} 分钟内"
                            f"出现近似金额双向回流：{format(source_amount, '.2f')} 元 -> {format(reverse_amount, '.2f')} 元。"
                        ),
                        detail_json={
                            "rule_engine_version": rule_engine_version,
                            "source_account": source_account,
                            "target_account": target_account,
                            "source_amount": round(source_amount, 2),
                            "reverse_amount": round(reverse_amount, 2),
                            "amount_gap": round(abs(reverse_amount - source_amount), 2),
                            "tolerance_rate": circular_tolerance,
                            "window_minutes": int(circular_window.total_seconds() // 60),
                            "source_time": _trim(source_row.get("txn_time")),
                            "reverse_time": _trim(matched_reverse.get("txn_time")),
                        },
                        score=0.72 + min(source_amount / max(fast_strong_amount, 1.0), 1.0) * 0.12,
                        severity="high",
                    )
                )
                break

        deduped: dict[str, dict[str, Any]] = {}
        for hit in hits:
            rule_code = _trim(hit.get("rule_code")).upper()
            if rule_code in _CASH_DEPENDENT_RULE_CODES and not cash_rules_ready:
                continue
            detail = dict(hit.get("detail_json") or {})
            detail["feature_input_readiness"] = {
                "cash_classification": (
                    cash_input_coverage
                    if rule_code in _CASH_DEPENDENT_RULE_CODES
                    else {"status": "not_required"}
                )
            }
            hit = {**hit, "detail_json": detail}
            current = deduped.get(hit["rule_hit_id"])
            if current is None or _as_float(hit.get("score")) > _as_float(current.get("score")):
                deduped[hit["rule_hit_id"]] = hit
        ordered_hits = sorted(
            deduped.values(),
            key=lambda item: (-_as_float(item.get("score")), _trim(item.get("severity")), _trim(item.get("rule_code"))),
        )
        return ordered_hits

    def _txn_evidence_payload(self, row: dict[str, Any]) -> tuple[str, str, dict[str, Any]]:
        title = f"交易证据 {_trim(row.get('txn_id'))}"
        amount = _optional_rounded_float(row.get("amount_val"), 2)
        balance = _optional_rounded_float(row.get("balance_val"), 2)
        if amount is None:
            snippet = "交易金额未核验；该记录不能支持金额、余额或资金流量事实。"
        else:
            snippet = (
                f"账户 {_trim(row.get('account_key'))} 于 {_trim(row.get('txn_time')) or '未知时间'} "
                f"{_direction_label(_trim(row.get('direction_norm')))} {format(amount, '.2f')} 元。"
            )
        payload = {
            "txn_id": _trim(row.get("txn_id")),
            "account_key": _trim(row.get("account_key")),
            "account_name": _trim(row.get("account_name")),
            "txn_time": _trim(row.get("txn_time")),
            "amount": amount,
            "balance": balance,
            "amount_status": "verified_value" if amount is not None else "unresolved",
            "balance_status": "verified_value" if balance is not None else "unresolved",
            "direction": _trim(row.get("direction_norm")),
            "currency": _trim(row.get("currency")) or None,
            "counterparty_acct": _trim(row.get("counterparty_acct")),
            "counterparty_name": _trim(row.get("counterparty_name")),
            "counterparty_bank": _trim(row.get("counterparty_bank")),
            "ip_addr": _trim(row.get("ip_addr")),
            "mac_addr": _trim(row.get("mac_addr")),
        }
        return title, snippet, payload

    def _persist_rule_hits(
        self,
        engine: DuckDBEngine,
        case_id: str,
        txn_rows: Sequence[dict[str, Any]],
        hits: Sequence[dict[str, Any]],
    ) -> list[dict[str, Any]]:
        self.ensure_analysis_schema(engine)
        self._rebuild_rule_hit_without_case(engine, case_id)
        self._rebuild_evidence_ref_without_rule_hit_refs(engine, case_id)
        txn_index = {_trim(row.get("txn_id")): row for row in txn_rows if _trim(row.get("txn_id"))}
        created_at = _now_text()
        persisted: list[dict[str, Any]] = []
        for hit in hits:
            score = _optional_finite_float(hit.get("score"))
            if score is None:
                continue
            txn_evidence_ids: list[str] = []
            referenced_txn_ids = _unique_texts(_clip_items(hit.get("txn_ids", []), 12))
            for txn_id in referenced_txn_ids:
                row = txn_index.get(txn_id)
                if row is None:
                    continue
                title, snippet, payload = self._txn_evidence_payload(row)
                evidence_id = self._upsert_evidence_ref(
                    engine,
                    case_id=case_id,
                    evidence_type="txn",
                    ref_table="fc_transaction_norm",
                    ref_pk=txn_id,
                    title=title,
                    snippet=snippet,
                    payload=payload,
                )
                if evidence_id:
                    txn_evidence_ids.append(evidence_id)
            if referenced_txn_ids and len(txn_evidence_ids) != len(referenced_txn_ids):
                continue
            engine.execute(
                """
                INSERT INTO analysis_rule_hit(
                    rule_hit_id, case_id, rule_code, risk_type, severity, score,
                    entity_ids_json, txn_ids_json, evidence_ids_json, summary, detail_json, created_at
                ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
                """,
                (
                    hit["rule_hit_id"],
                    case_id,
                    _trim(hit.get("rule_code")),
                    _trim(hit.get("risk_type")),
                    _trim(hit.get("severity")),
                    score,
                    _json_dumps(hit.get("entity_ids", [])),
                    _json_dumps(hit.get("txn_ids", [])),
                    "[]",
                    _trim(hit.get("summary")),
                    _json_dumps(hit.get("detail_json", {})),
                    created_at,
                ),
            )
            rule_evidence_id = self._upsert_evidence_ref(
                engine,
                case_id=case_id,
                evidence_type="rule_hit",
                ref_table="analysis_rule_hit",
                ref_pk=hit["rule_hit_id"],
                title=_trim(hit.get("title")) or _trim(hit.get("rule_code")),
                snippet=_trim(hit.get("summary"))[:240],
                payload={
                    "rule_hit_id": hit["rule_hit_id"],
                    "rule_code": _trim(hit.get("rule_code")),
                    "risk_type": _trim(hit.get("risk_type")),
                    "severity": _trim(hit.get("severity")),
                    "score": round(score, 2),
                    "entity_ids": hit.get("entity_ids", []),
                    "txn_ids": hit.get("txn_ids", []),
                    "detail_json": hit.get("detail_json", {}),
                },
            )
            evidence_ids = _unique_texts([*txn_evidence_ids, rule_evidence_id])
            engine.execute(
                "UPDATE analysis_rule_hit SET evidence_ids_json=? WHERE rule_hit_id=?",
                (_json_dumps(evidence_ids), hit["rule_hit_id"]),
            )
            persisted.append(
                {
                    **hit,
                    "evidence_ids": evidence_ids,
                    "created_at": created_at,
                }
            )
        return persisted

    def _hydrate_rule_hit_rows(self, rows: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        for row in rows:
            rule_code = _trim(row.get("rule_code"))
            definition = RULE_DEFINITIONS.get(rule_code, {})
            score = _optional_rounded_float(row.get("score"), 2)
            if score is None:
                continue
            items.append(
                {
                    "rule_hit_id": _trim(row.get("rule_hit_id")),
                    "rule_code": rule_code,
                    "risk_type": _trim(row.get("risk_type")) or definition.get("risk_type", ""),
                    "severity": _trim(row.get("severity")) or definition.get("severity", "medium"),
                    "score": score,
                    "title": definition.get("title", rule_code),
                    "summary": _trim(row.get("summary")),
                    "entity_ids": _json_loads(row.get("entity_ids_json"), []),
                    "txn_ids": _json_loads(row.get("txn_ids_json"), []),
                    "evidence_ids": _json_loads(row.get("evidence_ids_json"), []),
                    "detail_json": _json_loads(row.get("detail_json"), {}),
                    "created_at": _trim(row.get("created_at")),
                }
            )
        return items

    def _rule_hit_primary_amount(self, item: Mapping[str, Any]) -> float:
        detail = dict(item.get("detail_json") or {})
        values: list[float] = []
        for value in (
            item.get("out_amount"),
            detail.get("out_amount"),
            item.get("in_amount"),
            detail.get("in_amount"),
            detail.get("total_out_amount"),
            detail.get("amount_max"),
            detail.get("total_amount"),
        ):
            amount = _optional_finite_float(value)
            if amount is not None and amount > 0:
                values.append(amount)
        return max(values) if values else 0.0

    def _rule_hit_out_ratio(self, item: Mapping[str, Any]) -> float:
        detail = dict(item.get("detail_json") or {})
        ratio = _optional_finite_float(_first_known_value(detail.get("out_ratio"), item.get("out_ratio")))
        if ratio is not None and ratio > 0:
            return ratio
        in_amount = _optional_finite_float(_first_known_value(item.get("in_amount"), detail.get("in_amount")))
        out_amount = _optional_finite_float(_first_known_value(item.get("out_amount"), detail.get("out_amount")))
        if in_amount is not None and out_amount is not None and in_amount > 0 and out_amount > 0:
            return round(out_amount / in_amount, 4)
        return 0.0

    def _rule_hit_significance_sort_key(self, item: Mapping[str, Any]) -> tuple[Any, ...]:
        severity_rank = {"high": 3, "medium": 2, "low": 1}.get(_trim(item.get("severity")).lower(), 0)
        rule_priority = {
            "FAST_IN_FAST_OUT": 3,
            "FAN_IN_FAN_OUT": 3,
            "CIRCULAR_FLOW_RETURN": 3,
            "HIGH_FREQ_SMALL_OUT": 2,
            "THRESHOLD_SPLIT": 2,
            "NEAR_THRESHOLD_STRUCTURING": 2,
            "SMALL_FAST_IN_OUT": 2,
            "CASH_QUICK_IN_OUT": 2,
            "SINGLE_FAST_IN_OUT_CANDIDATE": 1,
            "SINGLE_CASH_QUICK_IN_OUT_CANDIDATE": 1,
        }.get(_trim(item.get("rule_code")).upper(), 1)
        detail = dict(item.get("detail_json") or {})
        start_time = _trim(detail.get("start_time")) or _trim(item.get("created_at"))
        txn_count = _optional_nonnegative_int(detail.get("txn_count"))
        score = _optional_finite_float(item.get("score"))
        return (
            -severity_rank,
            -rule_priority,
            -self._rule_hit_primary_amount(item),
            -self._rule_hit_out_ratio(item),
            -(txn_count if txn_count is not None else 0),
            -(score if score is not None else 0.0),
            start_time,
        )

    def _ensure_rule_hits(
        self,
        engine: DuckDBEngine,
        case_id: str,
        txn_rows: Sequence[dict[str, Any]],
        *,
        force_refresh: bool = False,
        native_pattern_features: Optional[Mapping[str, Any]] = None,
    ) -> list[dict[str, Any]]:
        del force_refresh
        if _rule_input_coverage(txn_rows).get("status") != "complete":
            return []
        params = self._load_rule_params(engine, case_id)
        return self._persist_rule_hits(
            engine,
            case_id,
            txn_rows,
            self._compute_rule_hits(
                case_id,
                txn_rows,
                params=params,
                native_pattern_features=native_pattern_features,
            ),
        )

    def get_rule_hits(
        self,
        case_id: str,
        *,
        entity_id: str = "",
        risk_type: str = "",
        severity: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        native_rule_txn_index_ready = False
        native_rule_pattern_index_ready = False
        native_rule_params: Optional[dict[str, Any]] = None
        engine = self.open_case_engine(case_id)
        owns_engine = True
        try:
            self.ensure_analysis_schema(engine)
            normalized_entity = _trim(entity_id)
            normalized_risk_type = _trim(risk_type).lower()
            normalized_severity = _trim(severity).lower()
            page_limit = max(1, min(int(limit or 20), 100))
            txn_rows = self._load_transaction_rows(
                engine,
                case_id,
                prefer_rule_txn_index=native_rule_txn_index_ready,
            )
            input_coverage = _rule_input_coverage(txn_rows)
            if input_coverage.get("status") != "complete":
                return {
                    "semantic_status": "unresolved",
                    "blocker": "rule_input_coverage_unresolved",
                    "fact_answer_allowed": False,
                    "query_id": "",
                    "items": [],
                    "stats": {
                        "total_hits": None,
                        "high": None,
                        "medium": None,
                        "low": None,
                        "rule_count": None,
                        "input_coverage": input_coverage,
                    },
                    "evidence_ids": [],
                    "cursor": "",
                    "has_more": False,
                    "truncated": False,
                    "coverage_status": _trim(input_coverage.get("status")) or "unresolved",
                    "warnings": [
                        {
                            "code": "RULE_INPUT_COVERAGE_UNRESOLVED",
                            "message": "规则输入存在缺失或不可解析事实；未知值未按 0、无异常或未命中处理。",
                            "severity": "warning",
                        }
                    ],
                }
            cash_feature_coverage = _cash_rule_input_coverage(txn_rows)
            effective_rule_params = self._load_rule_params(engine, case_id)
            native_pattern_features = (
                self._load_native_rule_pattern_features(
                    engine,
                    case_id,
                    params=native_rule_params or effective_rule_params,
                )
                if native_rule_pattern_index_ready
                else None
            )
            persisted_hits = self._ensure_rule_hits(
                engine,
                case_id,
                txn_rows,
                force_refresh=force_refresh,
                native_pattern_features=native_pattern_features,
            )
            normalized_account = self._parse_entity_account(entity_id)
            filtered_hits = []
            for item in persisted_hits:
                item_entity_ids = _unique_texts(item.get("entity_ids", []))
                if normalized_entity:
                    matched_entity = normalized_entity in item_entity_ids
                    matched_account = bool(normalized_account) and _build_entity_id("account", normalized_account) in item_entity_ids
                    if not matched_entity and not matched_account:
                        continue
                if normalized_risk_type and _trim(item.get("risk_type")).lower() != normalized_risk_type:
                    continue
                if normalized_severity and _trim(item.get("severity")).lower() != normalized_severity:
                    continue
                filtered_hits.append(item)
            filtered_hits.sort(key=self._rule_hit_significance_sort_key)
            offset = _decode_offset_cursor(cursor)
            page_items = filtered_hits[offset : offset + page_limit]
            has_more = offset + page_limit < len(filtered_hits)
            next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
            evidence_ids = _unique_texts(evidence_id for item in page_items for evidence_id in item.get("evidence_ids", []))
            stats = {
                "total_hits": len(filtered_hits),
                "high": sum(1 for item in filtered_hits if _trim(item.get("severity")).lower() == "high"),
                "medium": sum(1 for item in filtered_hits if _trim(item.get("severity")).lower() == "medium"),
                "low": sum(1 for item in filtered_hits if _trim(item.get("severity")).lower() == "low"),
                "rule_count": len({_trim(item.get("rule_code")) for item in filtered_hits if _trim(item.get("rule_code"))}),
                "input_coverage": input_coverage,
                "feature_readiness": {
                    "cash_dependent_rules": cash_feature_coverage,
                    "cash_independent_rules": {"status": "complete"},
                },
                "rule_engine_version": _trim(effective_rule_params.get("rule_engine_version")),
                "rule_params": {
                    key: effective_rule_params.get(key)
                    for key in (
                        "fast_in_out_min_amount",
                        "fast_in_out_window_minutes",
                        "fast_in_out_ratio",
                        "small_fast_in_out_min_amount",
                        "small_fast_in_out_max_amount",
                        "small_fast_in_out_min_event_count",
                        "small_fast_in_out_min_total_amount",
                        "single_fast_in_out_candidate_min_amount",
                        "single_fast_in_out_candidate_min_out_ratio",
                        "single_fast_in_out_candidate_max_out_ratio",
                        "single_fast_in_out_candidate_window_minutes",
                        "high_freq_small_min_total_amount",
                        "threshold_split_amount",
                        "threshold_split_tolerance_rate",
                        "threshold_split_min_count",
                        "near_threshold_amount",
                        "near_threshold_lower_rate",
                        "near_threshold_min_count",
                        "near_threshold_min_total_amount",
                        "cash_quick_in_out_min_amount",
                        "cash_quick_in_out_min_event_count",
                        "cash_quick_in_out_min_total_amount",
                        "cash_quick_candidate_min_amount",
                        "cash_quick_candidate_min_out_ratio",
                        "cash_quick_candidate_max_out_ratio",
                        "cash_quick_candidate_window_minutes",
                        "repeated_amount_min_amount",
                        "repeated_amount_min_count",
                        "repeated_amount_min_total_amount",
                        "round_amount_unit",
                        "round_amount_min_count",
                        "night_activity_start_hour",
                        "night_activity_end_hour",
                        "night_activity_min_total_amount",
                        "same_name_transfer_min_total_amount",
                        "fan_in_out_min_unique_in",
                        "fan_in_out_min_unique_out",
                        "fan_in_out_min_total_amount",
                        "circular_flow_min_amount",
                        "asset_purchase_min_amount",
                        "high_consumption_min_amount",
                        "property_parking_min_total_amount",
                    )
                },
            }
            breakdown_map: dict[tuple[str, str], dict[str, Any]] = {}
            for item in filtered_hits:
                rule_code = _trim(item.get("rule_code"))
                severity_value = _trim(item.get("severity")).lower() or "unknown"
                key = (rule_code, severity_value)
                bucket = breakdown_map.setdefault(
                    key,
                    {
                        "rule_code": rule_code,
                        "severity": severity_value,
                        "count": 0,
                    },
                )
                bucket["count"] = _as_int(bucket.get("count")) + 1
            stats["rule_breakdown"] = sorted(
                breakdown_map.values(),
                key=lambda item: (-_as_int(item.get("count")), str(item.get("rule_code") or ""), str(item.get("severity") or "")),
            )
            duration_ms = int((time.perf_counter() - started) * 1000)
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="get_rule_hits",
                params={
                    "case_id": case_id,
                    "entity_id": entity_id,
                    "risk_type": risk_type,
                    "severity": severity,
                    "cursor": cursor,
                    "limit": page_limit,
                },
                summary={
                    "total_hits": stats["total_hits"],
                    "returned_hits": len(page_items),
                    "high": stats["high"],
                    "medium": stats["medium"],
                    "low": stats["low"],
                    "has_more": has_more,
                },
                row_count=len(page_items),
                duration_ms=duration_ms,
            )
            result = {
                "semantic_status": (
                    "partial"
                    if cash_feature_coverage.get("status") != "complete"
                    else "candidate"
                ),
                "fact_answer_allowed": False,
                "coverage_status": (
                    "partial"
                    if cash_feature_coverage.get("status") != "complete"
                    else "complete"
                ),
                "query_id": query_id,
                "items": [
                    {
                        "rule_hit_id": _trim(item.get("rule_hit_id")),
                        "rule_code": _trim(item.get("rule_code")),
                        "risk_type": _trim(item.get("risk_type")),
                        "severity": _trim(item.get("severity")),
                        "score": _optional_rounded_float(item.get("score"), 2),
                        "title": _trim(item.get("title")),
                        "summary": _trim(item.get("summary")),
                        "entity_ids": _json_loads(_json_dumps(item.get("entity_ids", [])), []),
                        "txn_ids": _json_loads(_json_dumps(item.get("txn_ids", [])), []),
                        "evidence_ids": _json_loads(_json_dumps(item.get("evidence_ids", [])), []),
                        "detail_json": item.get("detail_json", {}),
                    }
                    for item in page_items
                ],
                "stats": stats,
                "evidence_ids": evidence_ids,
                "cursor": next_cursor,
                "has_more": has_more,
                "truncated": False,
                "warnings": (
                    [
                        {
                            "code": "CASH_CLASSIFICATION_COVERAGE_PARTIAL",
                            "message": "现金分类存在未知或冲突；现金及非现金依赖规则未计算，不能将缺少命中解释为无现金异常。",
                            "severity": "warning",
                        }
                    ]
                    if cash_feature_coverage.get("status") != "complete"
                    else []
                ),
            }
            return result
        finally:
            if owns_engine:
                engine.close()

    def _sanitize_attr_payload(self, value: Any) -> Any:
        if isinstance(value, set):
            return sorted(self._sanitize_attr_payload(item) for item in value)
        if isinstance(value, list):
            return [self._sanitize_attr_payload(item) for item in value]
        if isinstance(value, dict):
            return {str(key): self._sanitize_attr_payload(item) for key, item in value.items()}
        if isinstance(value, datetime):
            return value.strftime("%Y-%m-%d %H:%M:%S")
        if isinstance(value, float):
            return round(value, 4)
        return value

    def _ensure_materialized_graph(
        self,
        engine: DuckDBEngine,
        case_id: str,
        txn_rows: Optional[Sequence[dict[str, Any]]] = None,
        *,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        del force_refresh
        rows = list(txn_rows) if txn_rows is not None else self._load_transaction_rows(engine, case_id)
        if _rule_input_coverage(rows).get("status") != "complete":
            raise AnalysisGraphContractError()
        nodes_by_id: dict[str, dict[str, Any]] = {}
        edges_by_id: dict[str, dict[str, Any]] = {}
        person_device_pairs: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
        device_branch_pairs: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)

        def merge_time(current: str, candidate: str, *, pick_min: bool) -> str:
            current_dt = _coerce_datetime(current)
            candidate_dt = _coerce_datetime(candidate)
            if candidate_dt is None:
                return current
            if current_dt is None:
                return candidate
            return candidate if (candidate_dt < current_dt if pick_min else candidate_dt > current_dt) else current

        def ensure_node(
            *,
            entity_id: str,
            entity_type: str,
            entity_key: str,
            display_name: str,
            id_no: str = "",
            bank_name: str = "",
            attrs: Optional[dict[str, Any]] = None,
            seen_at: str = "",
        ) -> None:
            normalized_id = _trim(entity_id)
            if not normalized_id:
                return
            entry = nodes_by_id.get(normalized_id)
            if entry is None:
                entry = {
                    "entity_id": normalized_id,
                    "case_id": case_id,
                    "entity_type": _trim(entity_type) or "entity",
                    "entity_key": _trim(entity_key) or normalized_id,
                    "display_name": _trim(display_name) or _trim(entity_key) or normalized_id,
                    "id_no": _trim(id_no),
                    "bank_name": _trim(bank_name),
                    "attrs": {},
                    "first_seen_at": _trim(seen_at),
                    "last_seen_at": _trim(seen_at),
                }
                nodes_by_id[normalized_id] = entry
            if _trim(display_name):
                entry["display_name"] = _trim(display_name)
            if _trim(id_no):
                entry["id_no"] = _trim(id_no)
            if _trim(bank_name):
                entry["bank_name"] = _trim(bank_name)
            if _trim(seen_at):
                entry["first_seen_at"] = merge_time(_trim(entry.get("first_seen_at")), _trim(seen_at), pick_min=True)
                entry["last_seen_at"] = merge_time(_trim(entry.get("last_seen_at")), _trim(seen_at), pick_min=False)
            payload = attrs or {}
            attr_bucket = entry.setdefault("attrs", {})
            for raw_key, raw_value in payload.items():
                key = _trim(raw_key)
                if not key or raw_value in (None, "", []):
                    continue
                if isinstance(raw_value, (list, tuple, set)):
                    values = attr_bucket.setdefault(key, set())
                    if not isinstance(values, set):
                        values = set(_trim_list(values if isinstance(values, list) else [values]))
                    values.update(_trim(item) for item in raw_value if _trim(item))
                    attr_bucket[key] = values
                elif isinstance(raw_value, (int, float, bool)):
                    attr_bucket[key] = raw_value
                else:
                    attr_bucket[key] = _trim(raw_value)

        def ensure_edge(
            *,
            src_entity_id: str,
            dst_entity_id: str,
            relation_type: str,
            amount: float | None = None,
            seen_at: str = "",
            txn_id: str = "",
            attrs: Optional[dict[str, Any]] = None,
        ) -> None:
            src = _trim(src_entity_id)
            dst = _trim(dst_entity_id)
            relation = _trim(relation_type)
            if not src or not dst or not relation:
                return
            edge_id = _stable_slug("edge", case_id, src, relation, dst)
            entry = edges_by_id.get(edge_id)
            if entry is None:
                entry = {
                    "edge_id": edge_id,
                    "case_id": case_id,
                    "src_entity_id": src,
                    "dst_entity_id": dst,
                    "relation_type": relation,
                    "txn_count": 0,
                    "amount_sum": 0.0,
                    "amount_present_count": 0,
                    "amount_missing_count": 0,
                    "first_seen_at": _trim(seen_at),
                    "last_seen_at": _trim(seen_at),
                    "attrs": {"txn_ids": set()},
                }
                edges_by_id[edge_id] = entry
            normalized_txn_id = _trim(txn_id)
            if normalized_txn_id:
                entry["txn_count"] += 1
                normalized_amount = _optional_finite_float(amount)
                if normalized_amount is None:
                    entry["amount_missing_count"] += 1
                else:
                    entry["amount_present_count"] += 1
                    entry["amount_sum"] = round(entry["amount_sum"] + normalized_amount, 2)
            if _trim(seen_at):
                entry["first_seen_at"] = merge_time(_trim(entry.get("first_seen_at")), _trim(seen_at), pick_min=True)
                entry["last_seen_at"] = merge_time(_trim(entry.get("last_seen_at")), _trim(seen_at), pick_min=False)
            if normalized_txn_id:
                entry.setdefault("attrs", {}).setdefault("txn_ids", set()).add(normalized_txn_id)
            payload = attrs or {}
            attr_bucket = entry.setdefault("attrs", {})
            for raw_key, raw_value in payload.items():
                key = _trim(raw_key)
                if not key or raw_value in (None, "", []):
                    continue
                if isinstance(raw_value, (list, tuple, set)):
                    values = attr_bucket.setdefault(key, set())
                    if not isinstance(values, set):
                        values = set(_trim_list(values if isinstance(values, list) else [values]))
                    values.update(_trim(item) for item in raw_value if _trim(item))
                    attr_bucket[key] = values
                elif isinstance(raw_value, (int, float, bool)):
                    attr_bucket[key] = raw_value
                else:
                    attr_bucket[key] = _trim(raw_value)

        for row in rows:
            seen_at = _trim(row.get("txn_time"))
            account_entity_id = _trim(row.get("account_entity_id"))
            ensure_node(
                entity_id=account_entity_id,
                entity_type="account",
                entity_key=_trim(row.get("account_key")),
                display_name=_trim(row.get("account_name")) or _trim(row.get("account_key")),
                id_no=_trim(row.get("opener_id_no")),
                attrs={
                    "account_keys": [_trim(row.get("account_key"))],
                    "account_names": [_trim(row.get("account_name"))],
                },
                seen_at=seen_at,
            )
            if _trim(row.get("person_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("person_entity_id")),
                    entity_type="person",
                    entity_key=_trim(row.get("opener_id_no")),
                    display_name=_trim(row.get("account_name")) or _trim(row.get("opener_id_no")),
                    id_no=_trim(row.get("opener_id_no")),
                    attrs={"account_keys": [_trim(row.get("account_key"))]},
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("person_entity_id")),
                    relation_type="account_to_person_owner",
                    seen_at=seen_at,
                    attrs={"account_keys": [_trim(row.get("account_key"))]},
                )
            if _trim(row.get("device_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("device_entity_id")),
                    entity_type="device",
                    entity_key=_trim(row.get("device_key")) or _trim(row.get("mac_addr")) or _trim(row.get("ip_addr")),
                    display_name=_trim(row.get("mac_addr")) or _trim(row.get("ip_addr")) or _trim(row.get("device_key")),
                    attrs={
                        "ip_addrs": [_trim(row.get("ip_addr"))],
                        "mac_addrs": [_trim(row.get("mac_addr"))],
                        "account_keys": [_trim(row.get("account_key"))],
                    },
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("device_entity_id")),
                    relation_type="account_to_device_used",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                    attrs={"account_keys": [_trim(row.get("account_key"))]},
                )
                if _trim(row.get("person_entity_id")):
                    person_device_pairs[(_trim(row.get("person_entity_id")), _trim(row.get("device_entity_id")))].append(row)
            if _trim(row.get("branch_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("branch_entity_id")),
                    entity_type="branch",
                    entity_key=_trim(row.get("branch_key")),
                    display_name=_trim(row.get("branch_name")) or _trim(row.get("branch_key")),
                    attrs={"account_keys": [_trim(row.get("account_key"))]},
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("branch_entity_id")),
                    relation_type="account_to_branch_seen",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                    attrs={"branch_names": [_trim(row.get("branch_name"))]},
                )
                if _trim(row.get("device_entity_id")):
                    device_branch_pairs[(_trim(row.get("device_entity_id")), _trim(row.get("branch_entity_id")))].append(row)
            if _trim(row.get("location_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("location_entity_id")),
                    entity_type="location",
                    entity_key=_trim(row.get("location_key")),
                    display_name=_trim(row.get("location")) or _trim(row.get("location_key")),
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("location_entity_id")),
                    relation_type="account_to_location_seen",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                )
            if _trim(row.get("voucher_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("voucher_entity_id")),
                    entity_type="voucher",
                    entity_key=_trim(row.get("voucher_key")),
                    display_name=" / ".join(
                        _unique_texts((_trim(row.get("voucher_type")), _trim(row.get("voucher_no")), _trim(row.get("log_no"))))
                    ),
                    attrs={
                        "voucher_no": _trim(row.get("voucher_no")),
                        "receipt_no": _trim(row.get("receipt_no")),
                        "log_no": _trim(row.get("log_no")),
                    },
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("voucher_entity_id")),
                    relation_type="account_to_voucher_linked",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                    attrs={
                        "voucher_no": _trim(row.get("voucher_no")),
                        "receipt_no": _trim(row.get("receipt_no")),
                        "log_no": _trim(row.get("log_no")),
                    },
                )
            if _trim(row.get("teller_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("teller_entity_id")),
                    entity_type="teller",
                    entity_key=_trim(row.get("teller_no")),
                    display_name=_trim(row.get("teller_no")),
                    attrs={"branch_name": _trim(row.get("branch_name"))},
                    seen_at=seen_at,
                )
                ensure_edge(
                    src_entity_id=account_entity_id,
                    dst_entity_id=_trim(row.get("teller_entity_id")),
                    relation_type="account_to_teller_linked",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                    attrs={"branch_names": [_trim(row.get("branch_name"))]},
                )
            if _trim(row.get("counterparty_entity_id")):
                ensure_node(
                    entity_id=_trim(row.get("counterparty_entity_id")),
                    entity_type=_trim(row.get("counterparty_entity_type")) or "counterparty",
                    entity_key=_trim(row.get("counterparty_entity_key")),
                    display_name=_trim(row.get("counterparty_display_name")) or _trim(row.get("counterparty_entity_key")),
                    id_no=_trim(row.get("counterparty_id_no")),
                    bank_name=_trim(row.get("counterparty_bank")),
                    attrs={
                        "account_keys": [_trim(row.get("counterparty_acct"))],
                        "bank_names": [_trim(row.get("counterparty_bank"))],
                    },
                    seen_at=seen_at,
                )
                if row.get("direction_norm") == "in":
                    src_entity_id = _trim(row.get("counterparty_entity_id"))
                    dst_entity_id = account_entity_id
                elif row.get("direction_norm") == "out":
                    src_entity_id = account_entity_id
                    dst_entity_id = _trim(row.get("counterparty_entity_id"))
                else:
                    src_entity_id = account_entity_id
                    dst_entity_id = _trim(row.get("counterparty_entity_id"))
                ensure_edge(
                    src_entity_id=src_entity_id,
                    dst_entity_id=dst_entity_id,
                    relation_type="account_to_account_transfer",
                    amount=_optional_finite_float(row.get("amount_val")),
                    seen_at=seen_at,
                    txn_id=_trim(row.get("txn_id")),
                    attrs={
                        "account_keys": _unique_texts((_trim(row.get("account_key")), _trim(row.get("counterparty_acct")))),
                        "txn_types": [_trim(row.get("txn_type"))],
                        "direction": _trim(row.get("direction_norm")),
                    },
                )

        for (person_entity_id, device_entity_id), linked_rows in person_device_pairs.items():
            for linked_row in linked_rows:
                ensure_edge(
                    src_entity_id=person_entity_id,
                    dst_entity_id=device_entity_id,
                    relation_type="person_to_device_shared",
                    amount=_optional_finite_float(linked_row.get("amount_val")),
                    seen_at=_trim(linked_row.get("txn_time")),
                    txn_id=_trim(linked_row.get("txn_id")),
                    attrs={"account_keys": [_trim(linked_row.get("account_key"))]},
                )
        for (device_entity_id, branch_entity_id), linked_rows in device_branch_pairs.items():
            for linked_row in linked_rows:
                ensure_edge(
                    src_entity_id=device_entity_id,
                    dst_entity_id=branch_entity_id,
                    relation_type="device_to_branch_shared",
                    amount=_optional_finite_float(linked_row.get("amount_val")),
                    seen_at=_trim(linked_row.get("txn_time")),
                    txn_id=_trim(linked_row.get("txn_id")),
                    attrs={"account_keys": [_trim(linked_row.get("account_key"))]},
                )

        account_node_count = sum(
            1
            for node in nodes_by_id.values()
            if _trim(node.get("entity_type")) == "account"
        )
        if account_node_count <= 0:
            raise AnalysisGraphContractError()

        self._reset_entity_graph_tables(engine, case_id)

        now_text = _now_text()
        node_rows: list[tuple[Any, ...]] = []
        evidence_rows: list[tuple[Any, ...]] = []
        for node in nodes_by_id.values():
            attrs_json = _json_dumps(self._sanitize_attr_payload(node.get("attrs") or {}))
            node_rows.append(
                (
                    node["entity_id"],
                    case_id,
                    node["entity_type"],
                    node["entity_key"],
                    node["display_name"],
                    _trim(node.get("id_no")),
                    _trim(node.get("bank_name")),
                    attrs_json,
                    _trim(node.get("first_seen_at")),
                    _trim(node.get("last_seen_at")),
                    now_text,
                )
            )
            evidence_rows.append(
                (
                    _stable_slug("ev", case_id, "analysis_entity_node", node["entity_id"]),
                    case_id,
                    "entity_node",
                    "analysis_entity_node",
                    node["entity_id"],
                    node["display_name"],
                    f"{node['entity_type']} 节点 {node['display_name']}",
                    _json_dumps(
                        {
                            "entity_id": node["entity_id"],
                            "entity_type": node["entity_type"],
                            "entity_key": node["entity_key"],
                        }
                    ),
                    now_text,
                )
            )

        if node_rows:
            engine.executemany(
                """
                INSERT INTO analysis_entity_node(
                    entity_id, case_id, entity_type, entity_key, display_name, id_no, bank_name,
                    attrs_json, first_seen_at, last_seen_at, created_at
                ) VALUES (?,?,?,?,?,?,?,?,?,?,?)
                """,
                node_rows,
            )

        edge_rows: list[tuple[Any, ...]] = []
        unresolved_amount_edge_count = 0
        for edge in edges_by_id.values():
            txn_count = edge["txn_count"]
            amount_present_count = edge["amount_present_count"]
            amount_missing_count = edge["amount_missing_count"]
            amount_applicable = txn_count > 0
            amount_complete = (
                amount_applicable
                and amount_missing_count == 0
                and amount_present_count == txn_count
            )
            amount_sum = round(edge["amount_sum"], 2) if amount_complete else None
            weight = (
                _clamp_score(
                    0.42
                    + min(txn_count / 8, 1.0) * 0.2
                    + min(abs(amount_sum) / 500000, 1.0) * 0.18
                )
                if amount_sum is not None
                else None
            )
            if amount_applicable and not amount_complete:
                unresolved_amount_edge_count += 1
            attrs = dict(edge.get("attrs") or {})
            attrs["amount_coverage"] = {
                "status": (
                    "complete"
                    if amount_complete
                    else "unresolved"
                    if amount_applicable
                    else "not_applicable"
                ),
                "present_count": amount_present_count,
                "missing_count": amount_missing_count,
                "transaction_count": txn_count,
            }
            attrs_json = _json_dumps(self._sanitize_attr_payload(attrs))
            edge_rows.append(
                (
                    edge["edge_id"],
                    case_id,
                    edge["src_entity_id"],
                    edge["dst_entity_id"],
                    edge["relation_type"],
                    weight,
                    txn_count,
                    amount_sum,
                    _trim(edge.get("first_seen_at")),
                    _trim(edge.get("last_seen_at")),
                    attrs_json,
                )
            )
            if amount_complete:
                evidence_rows.append(
                    (
                        _stable_slug("ev", case_id, "analysis_relation_edge", edge["edge_id"]),
                        case_id,
                        "relation_edge",
                        "analysis_relation_edge",
                        edge["edge_id"],
                        f"{edge['relation_type']} · {edge['src_entity_id']} -> {edge['dst_entity_id']}",
                        f"{edge['relation_type']} 关联 {txn_count} 笔，累计 {format(amount_sum, '.2f')} 元。",
                        _json_dumps(
                            {
                                "edge_id": edge["edge_id"],
                                "relation_type": edge["relation_type"],
                                "src_entity_id": edge["src_entity_id"],
                                "dst_entity_id": edge["dst_entity_id"],
                                "txn_count": txn_count,
                                "amount_sum": amount_sum,
                                "amount_coverage": "complete",
                            }
                        ),
                        now_text,
                    )
                )

        if edge_rows:
            engine.executemany(
                """
                INSERT INTO analysis_relation_edge(
                    edge_id, case_id, src_entity_id, dst_entity_id, relation_type, weight, txn_count,
                    amount_sum, first_seen_at, last_seen_at, attrs_json
                ) VALUES (?,?,?,?,?,?,?,?,?,?,?)
                """,
                edge_rows,
            )
        if evidence_rows:
            engine.executemany(
                """
                INSERT INTO analysis_evidence_ref(
                    evidence_id, case_id, evidence_type, ref_table, ref_pk, title, snippet, payload_json, created_at
                ) VALUES (?,?,?,?,?,?,?,?,?)
                """,
                evidence_rows,
            )

        stats = {
            "node_count": len(nodes_by_id),
            "edge_count": len(edges_by_id),
            "txn_count": len(rows),
            "account_node_count": account_node_count,
            "unresolved_amount_edge_count": unresolved_amount_edge_count,
        }
        return _validated_graph_materialization_stats(
            stats,
            expected_txn_count=len(rows),
        )

    def _evidence_ids_for_refs(
        self,
        engine: DuckDBEngine,
        case_id: str,
        refs: Sequence[tuple[str, str]],
    ) -> list[str]:
        if not refs:
            return []
        ref_set = {(_trim(table), _trim(ref_pk)) for table, ref_pk in refs if _trim(table) and _trim(ref_pk)}
        evidence_rows = self._query_dicts(
            engine,
            "SELECT evidence_id, ref_table, ref_pk FROM analysis_evidence_ref WHERE case_id=?",
            (case_id,),
        )
        return _unique_texts(
            row.get("evidence_id")
            for row in evidence_rows
            if (_trim(row.get("ref_table")), _trim(row.get("ref_pk"))) in ref_set and _trim(row.get("evidence_id"))
        )

    def _apply_txn_filters(
        self,
        txn_rows: Sequence[dict[str, Any]],
        filters: Optional[dict[str, Any]],
    ) -> list[dict[str, Any]]:
        payload = dict(filters or {})
        entity_id = _trim(payload.get("entity_id"))
        account_key = _trim(payload.get("account_key")) or self._parse_entity_account(entity_id if entity_id.startswith("acct_") else "")
        person_key = _trim(payload.get("person_id")) or self._parse_entity_person(entity_id if entity_id.startswith("person_") else "")
        direction = _trim(payload.get("direction")).lower()
        counterparty_acct = _trim(payload.get("counterparty_acct"))
        counterparty_name = _trim(payload.get("counterparty_name"))
        ip_addr = _trim(payload.get("ip_addr"))
        mac_addr = _trim(payload.get("mac_addr"))
        branch_name = _trim(payload.get("branch_name"))
        voucher_no = _trim(payload.get("voucher_no"))
        teller_no = _trim(payload.get("teller_no"))
        log_no = _trim(payload.get("log_no"))
        keyword = _trim(payload.get("keyword"))
        txn_ids = {_trim(item) for item in payload.get("txn_ids", []) if _trim(item)}
        file_ids = {_trim(item) for item in payload.get("file_ids", []) if _trim(item)}
        time_range = payload.get("time_range") if isinstance(payload.get("time_range"), dict) else {}
        start_at = _coerce_datetime(time_range.get("start") or payload.get("start"))
        end_at = _coerce_datetime(time_range.get("end") or payload.get("end"))
        min_amount = payload.get("min_amount")
        max_amount = payload.get("max_amount")
        min_amount_value = None if min_amount in (None, "") else _optional_finite_float(min_amount)
        max_amount_value = None if max_amount in (None, "") else _optional_finite_float(max_amount)
        if min_amount not in (None, "") and min_amount_value is None:
            raise ValueError("transaction_min_amount_invalid")
        if max_amount not in (None, "") and max_amount_value is None:
            raise ValueError("transaction_max_amount_invalid")
        results: list[dict[str, Any]] = []
        for row in txn_rows:
            if account_key and _trim(row.get("account_key")) != account_key:
                continue
            if person_key and _trim(row.get("opener_id_no")) != person_key and _trim(row.get("person_entity_id")) != _build_entity_id("person", person_key):
                continue
            if entity_id and entity_id.startswith(("device_", "branch_", "voucher_", "teller_", "cp_", "loc_")):
                if entity_id not in {
                    _trim(row.get("device_entity_id")),
                    _trim(row.get("branch_entity_id")),
                    _trim(row.get("voucher_entity_id")),
                    _trim(row.get("teller_entity_id")),
                    _trim(row.get("counterparty_entity_id")),
                    _trim(row.get("location_entity_id")),
                }:
                    continue
            if direction and _trim(row.get("direction_norm")).lower() != direction:
                continue
            if counterparty_acct and _trim(row.get("counterparty_acct")) != counterparty_acct:
                continue
            if counterparty_name and counterparty_name not in _trim(row.get("counterparty_name")):
                continue
            if ip_addr and ip_addr not in {_trim(row.get("ip_addr")), _trim(row.get("ip_key"))}:
                continue
            if mac_addr and _normalize_mac(mac_addr) not in {_trim(row.get("mac_key")), _normalize_mac(row.get("mac_addr"))}:
                continue
            if branch_name and branch_name not in _trim(row.get("branch_name")):
                continue
            if voucher_no and voucher_no != _trim(row.get("voucher_no")):
                continue
            if teller_no and teller_no != _trim(row.get("teller_no")):
                continue
            if log_no and log_no != _trim(row.get("log_no")):
                continue
            if txn_ids and _trim(row.get("txn_id")) not in txn_ids:
                continue
            if file_ids and _trim(row.get("file_id")) not in file_ids:
                continue
            if start_at and (row.get("txn_ts") is None or row.get("txn_ts") < start_at):
                continue
            if end_at and (row.get("txn_ts") is None or row.get("txn_ts") > end_at):
                continue
            amount = _optional_finite_float(row.get("amount_val"))
            if min_amount_value is not None or max_amount_value is not None:
                if amount is None:
                    continue
                if min_amount_value is not None and amount < min_amount_value:
                    continue
                if max_amount_value is not None and amount > max_amount_value:
                    continue
            if keyword:
                haystack = " ".join(
                    _trim(value)
                    for value in (
                        row.get("summary"),
                        row.get("remark"),
                        row.get("counterparty_name"),
                        row.get("counterparty_acct"),
                        row.get("branch_name"),
                        row.get("txn_type"),
                    )
                )
                if keyword not in haystack:
                    continue
            results.append(row)
        return results

    def get_txn_slice(
        self,
        case_id: str,
        *,
        filters: Optional[dict[str, Any]] = None,
        visible_columns: Optional[Sequence[str]] = None,
        sort: Optional[dict[str, Any]] = None,
        cursor: str = "",
        limit: int = 50,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        owns_engine = True
        try:
            self.ensure_analysis_schema(engine)
            normalized_file_ids = _trim_list((filters or {}).get("file_ids") or [])
            sort_payload = sort if isinstance(sort, dict) else {}
            page_limit = max(1, min(int(limit or 50), 200))
            normalized_filters = dict(filters or {})
            normalized_filters["file_ids"] = normalized_file_ids
            txn_rows = self._load_transaction_rows(engine, case_id, file_ids=normalized_file_ids or None)
            matched_rows = self._apply_txn_filters(txn_rows, filters)
            sort_field = _trim(sort_payload.get("field")) or "txn_time"
            sort_order = "asc" if _trim(sort_payload.get("order")).lower() == "asc" else "desc"
            if sort_field == "amount":
                known_amount_rows = [
                    row for row in matched_rows if _optional_finite_float(row.get("amount_val")) is not None
                ]
                unresolved_amount_rows = [
                    row for row in matched_rows if _optional_finite_float(row.get("amount_val")) is None
                ]
                known_amount_rows.sort(
                    key=lambda row: (
                        float(_optional_finite_float(row.get("amount_val"))),
                        _trim(row.get("txn_id")),
                    ),
                    reverse=sort_order == "desc",
                )
                unresolved_amount_rows.sort(key=lambda row: _trim(row.get("txn_id")))
                matched_rows = [*known_amount_rows, *unresolved_amount_rows]
            else:
                matched_rows.sort(
                    key=lambda row: (
                        row.get("txn_ts") or datetime.min,
                        _trim(row.get("txn_id")),
                    ),
                    reverse=sort_order == "desc",
                )
            offset = _decode_offset_cursor(cursor)
            page_rows = matched_rows[offset : offset + page_limit]
            has_more = offset + page_limit < len(matched_rows)
            next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
            requested_columns = {_trim(item) for item in visible_columns or [] if _trim(item)}
            items: list[dict[str, Any]] = []
            evidence_ids: list[str] = []
            for row in page_rows:
                title, snippet, payload = self._txn_evidence_payload(row)
                evidence_id = self._upsert_evidence_ref(
                    engine,
                    case_id=case_id,
                    evidence_type="txn",
                    ref_table="fc_transaction_norm",
                    ref_pk=_trim(row.get("txn_id")),
                    title=title,
                    snippet=snippet,
                    payload=payload,
                )
                evidence_ids.append(evidence_id)
                item = {
                    "txn_id": _trim(row.get("txn_id")),
                    "account_key": _trim(row.get("account_key")),
                    "txn_time": _trim(row.get("txn_time")),
                    "direction": _trim(row.get("direction_norm")),
                    "amount": _optional_rounded_float(row.get("amount_val"), 2),
                    "balance": _optional_rounded_float(row.get("balance_val"), 2),
                    "counterparty_acct": _trim(row.get("counterparty_acct")),
                    "counterparty_name": _trim(row.get("counterparty_name")),
                    "counterparty_bank": _trim(row.get("counterparty_bank")),
                    "summary": _trim(row.get("summary")),
                    "txn_type": _trim(row.get("txn_type")),
                    "file_id": _trim(row.get("file_id")),
                    "is_cash": _optional_bool(row.get("cash_flag_norm")),
                    "is_success": _optional_bool(row.get("success_flag_norm")),
                    "ip_address": _trim(row.get("ip_addr")),
                    "mac_address": _trim(row.get("mac_addr")),
                    "branch_name": _trim(row.get("branch_name")),
                    "voucher_no": _trim(row.get("voucher_no")),
                    "log_no": _trim(row.get("log_no")),
                    "evidence_id": evidence_id,
                }
                if requested_columns:
                    item = {key: value for key, value in item.items() if key in requested_columns or key in {"txn_id", "evidence_id"}}
                items.append(item)
            stats = {
                "matched_rows": len(matched_rows),
                "returned_rows": len(page_rows),
                "sum_in_amount": _complete_amount_sum(
                    row.get("amount_val")
                    for row in matched_rows
                    if row.get("direction_norm") == "in"
                ),
                "sum_out_amount": _complete_amount_sum(
                    row.get("amount_val")
                    for row in matched_rows
                    if row.get("direction_norm") == "out"
                ),
            }
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="query_txn_slice",
                params={
                    "case_id": case_id,
                    "filters": filters or {},
                    "visible_columns": list(visible_columns or []),
                    "sort": sort or {},
                    "cursor": cursor,
                    "limit": page_limit,
                },
                summary=stats,
                row_count=len(page_rows),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            result = {
                "query_id": query_id,
                "items": items,
                "stats": stats,
                "evidence_ids": _unique_texts(evidence_ids),
                "cursor": next_cursor,
                "has_more": has_more,
                "truncated": False,
            }
            return result
        finally:
            if owns_engine:
                engine.close()

    def upsert_temp_transaction_scope(
        self,
        case_id: str,
        *,
        source_file_ids: Sequence[str],
        document_ids: Optional[Sequence[str]] = None,
        source_kind: str = "uploaded_file",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_document_ids = _trim_list(document_ids or [])
        normalized_requested_file_ids = _trim_list(source_file_ids or [])
        if not normalized_requested_file_ids and not normalized_document_ids:
            raise ValueError("temp_scope_source_required")
        normalized_source_kind = _trim(source_kind).lower()
        if normalized_source_kind not in _TEMP_SCOPE_SOURCE_KINDS:
            normalized_source_kind = "uploaded_file"

        engine = self.open_case_engine(case_id)
        transaction_started = False
        try:
            engine.execute("BEGIN TRANSACTION")
            transaction_started = True
            self.ensure_analysis_schema(engine)
            lifecycle_warnings = self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
            ingest_result = self._ingest_temp_transaction_files(
                engine,
                case_id=case_id,
                source_file_ids=normalized_requested_file_ids,
                document_ids=normalized_document_ids,
            )
            selected_file_ids = _unique_texts(ingest_result.get("resolved_file_ids") or [])
            if not selected_file_ids:
                raise ValueError("temp_scope_source_unavailable")

            selected_source_refs = self._temp_scope_source_refs(
                case_id=case_id,
                source_ids=selected_file_ids,
            )
            revalidated_file_ids = self._resolve_ready_temp_scope_file_ids(
                engine,
                case_id=case_id,
                stored_refs=selected_source_refs,
            )
            if (
                len(revalidated_file_ids) != len(selected_file_ids)
                or set(revalidated_file_ids) != set(selected_file_ids)
            ):
                raise ValueError("temp_scope_source_unavailable")

            normalized_file_ids = revalidated_file_ids
            source_binding_refs = self._temp_scope_source_refs(
                case_id=case_id,
                source_ids=normalized_file_ids,
            )
            source_revision = self._stats_source_revision(engine)
            scope_signature = _stable_slug(
                "temp_scope_v2",
                case_id,
                normalized_source_kind,
                source_revision,
                _json_dumps(sorted(normalized_file_ids)),
                _json_dumps(sorted(normalized_document_ids)),
            )
            scope_id = opaque_case_bound_ref(
                prefix="temp_scope_v1",
                case_id=case_id,
                value=scope_signature,
            )
            now_text = _now_text()
            expires_at, retention_until = self._temp_scope_lifecycle(now_text=now_text)
            existing_rows = self._query_dicts(
                engine,
                "SELECT scope_id, created_at FROM analysis_temp_scope WHERE case_id=? AND scope_signature=? LIMIT 2",
                (case_id, scope_signature),
            )
            if len(existing_rows) > 1:
                raise ValueError("temp_scope_invalid")
            created_at = _trim(existing_rows[0].get("created_at")) if existing_rows else now_text
            if not _coerce_datetime(created_at):
                raise ValueError("temp_scope_invalid")

            failure_items = [
                dict(item)
                for item in list(ingest_result.get("skipped_sources") or [])
                if isinstance(item, Mapping)
            ]
            document_binding_refs = _unique_texts(
                opaque_case_bound_ref(prefix="tempdoc_v1", case_id=case_id, value=document_id)
                for document_id in normalized_document_ids
            )
            requested_source_count = len(normalized_requested_file_ids) + len(normalized_document_ids)
            resolved_source_count = len(normalized_file_ids)
            payload_stats = {
                "contract": "TempScopeOperationalStatsV2",
                "requested_source_count": requested_source_count,
                "resolved_source_count": resolved_source_count,
                "failure_count": len(failure_items),
                "failures": failure_items,
                "lifecycle_warning_codes": [
                    _trim(item.get("code"))
                    for item in lifecycle_warnings
                    if isinstance(item, Mapping) and _trim(item.get("code"))
                ],
                "fact_answer_allowed": False,
                "raw_details_exposed": False,
                "lifecycle": {
                    "status": "active",
                    "ttl_hours": TEMP_SCOPE_TTL_HOURS,
                    "expires_at": expires_at,
                    "retention_until": retention_until,
                    "active_limit": TEMP_SCOPE_ACTIVE_LIMIT,
                },
            }
            audit_payload = {
                "contract": "TempScopeAuditV2",
                "status": "active",
                "requested_source_count": requested_source_count,
                "resolved_source_count": resolved_source_count,
                "failure_count": len(failure_items),
                "cleanup_policy": {
                    "ttl_hours": TEMP_SCOPE_TTL_HOURS,
                    "audit_retention_days": TEMP_SCOPE_AUDIT_RETENTION_DAYS,
                    "active_limit": TEMP_SCOPE_ACTIVE_LIMIT,
                },
                "events": [
                    {
                        "action": "created_or_updated",
                        "at": now_text,
                    }
                ],
                "restricted_details_withheld": True,
            }
            if existing_rows:
                engine.execute(
                    """
                    UPDATE analysis_temp_scope
                       SET scope_type=?, source_kind=?, status=?, source_revision=?, source_file_ids_json=?, document_ids_json=?, stats_json=?, audit_json=?, expires_at=?, retention_until=?, last_accessed_at=?, updated_at=?
                     WHERE scope_id=? AND case_id=?
                    """,
                    (
                        "file_scope",
                        normalized_source_kind,
                        "active",
                        int(source_revision),
                        _json_dumps(source_binding_refs),
                        _json_dumps(document_binding_refs),
                        _json_dumps(payload_stats),
                        _json_dumps(audit_payload),
                        expires_at,
                        retention_until,
                        now_text,
                        now_text,
                        scope_id,
                        case_id,
                    ),
                )
            else:
                engine.execute(
                    """
                    INSERT INTO analysis_temp_scope(
                        scope_id, case_id, scope_type, source_kind, scope_signature, status, source_revision,
                        source_file_ids_json, document_ids_json, stats_json, audit_json, expires_at, retention_until, last_accessed_at, created_at, updated_at
                    ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        scope_id,
                        case_id,
                        "file_scope",
                        normalized_source_kind,
                        scope_signature,
                        "active",
                        int(source_revision),
                        _json_dumps(source_binding_refs),
                        _json_dumps(document_binding_refs),
                        _json_dumps(payload_stats),
                        _json_dumps(audit_payload),
                        expires_at,
                        retention_until,
                        now_text,
                        created_at,
                        now_text,
                    ),
                )

            readback_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_temp_scope WHERE case_id=? AND scope_id=? LIMIT 2",
                (case_id, scope_id),
            )
            if len(readback_rows) != 1:
                raise ValueError("temp_scope_invalid")
            _, _, readback_source_refs, readback_document_refs = self._validated_active_temp_scope_contract(
                readback_rows[0],
                case_id=case_id,
                scope_id=scope_id,
            )
            if (
                readback_source_refs != source_binding_refs
                or readback_document_refs != document_binding_refs
            ):
                raise ValueError("temp_scope_invalid")
            final_ready_file_ids = self._resolve_ready_temp_scope_file_ids(
                engine,
                case_id=case_id,
                stored_refs=readback_source_refs,
            )
            if (
                len(final_ready_file_ids) != len(normalized_file_ids)
                or set(final_ready_file_ids) != set(normalized_file_ids)
            ):
                raise ValueError("temp_scope_source_unavailable")

            engine.execute("COMMIT")
            transaction_started = False
            return project_temp_scope_public(
                case_id=case_id,
                scope_id=scope_id,
                status="active",
                requested_source_count=requested_source_count,
                resolved_source_count=resolved_source_count,
                failure_items=failure_items,
            )
        except Exception:
            if transaction_started:
                try:
                    engine.execute("ROLLBACK")
                except Exception as exc:
                    transaction_started = False
                    raise RuntimeError("temp_scope_rollback_failed") from exc
                transaction_started = False
            raise
        finally:
            engine.close()

    def get_temp_transaction_scope(self, case_id: str, scope_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_scope_id = _trim(scope_id)
        if not normalized_scope_id:
            raise KeyError(scope_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
            row, _ = self._resolve_active_temp_scope_or_raise(
                engine,
                case_id=case_id,
                temp_scope_id=normalized_scope_id,
            )
            stats = _json_loads(row.get("stats_json"), {})
            if not isinstance(stats, Mapping):
                stats = {}
            requested_count = stats.get("requested_source_count")
            resolved_count = stats.get("resolved_source_count")
            failures = stats.get("failures") if isinstance(stats.get("failures"), list) else []
            return project_temp_scope_public(
                case_id=case_id,
                scope_id=normalized_scope_id,
                status=_trim(row.get("status")) or "active",
                requested_source_count=requested_count,
                resolved_source_count=resolved_count,
                failure_items=failures,
            )
        finally:
            engine.close()

    def build_evidence_pack(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        txn_ids: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        budget: Optional[dict[str, Any]] = None,
    ) -> dict[str, Any]:
        del (
            account_keys,
            txn_ids,
            source_file_ids,
            temp_scope_id,
            scope_source,
            budget,
        )
        return project_analysis_evidence_pack_public(case_id=case_id)

    def build_scope_account_snapshot(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric_mode: str = "full",
        group_by: str = "none",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        return project_account_fact_boundary(case_id, "scope_account_snapshot")

    def build_case_reconciliation(self, case_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            try:
                self._daily_agg.ensure_materialized(case_id, engine=engine)
            except Exception:
                pass
            tables = [
                "fc_transaction_raw",
                "fc_transaction_norm",
                "analysis_txn_detail_idx",
                "analysis_rule_txn_idx",
                "analysis_txn_daily_agg",
                "analysis_account_dim",
                "fc_account_norm",
                "fc_person_norm",
                "import_file_log",
                "analysis_rule_hit",
                "analysis_evidence_ref",
            ]
            counts: dict[str, int | None] = {}
            for table_name in tables:
                if not self._table_exists(engine, table_name):
                    counts[table_name] = None
                    continue
                rows = engine.query(f"SELECT COUNT(1) FROM {table_name}")
                counts[table_name] = _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None
            daily_sum: int | None = None
            if self._table_exists(engine, "analysis_txn_daily_agg") and "txn_count" in self._table_columns(engine, "analysis_txn_daily_agg"):
                rows = engine.query("SELECT SUM(txn_count) FROM analysis_txn_daily_agg")
                daily_sum = _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None
            counts["analysis_txn_daily_agg_sum_txn_count"] = daily_sum

            detail_count = counts.get("analysis_txn_detail_idx")
            norm_count = counts.get("fc_transaction_norm")
            rule_idx_count = counts.get("analysis_rule_txn_idx")
            statuses = {
                "norm_vs_detail_idx": _reconcile_count_status(norm_count, detail_count),
                "detail_idx_vs_rule_idx": _reconcile_count_status(detail_count, rule_idx_count),
                "daily_agg_sum_vs_detail_idx": _reconcile_count_status(daily_sum, detail_count),
            }
            field_quality: dict[str, Any] = {}
            if self._table_exists(engine, "analysis_txn_detail_idx"):
                scope_stats = self._scope_stats_from_materialized(engine, where_sql="1=1", params=())
                field_quality["detail_idx"] = scope_stats.get("field_coverage") or {}
            if self._table_exists(engine, "fc_transaction_norm"):
                norm_columns = self._table_columns(engine, "fc_transaction_norm")
                field_quality["transaction_norm"] = self._field_presence_rates(
                    engine,
                    table_name="fc_transaction_norm",
                    columns=[
                        "card_no",
                        "card_no_norm",
                        "acct_no",
                        "acct_no_norm",
                        "account_open_name",
                        "opener_id_no",
                        "txn_time",
                        "txn_ts",
                        "amount",
                        "amount_val",
                        "balance",
                        "balance_val",
                        "dc_flag",
                        "dc_final",
                        "counterparty_acct",
                        "counterparty_acct_norm",
                        "counterparty_name",
                        "counterparty_bank",
                        "cash_flag",
                        "summary",
                    ],
                    available_columns=norm_columns,
                )
            warnings: list[dict[str, Any]] = []
            if any(value == "mismatch" for value in statuses.values()):
                warnings.append(
                    {
                        "code": "CASE_RECONCILIATION_MISMATCH",
                        "message": "交易标准表、明细索引、规则索引或日聚合之间存在数量不一致，不能生成全量结论。",
                        "severity": "error",
                    }
                )
            missing = [name for name, value in counts.items() if value is None]
            if missing:
                warnings.append(
                    {
                        "code": "CASE_RECONCILIATION_TABLE_MISSING",
                        "message": f"以下分析表不可用：{', '.join(missing)}。",
                        "severity": "warning",
                    }
                )
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="case_reconciliation",
                params={"case_id": case_id},
                summary={"counts": counts, "statuses": statuses},
                row_count=_first_known_value(detail_count, norm_count),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "counts": counts,
                "reconciliation": statuses,
                "field_quality": field_quality,
                "all_transaction_indexes_matched": bool(statuses) and all(value == "matched" for value in statuses.values()),
                "warnings": warnings,
                "query_id": query_id,
            }
        finally:
            engine.close()

    def audit_case_data_quality(self, case_id: str, *, example_limit: int = 10) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        limit = max(1, min(int(example_limit or 10), 50))
        try:
            self.ensure_analysis_schema(engine)
            detail_materialization_current = False
            try:
                detail_materialization_current = self._daily_agg.ensure_materialized(case_id, engine=engine) is True
            except Exception:
                detail_materialization_current = False

            warnings: list[dict[str, Any]] = []
            if not detail_materialization_current:
                warnings.append(
                    {
                        "code": "SAME_FACT_MATERIALIZATION_UNAVAILABLE",
                        "message": "交易明细物化未通过当前性校验；同事实重复族和候选金额保持未解析。",
                        "severity": "warning",
                    }
                )

            def _normalize_count_fields(
                row: Mapping[str, Any],
                fields: Iterable[str],
            ) -> dict[str, Any]:
                normalized = dict(row or {})
                for field in fields:
                    normalized[field] = _optional_nonnegative_int(normalized.get(field))
                return normalized

            table_counts: dict[str, int | None] = {}
            for table_name in (
                "import_file_log",
                "cleaning_log",
                "fc_transaction_raw",
                "fc_transaction_norm",
                "analysis_txn_detail_idx",
                "analysis_rule_txn_idx",
                "analysis_txn_daily_agg",
                "analysis_account_dim",
            ):
                if not self._table_exists(engine, table_name):
                    table_counts[table_name] = None
                    continue
                rows = engine.query(f"SELECT COUNT(1) FROM {table_name}")
                table_counts[table_name] = _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None

            import_summary: dict[str, Any] = {}
            import_by_kind: list[dict[str, Any]] = []
            duplicate_file_groups: list[dict[str, Any]] = []
            if self._table_exists(engine, "import_file_log"):
                import_columns = self._table_columns(engine, "import_file_log")
                import_count_columns = (
                    "rows_total",
                    "rows_imported_raw",
                    "rows_imported_norm",
                    "rows_dedup",
                    "rows_error",
                    "rows_skipped_non_data",
                )

                def _sum_import_column(column_name: str, alias: str) -> str:
                    if column_name not in import_columns:
                        return f"NULL AS {alias}, NULL AS {alias}_present"
                    quoted = _quote_identifier(column_name)
                    return f"SUM({quoted}) AS {alias}, COUNT({quoted}) AS {alias}_present"

                def _normalize_import_aggregate(
                    row: Mapping[str, Any],
                    fields: Sequence[str] = import_count_columns,
                ) -> dict[str, Any]:
                    normalized = dict(row or {})
                    file_log_count = _optional_nonnegative_int(normalized.get("file_log_count"))
                    normalized["file_log_count"] = file_log_count
                    for field in fields:
                        normalized[field] = _complete_aggregate_count(
                            normalized.get(field),
                            expected_count=file_log_count,
                            present_count=normalized.get(f"{field}_present"),
                        )
                        normalized.pop(f"{field}_present", None)
                    return normalized

                import_summary_rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      COUNT(1) AS file_log_count,
                      {_sum_import_column("rows_total", "rows_total")},
                      {_sum_import_column("rows_imported_raw", "rows_imported_raw")},
                      {_sum_import_column("rows_imported_norm", "rows_imported_norm")},
                      {_sum_import_column("rows_dedup", "rows_dedup")},
                      {_sum_import_column("rows_error", "rows_error")},
                      {_sum_import_column("rows_skipped_non_data", "rows_skipped_non_data")}
                    FROM import_file_log
                    """
                )
                import_summary = _normalize_import_aggregate(import_summary_rows[0] if import_summary_rows else {})
                import_by_kind = [
                    _normalize_import_aggregate(row)
                    for row in self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      kind,
                      COUNT(1) AS file_log_count,
                      {_sum_import_column("rows_total", "rows_total")},
                      {_sum_import_column("rows_imported_raw", "rows_imported_raw")},
                      {_sum_import_column("rows_imported_norm", "rows_imported_norm")},
                      {_sum_import_column("rows_dedup", "rows_dedup")},
                      {_sum_import_column("rows_error", "rows_error")},
                      {_sum_import_column("rows_skipped_non_data", "rows_skipped_non_data")}
                    FROM import_file_log
                    GROUP BY kind
                    ORDER BY rows_total DESC, kind
                    """
                    )
                ]
                duplicate_file_groups = [
                    _normalize_import_aggregate(
                        row,
                        tuple(field for field in import_count_columns if field != "rows_error"),
                    )
                    for row in self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      kind,
                      filename,
                      COUNT(1) AS file_log_count,
                      {_sum_import_column("rows_total", "rows_total")},
                      {_sum_import_column("rows_imported_raw", "rows_imported_raw")},
                      {_sum_import_column("rows_imported_norm", "rows_imported_norm")},
                      {_sum_import_column("rows_dedup", "rows_dedup")},
                      {_sum_import_column("rows_skipped_non_data", "rows_skipped_non_data")}
                    FROM import_file_log
                    GROUP BY kind, filename
                    HAVING COUNT(1) > 1
                    ORDER BY rows_total DESC, file_log_count DESC
                    LIMIT ?
                    """,
                    (limit,),
                    )
                ]
                rows_dedup = _optional_nonnegative_int(import_summary.get("rows_dedup"))
                if rows_dedup is not None and rows_dedup > 0:
                    warnings.append(
                        {
                            "code": "IMPORT_DEDUP_SIGNIFICANT",
                            "message": "导入台账存在去重行；资金统计必须说明采用规范表口径，不能把 rows_total 直接当作有效流水。",
                            "severity": "warning",
                        }
                    )

            cleaning_flags: dict[str, Any] = {}
            field_quality: dict[str, Any] = {}
            transaction_time_quality: dict[str, Any] = {}
            if self._table_exists(engine, "fc_transaction_norm"):
                norm_columns = self._table_columns(engine, "fc_transaction_norm")

                def _flag_count_sql(column_name: str, alias: str) -> str:
                    if column_name not in norm_columns:
                        return f"NULL AS {alias}"
                    quoted = _quote_identifier(column_name)
                    return (
                        f"CASE WHEN COUNT(1)>0 AND COUNT({quoted})=COUNT(1) "
                        f"THEN COUNT(1) FILTER (WHERE {quoted}=1) ELSE NULL END AS {alias}"
                    )

                def _blank_count_sql(column_names: Sequence[str], alias: str) -> str:
                    available = [_quote_identifier(name) for name in column_names if name in norm_columns]
                    if not available:
                        return f"NULL AS {alias}"
                    return (
                        f"CASE WHEN COUNT(1)>0 THEN COUNT(1) FILTER "
                        f"(WHERE COALESCE({', '.join(available)}, '')='') ELSE NULL END AS {alias}"
                    )

                def _blank_presence_expr(column_names: Sequence[str]) -> str:
                    terms = [
                        f"NULLIF(TRIM(CAST({_quote_identifier(name)} AS VARCHAR)), '')"
                        for name in column_names
                        if name in norm_columns
                    ]
                    return "COALESCE(" + ", ".join(terms) + ", '')" if terms else "''"

                counterparty_name_presence_expr = _blank_presence_expr(["counterparty_name"])
                counterparty_account_presence_expr = _blank_presence_expr(["counterparty_acct_norm", "counterparty_acct"])
                account_identity_terms = [
                    f"NULLIF({_quote_identifier(name)}, '')"
                    for name in ("card_no_norm", "acct_no_norm", "card_no", "acct_no", "account_no")
                    if name in norm_columns
                ]
                if account_identity_terms:
                    blank_holder_account_sql = (
                        "COUNT(DISTINCT COALESCE("
                        + ", ".join(account_identity_terms)
                        + ", '')) FILTER (WHERE COALESCE("
                        + (_quote_identifier("account_open_name") if "account_open_name" in norm_columns else "''")
                        + ", '')='') AS txn_blank_holder_account_count"
                    )
                else:
                    blank_holder_account_sql = "NULL AS txn_blank_holder_account_count"
                cleaning_rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      COUNT(1) AS txn_total,
                      {_flag_count_sql("clean_duplicate", "clean_duplicate")},
                      {_flag_count_sql("clean_failed", "clean_failed")},
                      {_flag_count_sql("clean_reversal", "clean_reversal")},
                      {_flag_count_sql("clean_dc_inferred", "clean_dc_inferred")},
                      {_flag_count_sql("clean_card_filled", "clean_card_filled")},
                      {_flag_count_sql("clean_account_filled", "clean_account_filled")},
                      {_flag_count_sql("clean_amt_failed", "clean_amt_failed")},
                      {_flag_count_sql("clean_bal_failed", "clean_bal_failed")},
                      {_blank_count_sql(["account_open_name"], "txn_blank_holder_rows")},
                      {blank_holder_account_sql},
                      {_blank_count_sql(["counterparty_name"], "blank_counterparty_name_rows")},
                      {_blank_count_sql(["counterparty_acct_norm", "counterparty_acct"], "blank_counterparty_account_rows")},
                      CASE WHEN COUNT(1)>0 THEN COUNT(1) FILTER (WHERE {counterparty_name_presence_expr}='' AND {counterparty_account_presence_expr}='') ELSE NULL END AS blank_counterparty_both_rows,
                      {_blank_count_sql(["dc_final", "dc_norm", "dc_flag", "dc"], "blank_direction_rows")}
                    FROM fc_transaction_norm
                    """
                )
                cleaning_count_fields = (
                    "txn_total",
                    "clean_duplicate",
                    "clean_failed",
                    "clean_reversal",
                    "clean_dc_inferred",
                    "clean_card_filled",
                    "clean_account_filled",
                    "clean_amt_failed",
                    "clean_bal_failed",
                    "txn_blank_holder_rows",
                    "txn_blank_holder_account_count",
                    "blank_counterparty_name_rows",
                    "blank_counterparty_account_rows",
                    "blank_counterparty_both_rows",
                    "blank_direction_rows",
                )
                cleaning_flags = _normalize_count_fields(
                    cleaning_rows[0] if cleaning_rows else {},
                    cleaning_count_fields,
                )
                field_quality = self._field_presence_rates(
                    engine,
                    table_name="fc_transaction_norm",
                    columns=[
                        "account_open_name",
                        "opener_id_no",
                        "card_no_norm",
                        "acct_no_norm",
                        "txn_ts",
                        "amount_val",
                        "balance_val",
                        "dc_final",
                        "counterparty_acct_norm",
                        "counterparty_name",
                        "counterparty_bank",
                        "summary",
                        "remark",
                        "ip_addr",
                        "mac_addr",
                    ],
                    available_columns=norm_columns,
                )
                blank_counterparty_name_rows = _optional_nonnegative_int(cleaning_flags.get("blank_counterparty_name_rows"))
                blank_counterparty_account_rows = _optional_nonnegative_int(cleaning_flags.get("blank_counterparty_account_rows"))
                if (
                    blank_counterparty_name_rows is not None and blank_counterparty_name_rows > 0
                ) or (
                    blank_counterparty_account_rows is not None and blank_counterparty_account_rows > 0
                ):
                    warnings.append(
                        {
                            "code": "COUNTERPARTY_FIELD_GAPS",
                            "message": "部分交易缺失对手户名或对手账号；资金去向只能写到工具可证实层级，不能补写最终收款人。",
                            "severity": "warning",
                        }
                    )
            if self._table_exists(engine, "analysis_txn_detail_idx"):
                detail_columns = self._table_columns(engine, "analysis_txn_detail_idx")
                if "txn_ts" in detail_columns:
                    time_rows = self._query_dicts(
                        engine,
                        """
                        WITH tx AS (
                          SELECT TRY_CAST(txn_ts AS TIMESTAMP) AS txn_ts
                          FROM analysis_txn_detail_idx
                        )
                        SELECT
                          CAST(MIN(txn_ts) AS VARCHAR) AS txn_time_min,
                          CAST(MAX(txn_ts) AS VARCHAR) AS txn_time_max,
                          COUNT(1) FILTER (WHERE txn_ts IS NOT NULL AND txn_ts < TIMESTAMP '1900-01-01') AS abnormal_early_txn_time_rows,
                          COUNT(1) FILTER (WHERE txn_ts IS NULL) AS missing_txn_time_rows
                        FROM tx
                        """
                    )
                    transaction_time_quality = _normalize_count_fields(
                        time_rows[0] if time_rows else {},
                        ("abnormal_early_txn_time_rows", "missing_txn_time_rows"),
                    )
                    abnormal_early_rows = _optional_nonnegative_int(
                        transaction_time_quality.get("abnormal_early_txn_time_rows")
                    )
                    if abnormal_early_rows is not None and abnormal_early_rows > 0:
                        warnings.append(
                            {
                                "code": "ABNORMAL_EARLY_TXN_TIME_BOUNDARY",
                                "message": "发现早于 1900 年的解析交易时间；这是时间字段质量边界，不能作为正常交易年份或真实发生时间结论。",
                                "severity": "warning",
                            }
                        )

            account_identity_quality: dict[str, Any] = {
                "account_dim_available": False,
                "account_count": None,
                "registered_account_count": None,
                "unregistered_account_count": None,
                "unregistered_txn_count": None,
                "unregistered_turnover_total": None,
                "txn_blank_holder_account_count": _optional_nonnegative_int(cleaning_flags.get("txn_blank_holder_account_count")),
                "txn_blank_holder_rows": _optional_nonnegative_int(cleaning_flags.get("txn_blank_holder_rows")),
                "txn_blank_holder_resolved_account_count": None,
                "txn_blank_holder_unregistered_account_count": None,
                "txn_blank_holder_resolution_examples": [],
                "unregistered_accounts": [],
            }
            if self._table_exists(engine, "analysis_account_dim"):
                account_identity_quality["account_dim_available"] = True
                identity_rows = self._query_dicts(
                    engine,
                    """
                    WITH dim AS (
                      SELECT
                        account_key,
                        COALESCE(NULLIF(TRIM(open_name), ''), '') AS open_name,
                        COALESCE(NULLIF(TRIM(id_no), ''), '') AS id_no
                      FROM analysis_account_dim
                    ),
                    tx AS (
                      SELECT
                        acct_key,
                        COUNT(1) AS txn_count,
                        COUNT(amount) AS amount_present_count,
                        SUM(ABS(amount)) AS turnover_total
                      FROM analysis_txn_detail_idx
                      GROUP BY acct_key
                    )
                    SELECT
                      COUNT(1) AS account_count,
                      COUNT(1) FILTER (WHERE open_name <> '') AS registered_account_count,
                      COUNT(1) FILTER (WHERE open_name = '') AS unregistered_account_count,
                      SUM(tx.txn_count) FILTER (WHERE open_name = '') AS unregistered_txn_count,
                      SUM(tx.amount_present_count) FILTER (WHERE open_name = '') AS unregistered_amount_present_count,
                      SUM(tx.turnover_total) FILTER (WHERE open_name = '') AS unregistered_turnover_total
                    FROM dim
                    LEFT JOIN tx ON tx.acct_key = dim.account_key
                    """
                )
                if identity_rows:
                    identity = identity_rows[0]
                    unregistered_txn_count = _optional_nonnegative_int(identity.get("unregistered_txn_count"))
                    account_identity_quality.update(
                        {
                            "account_count": _optional_nonnegative_int(identity.get("account_count")),
                            "registered_account_count": _optional_nonnegative_int(identity.get("registered_account_count")),
                            "unregistered_account_count": _optional_nonnegative_int(identity.get("unregistered_account_count")),
                            "unregistered_txn_count": unregistered_txn_count,
                            "unregistered_turnover_total": _complete_aggregate_amount(
                                identity.get("unregistered_turnover_total"),
                                expected_count=unregistered_txn_count,
                                present_count=identity.get("unregistered_amount_present_count"),
                            ),
                        }
                    )
                unregistered_account_rows = self._query_dicts(
                    engine,
                    """
                    SELECT
                      d.account_key,
                      COALESCE(NULLIF(TRIM(d.open_name), ''), '') AS open_name,
                      COALESCE(NULLIF(TRIM(d.id_no), ''), '') AS id_no,
                      tx.txn_count AS txn_count,
                      tx.amount_present_count AS amount_present_count,
                      tx.turnover_total AS turnover_total
                    FROM analysis_account_dim d
                    LEFT JOIN (
                      SELECT acct_key, COUNT(1) AS txn_count, COUNT(amount) AS amount_present_count, SUM(ABS(amount)) AS turnover_total
                      FROM analysis_txn_detail_idx
                      GROUP BY acct_key
                    ) tx ON tx.acct_key = d.account_key
                    WHERE COALESCE(NULLIF(TRIM(d.open_name), ''), '') = ''
                    ORDER BY tx.turnover_total DESC NULLS LAST, d.account_key
                    LIMIT ?
                    """,
                    (limit,),
                )
                account_identity_quality["unregistered_accounts"] = [
                    {
                        **row,
                        "txn_count": (txn_count := _optional_nonnegative_int(row.get("txn_count"))),
                        "turnover_total": _complete_aggregate_amount(
                            row.get("turnover_total"),
                            expected_count=txn_count,
                            present_count=row.get("amount_present_count"),
                        ),
                    }
                    for row in unregistered_account_rows
                ]
                for row in account_identity_quality["unregistered_accounts"]:
                    row.pop("amount_present_count", None)
                if self._table_exists(engine, "fc_transaction_norm"):
                    blank_txn_account_expr = (
                        "COALESCE(" + ", ".join(account_identity_terms) + ", '')"
                        if account_identity_terms
                        else "''"
                    )
                    blank_txn_holder_expr = _quote_identifier("account_open_name") if "account_open_name" in norm_columns else "''"
                    resolution_rows = self._query_dicts(
                        engine,
                        f"""
                        WITH blank_txn_accounts AS (
                          SELECT DISTINCT {blank_txn_account_expr} AS account_key
                          FROM fc_transaction_norm
                          WHERE COALESCE({blank_txn_holder_expr}, '') = ''
                        ),
                        resolved AS (
                          SELECT
                            b.account_key,
                            COALESCE(NULLIF(TRIM(d.open_name), ''), '') AS open_name,
                            COALESCE(NULLIF(TRIM(d.id_no), ''), '') AS id_no
                          FROM blank_txn_accounts b
                          LEFT JOIN analysis_account_dim d ON d.account_key = b.account_key
                          WHERE b.account_key <> ''
                        )
                        SELECT
                          COUNT(1) AS txn_blank_holder_account_count,
                          COUNT(1) FILTER (WHERE open_name <> '') AS txn_blank_holder_resolved_account_count,
                          COUNT(1) FILTER (WHERE open_name = '') AS txn_blank_holder_unregistered_account_count
                        FROM resolved
                        """
                    )
                    if resolution_rows:
                        resolution = resolution_rows[0]
                        account_identity_quality.update(
                            {
                                "txn_blank_holder_account_count": _optional_nonnegative_int(resolution.get("txn_blank_holder_account_count")),
                                "txn_blank_holder_resolved_account_count": _optional_nonnegative_int(resolution.get("txn_blank_holder_resolved_account_count")),
                                "txn_blank_holder_unregistered_account_count": _optional_nonnegative_int(resolution.get("txn_blank_holder_unregistered_account_count")),
                            }
                    )
                    account_identity_quality["txn_blank_holder_resolution_examples"] = self._query_dicts(
                        engine,
                        f"""
                        WITH blank_txn_accounts AS (
                          SELECT DISTINCT {blank_txn_account_expr} AS account_key
                          FROM fc_transaction_norm
                          WHERE COALESCE({blank_txn_holder_expr}, '') = ''
                        )
                        SELECT
                          b.account_key,
                          COALESCE(NULLIF(TRIM(d.open_name), ''), '') AS resolved_open_name,
                          COALESCE(NULLIF(TRIM(d.id_no), ''), '') AS resolved_id_no
                        FROM blank_txn_accounts b
                        LEFT JOIN analysis_account_dim d ON d.account_key = b.account_key
                        WHERE b.account_key <> ''
                        ORDER BY resolved_open_name DESC, b.account_key
                        LIMIT ?
                        """,
                        (limit,),
                    )
                cleaning_flags["blank_holder_account_count"] = account_identity_quality["unregistered_account_count"]
                cleaning_flags["blank_holder_rows"] = account_identity_quality["unregistered_txn_count"]
                cleaning_flags["txn_blank_holder_account_count"] = account_identity_quality["txn_blank_holder_account_count"]
                cleaning_flags["txn_blank_holder_rows"] = account_identity_quality["txn_blank_holder_rows"]
                unregistered_accounts = _optional_nonnegative_int(account_identity_quality.get("unregistered_account_count"))
                if unregistered_accounts is not None and unregistered_accounts > 0:
                    warnings.append(
                        {
                            "code": "ACCOUNT_DIM_UNREGISTERED_ACCOUNTS_PRESENT",
                            "message": f"账户维表统计到 {unregistered_accounts} 个未登记户名账户；这是户名归属判断的权威口径，不能用交易明细空户名字段替代。",
                            "severity": "warning",
                        }
                    )
                resolved_blank_accounts = _optional_nonnegative_int(account_identity_quality.get("txn_blank_holder_resolved_account_count"))
                if resolved_blank_accounts is not None and resolved_blank_accounts > 0:
                    warnings.append(
                        {
                            "code": "TXN_BLANK_HOLDER_RESOLVED_BY_ACCOUNT_DIM",
                            "message": f"交易明细中有 {resolved_blank_accounts} 个空户名账号已由账户维表解析出户名；不能把明细空户名直接写成未登记户名账户。",
                            "severity": "warning",
                        }
                    )

            duplicate_summary: dict[str, Any] = {
                "row_hash_duplicate_groups": None,
                "row_hash_extra_rows": None,
                "same_txn_id_cross_account_groups": None,
                "same_txn_id_cross_account_extra_rows": None,
                "same_fact_cross_account_groups": None,
                "same_fact_cross_account_extra_rows": None,
                "same_holder_same_fact_groups": None,
                "same_holder_same_fact_extra_rows": None,
                "same_holder_same_fact_candidate_duplicate_amount": None,
                "full_case_same_fact_cross_holder_groups": None,
                "natural_duplicate_within_account_groups": None,
                "natural_duplicate_within_account_extra_rows": None,
            }
            duplicate_examples: dict[str, list[dict[str, Any]]] = {
                "same_txn_id_cross_account": [],
                "same_fact_cross_account": [],
                "natural_duplicate_within_account": [],
            }
            if self._table_exists(engine, "fc_transaction_norm"):
                def _string_key_sql(column_names: Sequence[str]) -> str:
                    terms = [
                        f"NULLIF(TRIM(CAST({_quote_identifier(name)} AS VARCHAR)), '')"
                        for name in column_names
                        if name in norm_columns
                    ]
                    return "COALESCE(" + ", ".join(terms) + ", '')" if terms else "''"

                def _number_key_sql(column_names: Sequence[str]) -> str:
                    return _first_finite_number_sql(norm_columns, column_names)

                row_hash_expr = _string_key_sql(["row_hash"])
                account_key_expr = _string_key_sql(["card_no_norm", "acct_no_norm", "card_no", "acct_no", "account_no"])
                holder_name_expr = _string_key_sql(["account_open_name", "open_name", "holder_name"])
                txn_time_expr = _string_key_sql(["txn_ts", "txn_time", "交易时间"])
                amount_number_expr = _number_key_sql(["amount_val", "amount", "金额"])
                amount_text_expr = (
                    f"CASE WHEN ({amount_number_expr}) IS NOT NULL "
                    f"THEN printf('%.2f', ({amount_number_expr})) ELSE '' END"
                )
                direction_expr = _string_key_sql(["dc_final", "dc_norm", "dc_flag", "dc", "direction"])
                counterparty_account_expr = _string_key_sql(["counterparty_acct_norm", "counterparty_acct", "counterparty_account"])
                counterparty_name_expr = _string_key_sql(["counterparty_name"])
                txn_id_expr = _string_key_sql(["txn_id", "log_no"])
                row_no_expr = _quote_identifier("row_no") if "row_no" in norm_columns else "NULL::BIGINT"
                row_numbers_expr = (
                    f"list({row_no_expr} ORDER BY {row_no_expr}) FILTER (WHERE {row_no_expr} IS NOT NULL)[:8]"
                    if "row_no" in norm_columns
                    else "[]"
                )
                duplicate_rows = self._query_dicts(
                    engine,
                    f"""
                    WITH row_hash_dup AS (
                      SELECT {row_hash_expr} AS row_hash, COUNT(1) AS c
                      FROM fc_transaction_norm
                      WHERE {row_hash_expr} <> ''
                      GROUP BY {row_hash_expr}
                      HAVING COUNT(1) > 1
                    ),
                    same_txn_id_cross_account AS (
                      SELECT {txn_id_expr} AS txn_id, COUNT(1) AS c, COUNT(DISTINCT {account_key_expr}) AS account_count
                      FROM fc_transaction_norm
                      WHERE {_valid_txn_id_sql(txn_id_expr)}
                        AND {_valid_txn_id_sql(account_key_expr)}
                      GROUP BY {txn_id_expr}
                      HAVING COUNT(1) > 1 AND COUNT(DISTINCT {account_key_expr}) > 1
                    ),
                    natural_duplicate_within_account AS (
                      SELECT
                        {account_key_expr} AS account_key,
                        {txn_time_expr} AS txn_time_key,
                        {amount_text_expr} AS amount_key,
                        {direction_expr} AS direction_key,
                        {counterparty_account_expr} AS cp_account_key,
                        {txn_id_expr} AS txn_id_key,
                        COUNT(1) AS c
                      FROM fc_transaction_norm
                      WHERE {account_key_expr} <> ''
                        AND {txn_time_expr} <> ''
                        AND {amount_text_expr} <> ''
                        AND ({amount_number_expr}) IS NOT NULL
                        AND {direction_expr} <> ''
                      GROUP BY 1,2,3,4,5,6
                      HAVING COUNT(1) > 1
                    )
                    SELECT
                      (SELECT COUNT(1) FROM row_hash_dup) AS row_hash_duplicate_groups,
                      (SELECT SUM(c - 1) FROM row_hash_dup) AS row_hash_extra_rows,
                      (SELECT COUNT(1) FROM same_txn_id_cross_account) AS same_txn_id_cross_account_groups,
                      (SELECT SUM(c - 1) FROM same_txn_id_cross_account) AS same_txn_id_cross_account_extra_rows,
                      (SELECT COUNT(1) FROM natural_duplicate_within_account) AS natural_duplicate_within_account_groups,
                      (SELECT SUM(c - 1) FROM natural_duplicate_within_account) AS natural_duplicate_within_account_extra_rows
                    """
                )
                if duplicate_rows:
                    duplicate_summary.update(
                        {
                            key: _optional_nonnegative_int(value)
                            for key, value in duplicate_rows[0].items()
                        }
                    )
                duplicate_examples["same_txn_id_cross_account"] = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      {txn_id_expr} AS txn_id,
                      COUNT(1) AS row_count,
                      COUNT(DISTINCT {account_key_expr}) AS account_count,
                      CASE WHEN COUNT({amount_number_expr})=COUNT(1) THEN SUM(ABS({amount_number_expr})) ELSE NULL END AS amount_sum,
                      CASE WHEN COUNT({amount_number_expr})=COUNT(1) THEN MAX(ABS({amount_number_expr})) ELSE NULL END AS max_amount,
                      MIN({txn_time_expr}) AS first_time,
                      MAX({txn_time_expr}) AS last_time,
                      list(DISTINCT {account_key_expr})[:8] AS account_keys,
                      list(DISTINCT COALESCE(NULLIF({holder_name_expr}, ''), '未识别户名'))[:8] AS holder_names
                    FROM fc_transaction_norm
                    WHERE {_valid_txn_id_sql(txn_id_expr)}
                      AND {_valid_txn_id_sql(account_key_expr)}
                    GROUP BY {txn_id_expr}
                    HAVING COUNT(1) > 1 AND COUNT(DISTINCT {account_key_expr}) > 1
                    ORDER BY max_amount DESC, row_count DESC
                    LIMIT ?
                    """,
                    (limit,),
                )
                duplicate_examples["natural_duplicate_within_account"] = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      {account_key_expr} AS account_key,
                      ANY_VALUE(COALESCE(NULLIF({holder_name_expr}, ''), '未识别户名')) AS holder_name,
                      {txn_time_expr} AS txn_time,
                      {amount_text_expr} AS amount,
                      {direction_expr} AS direction,
                      {counterparty_account_expr} AS counterparty_account,
                      ANY_VALUE({counterparty_name_expr}) AS counterparty_name,
                      {txn_id_expr} AS txn_id,
                      COUNT(1) AS row_count,
                      {row_numbers_expr} AS row_numbers,
                      MAX(ABS({amount_number_expr})) AS max_amount
                    FROM fc_transaction_norm
                      WHERE {account_key_expr} <> ''
                        AND {txn_time_expr} <> ''
                        AND {amount_text_expr} <> ''
                      AND ({amount_number_expr}) IS NOT NULL
                        AND {direction_expr} <> ''
                    GROUP BY 1,3,4,5,6,8
                    HAVING COUNT(1) > 1
                    ORDER BY row_count DESC, max_amount DESC
                    LIMIT ?
                    """,
                    (limit,),
                )
                for rows in duplicate_examples.values():
                    for row in rows:
                        for count_key in ("row_count", "account_count"):
                            if count_key in row:
                                row[count_key] = _optional_nonnegative_int(row.get(count_key))
                        for amount_key in ("amount_sum", "max_amount"):
                            if amount_key in row:
                                row[amount_key] = _optional_rounded_float(row.get(amount_key), 2)
                clean_duplicate = _optional_nonnegative_int(cleaning_flags.get("clean_duplicate"))
                natural_duplicate_extra_rows = _optional_nonnegative_int(
                    duplicate_summary.get("natural_duplicate_within_account_extra_rows")
                )
                if clean_duplicate == 0 and (
                    natural_duplicate_extra_rows is not None and natural_duplicate_extra_rows > 0
                ):
                    warnings.append(
                        {
                            "code": "NATURAL_DUPLICATES_NOT_MARKED_BY_CLEANING",
                            "message": "清洗标记 clean_duplicate 为 0，但按账户、交易时间、金额、方向、对手账号和交易号可发现自然重复候选；不能仅依赖全项重复清洗规则判断无重复。",
                            "severity": "warning",
                        }
                    )
            same_fact_eligibility = _project_same_fact_eligibility(None, None)
            canonical_same_fact_tables_available = (
                self._table_exists(engine, "analysis_txn_detail_idx")
                and self._table_exists(engine, "analysis_account_dim")
            )
            if detail_materialization_current and canonical_same_fact_tables_available:
                base_cte = _same_fact_family_cte_sql("TRUE")
                same_fact_summary_rows = self._query_dicts(
                    engine,
                    base_cte
                    + """
                    SELECT
                      (SELECT COUNT(1) FROM scoped_input) AS requested_row_count,
                      (SELECT COUNT(1) FROM scoped_txn WHERE fact_rejection_reason IS NULL) AS fact_eligible_row_count,
                      (SELECT COUNT(1) FROM eligible_txn) AS family_eligible_row_count,
                      (SELECT COUNT(1) FROM scoped_txn WHERE rejection_reason IS NOT NULL) AS rejected_row_count,
                      (SELECT COUNT(1) FROM same_holder_families) AS same_holder_group_count,
                      (SELECT COALESCE(SUM(row_count - 1), 0) FROM same_holder_families) AS same_holder_extra_rows,
                      (SELECT COALESCE(SUM(candidate_duplicate_amount), 0.0) FROM same_holder_families) AS same_holder_candidate_duplicate_amount,
                      (SELECT COUNT(1) FROM selected_scope_families) AS full_case_group_count,
                      (SELECT COALESCE(SUM(row_count - 1), 0) FROM selected_scope_families) AS full_case_extra_rows,
                      (SELECT COUNT(1) FROM selected_scope_families WHERE holder_identity_count > 1) AS full_case_cross_holder_group_count
                    """,
                )
                same_fact_summary = same_fact_summary_rows[0] if same_fact_summary_rows else {}
                same_fact_reason_rows = (
                    self._query_dicts(
                        engine,
                        base_cte
                        + """
                        SELECT rejection_reason, COUNT(1) AS row_count
                        FROM scoped_txn
                        WHERE rejection_reason IS NOT NULL
                        GROUP BY rejection_reason
                        ORDER BY rejection_reason
                        """,
                    )
                    if same_fact_summary_rows
                    else None
                )
                same_fact_eligibility = _project_same_fact_eligibility(
                    same_fact_summary,
                    same_fact_reason_rows,
                )
                if same_fact_eligibility["coverage_status"] == "complete":
                    same_holder_group_count = _exact_nonnegative_db_int(
                        same_fact_summary.get("same_holder_group_count")
                    )
                    same_holder_extra_rows = _exact_nonnegative_db_int(
                        same_fact_summary.get("same_holder_extra_rows")
                    )
                    same_holder_candidate_amount = _optional_rounded_float(
                        same_fact_summary.get("same_holder_candidate_duplicate_amount"),
                        2,
                    )
                    full_case_group_count = _exact_nonnegative_db_int(
                        same_fact_summary.get("full_case_group_count")
                    )
                    full_case_extra_rows = _exact_nonnegative_db_int(
                        same_fact_summary.get("full_case_extra_rows")
                    )
                    full_case_cross_holder_groups = _exact_nonnegative_db_int(
                        same_fact_summary.get("full_case_cross_holder_group_count")
                    )
                    if (
                        same_holder_group_count is None
                        or same_holder_extra_rows is None
                        or same_holder_candidate_amount is None
                        or full_case_group_count is None
                        or full_case_extra_rows is None
                        or full_case_cross_holder_groups is None
                    ):
                        same_fact_eligibility = _project_same_fact_eligibility(None, None)
                        same_fact_eligibility["blocker"] = "same_fact_family_summary_invalid"
                    else:
                        duplicate_summary.update(
                            {
                                "same_fact_cross_account_groups": full_case_group_count,
                                "same_fact_cross_account_extra_rows": full_case_extra_rows,
                                "same_holder_same_fact_groups": same_holder_group_count,
                                "same_holder_same_fact_extra_rows": same_holder_extra_rows,
                                "same_holder_same_fact_candidate_duplicate_amount": same_holder_candidate_amount,
                                "full_case_same_fact_cross_holder_groups": full_case_cross_holder_groups,
                            }
                        )
                        duplicate_examples["same_fact_cross_account"] = self._query_dicts(
                            engine,
                            base_cte
                            + """
                            SELECT
                              holder_name,
                              id_no,
                              holder_identity_count,
                              txn_time_key AS txn_time,
                              amount_key AS amount,
                              balance_key AS balance,
                              direction_key AS direction,
                              counterparty_account_key AS counterparty_account,
                              counterparty_name_key AS counterparty_name,
                              summary_key AS summary,
                              remark_key AS remark,
                              txn_type_key AS txn_type,
                              row_count,
                              account_count,
                              account_keys,
                              holder_names,
                              txn_ids,
                              canonical_abs_amount AS max_amount
                            FROM selected_scope_families
                            ORDER BY canonical_abs_amount DESC, row_count DESC, txn_time_key
                            LIMIT ?
                            """,
                            (limit,),
                        )
                        for row in duplicate_examples["same_fact_cross_account"]:
                            row["holder_identity_count"] = _optional_nonnegative_int(
                                row.get("holder_identity_count")
                            )
                            row["row_count"] = _optional_nonnegative_int(row.get("row_count"))
                            row["account_count"] = _optional_nonnegative_int(row.get("account_count"))
                            row["max_amount"] = _optional_rounded_float(row.get("max_amount"), 2)
                        if full_case_extra_rows > 0:
                            warnings.append(
                                {
                                    "code": "SAME_FACT_CROSS_ACCOUNT_DUPLICATE_CANDIDATES",
                                    "message": "发现通过 SameFactEligibilityV1 完整校验的跨账户同事实重复候选；换卡/补卡确认前不得直接扣减金额。",
                                    "severity": "warning",
                                }
                            )
                        if full_case_cross_holder_groups > 0:
                            warnings.append(
                                {
                                    "code": "FULL_CASE_BLIND_DEDUPE_NOT_ALLOWED",
                                    "message": "发现跨户名同事实候选族；不能按全案字段相同直接去重，最低应约束到同一户名/同一证件/显式账户集合。",
                                    "severity": "warning",
                                }
                            )
                        clean_duplicate = _optional_nonnegative_int(cleaning_flags.get("clean_duplicate"))
                        if clean_duplicate == 0 and full_case_extra_rows > 0:
                            warnings.append(
                                {
                                    "code": "NATURAL_DUPLICATES_NOT_MARKED_BY_CLEANING",
                                    "message": "清洗标记 clean_duplicate 为 0，但 SameFactEligibilityV1 已确认存在重复候选；不能仅依赖全项重复清洗规则判断无重复。",
                                    "severity": "warning",
                                }
                            )
            if same_fact_eligibility["coverage_status"] != "complete":
                warnings.append(
                    {
                        "code": "SAME_FACT_ELIGIBILITY_INCOMPLETE",
                        "message": "同事实资格校验未完整覆盖当前案件；重复族、候选差额和无重复结论均保持未解析。",
                        "severity": "warning",
                    }
                )

            materialization_meta: list[dict[str, Any]] = []
            if self._table_exists(engine, "analysis_materialization_meta"):
                meta_columns = self._table_columns(engine, "analysis_materialization_meta")
                source_signature_expr = "source_signature" if "source_signature" in meta_columns else "'' AS source_signature"
                materialization_meta = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      agg_name,
                      agg_version,
                      source_revision,
                      source_row_count,
                      CAST(source_max_txn_ts AS VARCHAR) AS source_max_txn_ts,
                      source_max_id,
                      CAST(built_at AS VARCHAR) AS built_at,
                      row_count,
                      {source_signature_expr}
                    FROM analysis_materialization_meta
                    ORDER BY agg_name
                    """,
                )

            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="audit_case_data_quality",
                params={"case_id": case_id, "example_limit": limit},
                summary={
                    "table_counts": table_counts,
                    "import_summary": import_summary,
                    "cleaning_flags": cleaning_flags,
                    "account_identity_quality": account_identity_quality,
                    "duplicate_summary": duplicate_summary,
                    "same_fact_eligibility": same_fact_eligibility,
                    "warning_count": len(warnings),
                },
                row_count=_first_known_value(
                    table_counts.get("analysis_txn_detail_idx"),
                    table_counts.get("fc_transaction_norm"),
                ),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "table_counts": table_counts,
                "import_lineage": {
                    "summary": import_summary,
                    "by_kind": import_by_kind,
                    "duplicate_filename_groups": duplicate_file_groups,
                },
                "cleaning_quality": {
                    "flags": cleaning_flags,
                    "field_quality": field_quality,
                    "transaction_time_quality": transaction_time_quality,
                    "duplicate_summary": duplicate_summary,
                    "duplicate_examples": duplicate_examples,
                    "same_fact_eligibility": same_fact_eligibility,
                    "materialization_meta": materialization_meta,
                },
                "account_identity_quality": account_identity_quality,
                "interpretation": {
                    "stats_scope_rule": "资金统计默认采用 fc_transaction_norm / analysis_txn_detail_idx 规范明细口径；当同事实重复候选存在时，报告必须说明是否采用同事实去重口径。",
                    "account_identity_rule": "户名归属以 analysis_account_dim 账户维表为准；fc_transaction_norm.account_open_name 为空只表示明细字段缺口，不能直接认定账户未登记户名。",
                    "ui_empty_holder_rule": "未登记户名账户数来自账户维表，不等于前端左侧树实际可见节点/分组数量；前端展示受 byName/byCard、搜索、折叠和 stats tree 返回 group 结构影响。",
                    "card_replacement_rule": "跨账户同事实重复候选只能提示换卡/补卡/银行记录差异，确认前不得直接删除或合并资金。",
                },
                "warnings": warnings,
                "query_id": query_id,
            }
        finally:
            engine.close()

    def resolve_duplicate_families(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        min_amount: float = 0.0,
        scope_mode: str = "same_holder_accounts",
        limit: int = 20,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        normalized_limit = max(1, min(int(limit or 20), 200))
        normalized_scope_mode = _trim(scope_mode).lower() or "same_holder_accounts"
        if normalized_scope_mode not in {"same_holder_accounts", "selected_scope", "full_case_review"}:
            raise ValueError("scope_mode must be same_holder_accounts, selected_scope, or full_case_review")
        min_amount_value = _as_float(min_amount)
        if min_amount_value < 0:
            raise ValueError("min_amount must be nonnegative")
        scope = self._resolve_rank_scope_accounts(
            case_id,
            account_keys=account_keys,
            account_key=account_key,
            card_no=card_no,
            acct_no=acct_no,
            holder_name=holder_name,
            id_no=id_no,
            match_mode=match_mode,
        )
        resolved_selected_accounts = _trim_list(scope.get("selected_accounts") or [])
        if normalized_scope_mode != "full_case_review" and not bool(scope.get("scope_requested")):
            raise ValueError(f"{normalized_scope_mode} requires an explicit holder, identity, or account scope")
        if normalized_scope_mode == "selected_scope" and not resolved_selected_accounts:
            raise ValueError("selected_scope requires at least one resolved account")
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            if self._daily_agg.ensure_materialized(case_id, engine=engine) is not True:
                raise TransactionFactSourceUnavailableError()
            selected_accounts = resolved_selected_accounts
            selected_account_preview_limit = 50
            selected_accounts_preview = selected_accounts[:selected_account_preview_limit]
            selected_accounts_truncated = len(selected_accounts) > len(selected_accounts_preview)
            where_sql, params = self._rank_scope_where(
                engine,
                case_id=case_id,
                selected_accounts=selected_accounts,
                scope_requested=bool(scope.get("scope_requested")),
                source_file_ids=[],
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter="all",
                cash_filter="all",
            )
            if min_amount_value > 0:
                where_sql = f"({where_sql}) AND t.amount IS NOT NULL AND ABS(t.amount) >= ?"
                params.append(min_amount_value)

            base_cte = _same_fact_family_cte_sql(where_sql)
            summary_rows = self._query_dicts(
                engine,
                base_cte
                + """
                SELECT
                  (SELECT COUNT(1) FROM scoped_input) AS requested_row_count,
                  (SELECT COUNT(1) FROM scoped_txn WHERE fact_rejection_reason IS NULL) AS fact_eligible_row_count,
                  (SELECT COUNT(1) FROM eligible_txn) AS family_eligible_row_count,
                  (SELECT COUNT(1) FROM scoped_txn WHERE rejection_reason IS NOT NULL) AS rejected_row_count,
                  (SELECT COUNT(DISTINCT account_key) FROM eligible_txn) AS eligible_account_count,
                  (SELECT COUNT(1) FROM same_holder_families) AS same_holder_group_count,
                  (SELECT SUM(row_count - 1) FROM same_holder_families) AS same_holder_extra_rows,
                  (SELECT SUM(candidate_duplicate_amount) FROM same_holder_families) AS same_holder_candidate_duplicate_amount,
                  (SELECT COUNT(1) FROM selected_scope_families) AS selected_scope_group_count,
                  (SELECT SUM(row_count - 1) FROM selected_scope_families) AS selected_scope_extra_rows,
                  (SELECT SUM(candidate_duplicate_amount) FROM selected_scope_families) AS selected_scope_candidate_duplicate_amount,
                  (SELECT COUNT(1) FROM selected_scope_families WHERE holder_identity_count > 1) AS selected_scope_cross_holder_group_count,
                  (SELECT COUNT(1) FROM full_case_review_families) AS full_case_group_count,
                  (SELECT SUM(row_count - 1) FROM full_case_review_families) AS full_case_extra_rows,
                  (SELECT SUM(candidate_duplicate_amount) FROM full_case_review_families) AS full_case_candidate_duplicate_amount,
                  (SELECT COUNT(1) FROM full_case_review_families WHERE holder_identity_count > 1) AS full_case_cross_holder_group_count
                """,
                tuple(params),
            )
            summary = summary_rows[0] if summary_rows else {}
            reason_rows = (
                self._query_dicts(
                    engine,
                    base_cte
                    + """
                    SELECT rejection_reason, COUNT(1) AS row_count
                    FROM scoped_txn
                    WHERE rejection_reason IS NOT NULL
                    GROUP BY rejection_reason
                    ORDER BY rejection_reason
                    """,
                    tuple(params),
                )
                if summary_rows
                else None
            )
            same_fact_eligibility = _project_same_fact_eligibility(summary, reason_rows)
            complete_same_fact_coverage = same_fact_eligibility["coverage_status"] == "complete"

            if normalized_scope_mode == "full_case_review":
                family_select = """
                  SELECT
                    same_fact_key,
                    '' AS holder_name,
                    '' AS id_no,
                    holder_identity_count,
                    txn_time_key,
                    amount_key,
                    balance_key,
                    direction_key,
                    counterparty_account_key,
                    counterparty_name_key,
                    summary_key,
                    remark_key,
                    txn_type_key,
                    row_count,
                    account_count,
                    NULL AS txn_id_count,
                    NULL AS source_file_count,
                    gross_abs_amount,
                    canonical_abs_amount,
                    candidate_duplicate_amount,
                    [] AS account_keys,
                    [] AS txn_ids,
                    [] AS source_file_ids
                  FROM full_case_review_families
                  ORDER BY canonical_abs_amount DESC, row_count DESC, txn_time_key
                  LIMIT ?
                """
            elif normalized_scope_mode == "selected_scope":
                family_select = """
                  SELECT
                    same_fact_key,
                    holder_name,
                    id_no,
                    holder_identity_count,
                    txn_time_key,
                    amount_key,
                    balance_key,
                    direction_key,
                    counterparty_account_key,
                    counterparty_name_key,
                    summary_key,
                    remark_key,
                    txn_type_key,
                    row_count,
                    account_count,
                    txn_id_count,
                    source_file_count,
                    gross_abs_amount,
                    canonical_abs_amount,
                    candidate_duplicate_amount,
                    account_keys,
                    txn_ids,
                    source_file_ids
                  FROM selected_scope_families
                  ORDER BY canonical_abs_amount DESC, row_count DESC, txn_time_key
                  LIMIT ?
                """
            else:
                family_select = """
                  SELECT
                    same_fact_key,
                    holder_name,
                    id_no,
                    1 AS holder_identity_count,
                    txn_time_key,
                    amount_key,
                    balance_key,
                    direction_key,
                    counterparty_account_key,
                    counterparty_name_key,
                    summary_key,
                    remark_key,
                    txn_type_key,
                    row_count,
                    account_count,
                    txn_id_count,
                    source_file_count,
                    gross_abs_amount,
                    canonical_abs_amount,
                    candidate_duplicate_amount,
                    account_keys,
                    txn_ids,
                    source_file_ids
                  FROM same_holder_families
                  ORDER BY canonical_abs_amount DESC, row_count DESC, txn_time_key
                  LIMIT ?
                """
            family_rows = (
                self._query_dicts(engine, base_cte + family_select, tuple([*params, normalized_limit]))
                if complete_same_fact_coverage
                else []
            )

            families: list[dict[str, Any]] = []
            for row in family_rows:
                family_key = "|".join(
                    (
                        normalized_scope_mode,
                        _trim(row.get("holder_name")),
                        _trim(row.get("id_no")),
                        _trim(row.get("same_fact_key")),
                    )
                )
                families.append(
                    {
                        "family_id": f"same_fact_{hashlib.sha256(family_key.encode('utf-8')).hexdigest()[:16]}",
                        "holder_name": _trim(row.get("holder_name")),
                        "id_no": _trim(row.get("id_no")),
                        "holder_identity_count": _optional_nonnegative_int(row.get("holder_identity_count")),
                        "txn_time": _trim(row.get("txn_time_key")),
                        "amount": _trim(row.get("amount_key")),
                        "balance": _trim(row.get("balance_key")),
                        "direction": _trim(row.get("direction_key")),
                        "counterparty_account": _trim(row.get("counterparty_account_key")),
                        "counterparty_name": _trim(row.get("counterparty_name_key")),
                        "summary": _trim(row.get("summary_key")),
                        "remark": _trim(row.get("remark_key")),
                        "txn_type": _trim(row.get("txn_type_key")),
                        "row_count": _optional_nonnegative_int(row.get("row_count")),
                        "account_count": _optional_nonnegative_int(row.get("account_count")),
                        "txn_id_count": _optional_nonnegative_int(row.get("txn_id_count")),
                        "source_file_count": _optional_nonnegative_int(row.get("source_file_count")),
                        "gross_abs_amount": _optional_rounded_float(row.get("gross_abs_amount"), 2),
                        "canonical_abs_amount": _optional_rounded_float(row.get("canonical_abs_amount"), 2),
                        "candidate_duplicate_amount": _optional_rounded_float(row.get("candidate_duplicate_amount"), 2),
                        "account_keys": _trim_list(row.get("account_keys") or []),
                        "txn_ids": _trim_list(row.get("txn_ids") or []),
                        "source_file_ids": _trim_list(row.get("source_file_ids") or []),
                    }
                )

            warnings = list(scope.get("warnings") or [])
            same_holder_extra_rows = (
                _optional_nonnegative_int(summary.get("same_holder_extra_rows"))
                if complete_same_fact_coverage
                else None
            )
            full_case_cross_holder_groups = (
                _optional_nonnegative_int(summary.get("full_case_cross_holder_group_count"))
                if complete_same_fact_coverage
                else None
            )
            selected_scope_extra_rows = (
                _optional_nonnegative_int(summary.get("selected_scope_extra_rows"))
                if complete_same_fact_coverage
                else None
            )
            selected_scope_cross_holder_groups = (
                _optional_nonnegative_int(summary.get("selected_scope_cross_holder_group_count"))
                if complete_same_fact_coverage
                else None
            )
            if same_holder_extra_rows is not None and same_holder_extra_rows > 0:
                warnings.append(
                    {
                        "code": "SAME_HOLDER_SAME_FACT_DUPLICATE_FAMILIES",
                        "message": f"同一户名/同一人账户集合内发现 {same_holder_extra_rows} 行同事实重复候选；换卡/补卡确认前不得直接扣减金额。",
                        "severity": "warning",
                    }
                )
            if full_case_cross_holder_groups is not None and full_case_cross_holder_groups > 0:
                warnings.append(
                    {
                        "code": "FULL_CASE_BLIND_DEDUPE_NOT_ALLOWED",
                        "message": f"全案同事实扫描存在 {full_case_cross_holder_groups} 个跨户名候选族；不能按全案字段相同直接去重，必须至少约束到同一人账户集合。",
                        "severity": "warning",
                    }
                )
            if selected_scope_cross_holder_groups is not None and selected_scope_cross_holder_groups > 0:
                warnings.append(
                    {
                        "code": "SELECTED_SCOPE_CROSS_HOLDER_REVIEW",
                        "message": f"显式账户范围内有 {selected_scope_cross_holder_groups} 个跨户名同事实候选族；这仅证明所选账户集合内存在候选，不证明主体同一。",
                        "severity": "warning",
                    }
                )
            if not bool(scope.get("scope_requested")):
                warnings.append(
                    {
                        "code": "DUPLICATE_SCAN_IS_CASE_REVIEW_NOT_DEDUCTION",
                        "message": "未指定户名或账户时，本工具只做全案风险地图；报告级扣减必须回到同一户名/同一证件/显式账户集合逐族确认。",
                        "severity": "info",
                    }
                )
            if not complete_same_fact_coverage:
                warnings.append(
                    {
                        "code": "SAME_FACT_ELIGIBILITY_INCOMPLETE",
                        "message": "同事实资格校验未完整覆盖当前范围；重复族、候选差额和无重复结论均保持未解析。",
                        "severity": "warning",
                    }
                )

            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="resolve_duplicate_families",
                params={
                    "case_id": case_id,
                    "scope_mode": normalized_scope_mode,
                    "scope": {
                        "scope_type": str(scope.get("scope_type") or "case"),
                        "selected_account_count": len(selected_accounts),
                        "selected_accounts_preview": selected_accounts_preview,
                        "selected_accounts_truncated": selected_accounts_truncated,
                        "holder_name": _trim(holder_name),
                        "id_no_provided": bool(_trim(id_no)),
                    },
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "direction_mode": _trim(direction_mode) or "both",
                    "min_amount": min_amount_value,
                    "limit": normalized_limit,
                },
                summary={
                    "same_fact_eligibility": same_fact_eligibility,
                    "same_holder_extra_rows": same_holder_extra_rows,
                    "selected_scope_extra_rows": selected_scope_extra_rows,
                    "full_case_extra_rows": (
                        _optional_nonnegative_int(summary.get("full_case_extra_rows"))
                        if complete_same_fact_coverage
                        else None
                    ),
                    "full_case_cross_holder_group_count": full_case_cross_holder_groups,
                    "returned_families": len(families),
                },
                row_count=same_fact_eligibility["requested_row_count"],
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "coverage_status": same_fact_eligibility["coverage_status"],
                "blocker": same_fact_eligibility["blocker"],
                "same_fact_eligibility": same_fact_eligibility,
                "scope": {
                    "scope_type": str(scope.get("scope_type") or "case"),
                    "scope_mode": normalized_scope_mode,
                    "selected_account_count": len(selected_accounts),
                    "selected_accounts_preview": selected_accounts_preview,
                    "selected_accounts_truncated": selected_accounts_truncated,
                    "holder_name": _trim(holder_name),
                    "id_no_provided": bool(_trim(id_no)),
                    "match_mode": _trim(match_mode) or "exact",
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "direction_mode": _trim(direction_mode) or "both",
                    "min_amount": min_amount_value,
                    "limit": normalized_limit,
                },
                "summary": {
                    "scoped_txn_count": (
                        same_fact_eligibility["requested_row_count"] if complete_same_fact_coverage else None
                    ),
                    "scoped_account_count": (
                        _optional_nonnegative_int(summary.get("eligible_account_count"))
                        if complete_same_fact_coverage
                        else None
                    ),
                    "same_holder_same_fact": {
                        "group_count": (
                            _optional_nonnegative_int(summary.get("same_holder_group_count"))
                            if complete_same_fact_coverage
                            else None
                        ),
                        "extra_rows": same_holder_extra_rows,
                        "candidate_duplicate_amount": (
                            _optional_rounded_float(summary.get("same_holder_candidate_duplicate_amount"), 2)
                            if complete_same_fact_coverage
                            else None
                        ),
                    },
                    "selected_scope_review_only": {
                        "group_count": (
                            _optional_nonnegative_int(summary.get("selected_scope_group_count"))
                            if complete_same_fact_coverage
                            else None
                        ),
                        "extra_rows": selected_scope_extra_rows,
                        "candidate_duplicate_amount": (
                            _optional_rounded_float(summary.get("selected_scope_candidate_duplicate_amount"), 2)
                            if complete_same_fact_coverage
                            else None
                        ),
                        "cross_holder_group_count": selected_scope_cross_holder_groups,
                    },
                    "full_case_review_only": {
                        "group_count": (
                            _optional_nonnegative_int(summary.get("full_case_group_count"))
                            if complete_same_fact_coverage
                            else None
                        ),
                        "extra_rows": (
                            _optional_nonnegative_int(summary.get("full_case_extra_rows"))
                            if complete_same_fact_coverage
                            else None
                        ),
                        "candidate_duplicate_amount": (
                            _optional_rounded_float(summary.get("full_case_candidate_duplicate_amount"), 2)
                            if complete_same_fact_coverage
                            else None
                        ),
                        "cross_holder_group_count": full_case_cross_holder_groups,
                    },
                },
                "families": families,
                "dedupe_policy": {
                    "minimum_safe_scope": "same_holder_or_same_id_account_set",
                    "default_scope": "same_holder_accounts",
                    "full_case_blind_dedupe_allowed": False,
                    "deduct_from_totals_allowed": False,
                    "confirmation_required": [
                        "同一户名/证件或明确同一控制人账户集合",
                        "银行换卡/补卡/同账号映射或回单佐证",
                        "报告中明确采用规范明细口径还是候选去重口径",
                    ],
                },
                "amount_semantics": {
                    "gross_abs_amount": "候选族内所有行绝对金额合计，不能直接作为新增资金。",
                    "canonical_abs_amount": "候选族内最大单笔绝对金额，仅用于估算同事实代表金额。",
                    "candidate_duplicate_amount": "gross_abs_amount - canonical_abs_amount；仅为候选差额，未经确认不得扣减。",
                },
                "warnings": warnings,
                "query_id": query_id,
            }
        finally:
            engine.close()

    def build_scope_coverage(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            try:
                self._daily_agg.ensure_materialized(case_id, engine=engine)
            except Exception:
                pass
            selected_accounts = _trim_list(account_keys or [])
            where_sql, params = self._materialized_filtered_detail_scope(
                engine,
                case_id=case_id,
                account_keys=selected_accounts,
                file_ids=source_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            detail_index_available = self._table_exists(engine, "analysis_txn_detail_idx")
            total_rows = engine.query("SELECT COUNT(1) FROM analysis_txn_detail_idx") if detail_index_available else []
            txn_total = _optional_nonnegative_int(total_rows[0][0]) if total_rows and total_rows[0] else None
            source_file_count: int | None = None
            if detail_index_available and "file_id" in self._table_columns(engine, "analysis_txn_detail_idx"):
                rows = engine.query(f"SELECT COUNT(DISTINCT file_id) FROM analysis_txn_detail_idx WHERE {where_sql}", tuple(params))
                source_file_count = _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None
            txn_analyzed = _optional_nonnegative_int(scope_stats.get("txn_count"))
            coverage = {
                "case_id": case_id,
                "scope": (
                    "account_group"
                    if len(selected_accounts) > 1
                    else "account"
                    if selected_accounts
                    else "source_files"
                    if _trim_list(source_file_ids or [])
                    else "case"
                ),
                "selected_accounts": selected_accounts,
                "account_count": len(selected_accounts) if selected_accounts else _optional_nonnegative_int(scope_stats.get("account_count")),
                "txn_total": txn_total,
                "txn_analyzed": txn_analyzed,
                "directed_transaction_rows": _optional_nonnegative_int(scope_stats.get("directed_txn_count")),
                "undirected_transaction_rows": _optional_nonnegative_int(scope_stats.get("undirected_txn_count")),
                "inflow_total": _optional_rounded_float(scope_stats.get("in_amount"), 2),
                "outflow_total": _optional_rounded_float(scope_stats.get("out_amount"), 2),
                "turnover_total": _optional_rounded_float(scope_stats.get("turnover_total"), 2),
                "txn_excluded": (
                    max(0, txn_total - txn_analyzed)
                    if txn_total is not None and txn_analyzed is not None
                    else None
                ),
                "source_file_count": source_file_count,
                "date_min": _trim(scope_stats.get("first_txn_at")),
                "date_max": _trim(scope_stats.get("last_txn_at")),
                "field_quality": scope_stats.get("field_coverage") or {},
                "coverage_status": _trim(scope_stats.get("coverage_status")) or "unresolved",
            }
            warnings: list[dict[str, Any]] = []
            if selected_accounts and txn_analyzed == 0:
                warnings.append(
                    {
                        "code": "SCOPE_COVERAGE_EMPTY",
                        "message": "当前账户范围未命中交易，不能生成账户事实结论。",
                        "severity": "warning",
                    }
                )
            query_id = ""
            if coverage["coverage_status"] == "complete":
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="scope_coverage",
                    params={
                        "case_id": case_id,
                        "account_keys": selected_accounts,
                        "source_file_ids": _trim_list(source_file_ids or []),
                        "date_start": _trim(date_start),
                        "date_end": _trim(date_end),
                        "direction_mode": _trim(direction_mode) or "both",
                        "success_filter": _trim(success_filter) or "all",
                        "cash_filter": _trim(cash_filter) or "all",
                    },
                    summary=coverage,
                    row_count=txn_analyzed,
                    duration_ms=int((time.perf_counter() - started) * 1000),
                )
            return {"coverage": coverage, "scope_stats": scope_stats, "warnings": warnings, "query_id": query_id}
        finally:
            engine.close()

    def resolve_account_scope(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            try:
                self._daily_agg.ensure_materialized(case_id, engine=engine)
            except Exception:
                pass
            requested = _unique_texts([*(account_keys or []), account_key, card_no, acct_no])
            if not requested:
                raise ValueError("resolve_account_scope requires account_key, card_no, acct_no, or account_keys")
            candidates: set[str] = set()
            if self._table_exists(engine, "analysis_txn_detail_idx"):
                detail_columns = self._table_columns(engine, "analysis_txn_detail_idx")
                match_columns = [col for col in ("acct_key", "card_no", "acct_no") if col in detail_columns]
                if match_columns:
                    where_parts = []
                    params: list[Any] = []
                    for column in match_columns:
                        where_parts.append(f"{column} IN ({','.join(['?'] * len(requested))})")
                        params.extend(requested)
                    if "acct_key" in detail_columns:
                        where_parts.append(
                            "REGEXP_REPLACE(acct_key, '(CNY|RMB|USD|HKD|EUR|JPY|GBP|AUD|CAD|SGD)0?$', '') "
                            f"IN ({','.join(['?'] * len(requested))})"
                        )
                        params.extend(requested)
                    rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT DISTINCT acct_key
                        FROM analysis_txn_detail_idx
                        WHERE ({' OR '.join(where_parts)})
                          AND acct_key IS NOT NULL
                          AND TRIM(acct_key) <> ''
                        ORDER BY acct_key ASC
                        LIMIT 256
                        """,
                        tuple(params),
                    )
                    candidates.update(_trim(row.get("acct_key")) for row in rows if _trim(row.get("acct_key")))
            if self._table_exists(engine, "fc_transaction_norm"):
                norm_columns = self._table_columns(engine, "fc_transaction_norm")
                key_expr = _coalesce_text_expr(
                    norm_columns,
                    ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
                )
                match_columns = [col for col in ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no") if col in norm_columns]
                if key_expr and match_columns:
                    where_parts = []
                    params = []
                    for column in match_columns:
                        where_parts.append(f"{column} IN ({','.join(['?'] * len(requested))})")
                        params.extend(requested)
                    rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT DISTINCT {key_expr} AS account_key
                        FROM fc_transaction_norm
                        WHERE ({' OR '.join(where_parts)})
                        ORDER BY account_key ASC
                        LIMIT 256
                        """,
                        tuple(params),
                    )
                    candidates.update(_trim(row.get("account_key")) for row in rows if _trim(row.get("account_key")))
            resolved_accounts = sorted(candidates)
            coverage = self._build_scope_coverage_with_engine(
                engine,
                case_id=case_id,
                account_keys=resolved_accounts,
                started=started,
            )
            warnings: list[dict[str, Any]] = []
            if not resolved_accounts:
                warnings.append({"code": "ACCOUNT_SCOPE_NOT_FOUND", "message": "输入账户/卡号未在当前案件交易索引中命中。", "severity": "warning"})
            if len(resolved_accounts) > 1:
                warnings.append({"code": "ACCOUNT_SCOPE_MULTIPLE_MATCHES", "message": "输入账户/卡号命中多个账户键，后续分析将按账户集合处理。", "severity": "warning"})
            scope_id = _stable_slug("acct_scope", case_id, _json_dumps(resolved_accounts), _json_dumps(requested))
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="resolve_account_scope",
                params={"case_id": case_id, "requested_count": len(requested)},
                summary={"scope_id": scope_id, "account_count": len(resolved_accounts), "txn_analyzed": coverage["coverage"]["txn_analyzed"]},
                row_count=coverage["coverage"]["txn_analyzed"],
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "scope_id": scope_id,
                "scope_type": "account_group" if len(resolved_accounts) > 1 else "account",
                "requested": requested,
                "account_keys": resolved_accounts,
                "match_confidence": "exact" if resolved_accounts else "none",
                "coverage": coverage["coverage"],
                "warnings": [*warnings, *list(coverage.get("warnings") or [])],
                "query_id": query_id,
            }
        finally:
            engine.close()

    def resolve_holder_scope(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            try:
                self._daily_agg.ensure_materialized(case_id, engine=engine)
            except Exception:
                pass
            normalized_name = _trim(holder_name)
            normalized_id_no = _trim(id_no)
            if not normalized_name and not normalized_id_no:
                raise ValueError("resolve_holder_scope requires holder_name or id_no")
            account_keys: set[str] = set()
            holder_labels: set[str] = set()
            holder_id_numbers: set[str] = set()
            warnings: list[dict[str, Any]] = []
            if self._table_exists(engine, "analysis_account_dim"):
                where_parts: list[str] = []
                params: list[Any] = []
                if normalized_name:
                    if _trim(match_mode).lower() == "contains":
                        where_parts.append("open_name LIKE ?")
                        params.append(f"%{normalized_name}%")
                    else:
                        where_parts.append("open_name=?")
                        params.append(normalized_name)
                if normalized_id_no:
                    where_parts.append("id_no=?")
                    params.append(normalized_id_no)
                if where_parts:
                    joiner = " AND " if normalized_name and normalized_id_no else " OR "
                    rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT
                          account_key,
                          COALESCE(NULLIF(TRIM(open_name), ''), '') AS holder_name,
                          COALESCE(NULLIF(TRIM(id_no), ''), '') AS id_no
                        FROM analysis_account_dim
                        WHERE {joiner.join(f'({part})' for part in where_parts)}
                          AND account_key IS NOT NULL
                          AND TRIM(account_key) <> ''
                        ORDER BY holder_name ASC, account_key ASC
                        LIMIT 5000
                        """,
                        tuple(params),
                    )
                    account_keys.update(_trim(row.get("account_key")) for row in rows if _trim(row.get("account_key")))
                    holder_labels.update(_trim(row.get("holder_name")) for row in rows if _trim(row.get("holder_name")))
                    holder_id_numbers.update(_trim(row.get("id_no")) for row in rows if _trim(row.get("id_no")))
            if not account_keys and self._table_exists(engine, "analysis_txn_detail_idx"):
                columns = self._table_columns(engine, "analysis_txn_detail_idx")
                where_parts: list[str] = []
                params: list[Any] = []
                if normalized_name and "account_open_name" in columns:
                    if _trim(match_mode).lower() == "contains":
                        where_parts.append("account_open_name LIKE ?")
                        params.append(f"%{normalized_name}%")
                    else:
                        where_parts.append("account_open_name=?")
                        params.append(normalized_name)
                if normalized_id_no:
                    id_columns = [col for col in ("opener_id_no", "account_id_no", "id_no") if col in columns]
                    for column in id_columns:
                        where_parts.append(f"{column}=?")
                        params.append(normalized_id_no)
                    if not id_columns:
                        warnings.append({"code": "HOLDER_ID_COLUMN_UNAVAILABLE", "message": "当前交易明细索引未提供开户证件字段，证件号仅可在可用标准表中辅助匹配。", "severity": "warning"})
                if where_parts:
                    rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT acct_key, MAX(COALESCE(account_open_name, '')) AS holder_name, COUNT(1) AS txn_count
                        FROM analysis_txn_detail_idx
                        WHERE {' OR '.join(f'({part})' for part in where_parts)}
                          AND acct_key IS NOT NULL
                          AND TRIM(acct_key) <> ''
                        GROUP BY acct_key
                        ORDER BY txn_count DESC, acct_key ASC
                        LIMIT 5000
                        """,
                        tuple(params),
                    )
                    account_keys.update(_trim(row.get("acct_key")) for row in rows if _trim(row.get("acct_key")))
                    holder_labels.update(_trim(row.get("holder_name")) for row in rows if _trim(row.get("holder_name")))
            if not account_keys and normalized_id_no and self._table_exists(engine, "fc_transaction_norm"):
                columns = self._table_columns(engine, "fc_transaction_norm")
                key_expr = _coalesce_text_expr(columns, ("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"))
                if key_expr and "opener_id_no" in columns:
                    rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT DISTINCT {key_expr} AS account_key, COALESCE(account_open_name, '') AS holder_name
                        FROM fc_transaction_norm
                        WHERE opener_id_no=?
                        ORDER BY account_key ASC
                        LIMIT 5000
                        """,
                        (normalized_id_no,),
                    )
                    account_keys.update(_trim(row.get("account_key")) for row in rows if _trim(row.get("account_key")))
                    holder_labels.update(_trim(row.get("holder_name")) for row in rows if _trim(row.get("holder_name")))
            resolved_accounts = sorted(account_keys)
            coverage = self._build_scope_coverage_with_engine(
                engine,
                case_id=case_id,
                account_keys=resolved_accounts,
                started=started,
            )
            if not resolved_accounts:
                warnings.append({"code": "HOLDER_SCOPE_NOT_FOUND", "message": "输入户名/证件号未在当前案件交易索引中命中账户。", "severity": "warning"})
            if len(holder_labels) > 1 and not normalized_id_no:
                warnings.append({"code": "HOLDER_NAME_AMBIGUOUS", "message": "当前户名匹配到多个展示标签，未提供证件号时应按候选集合复核。", "severity": "warning"})
            scope_id = _stable_slug("holder_scope", case_id, normalized_name, normalized_id_no, _json_dumps(resolved_accounts))
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="resolve_holder_scope",
                params={"case_id": case_id, "has_holder_name": bool(normalized_name), "has_id_no": bool(normalized_id_no), "match_mode": _trim(match_mode) or "exact"},
                summary={"scope_id": scope_id, "account_count": len(resolved_accounts), "txn_analyzed": coverage["coverage"]["txn_analyzed"]},
                row_count=coverage["coverage"]["txn_analyzed"],
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "scope_id": scope_id,
                "scope_type": "holder_account_group",
                "holder_name": normalized_name,
                "id_no_provided": bool(normalized_id_no),
                "account_keys": resolved_accounts,
                "holder_labels": sorted(holder_labels),
                "holder_id_numbers": sorted(holder_id_numbers),
                "match_confidence": "exact" if resolved_accounts and (normalized_id_no or _trim(match_mode).lower() == "exact") else "candidate" if resolved_accounts else "none",
                "coverage": coverage["coverage"],
                "warnings": [*warnings, *list(coverage.get("warnings") or [])],
                "query_id": query_id,
            }
        finally:
            engine.close()

    def _build_scope_coverage_with_engine(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        account_keys: Sequence[str],
        started: float,
    ) -> dict[str, Any]:
        if not self._table_exists(engine, "analysis_txn_detail_idx"):
            return {
                "coverage": {
                    "case_id": case_id,
                    "scope": "account_group" if len(account_keys) > 1 else "account" if account_keys else "case",
                    "selected_accounts": list(account_keys),
                    "account_count": len(account_keys),
                    "txn_total": None,
                    "txn_analyzed": None,
                    "directed_transaction_rows": None,
                    "undirected_transaction_rows": None,
                    "inflow_total": None,
                    "outflow_total": None,
                    "turnover_total": None,
                    "txn_excluded": None,
                    "source_file_count": None,
                    "date_min": "",
                    "date_max": "",
                    "field_quality": {},
                    "coverage_status": "unresolved",
                },
                "warnings": [{"code": "DETAIL_INDEX_UNAVAILABLE", "message": "analysis_txn_detail_idx 不可用。", "severity": "error"}],
                "query_id": "",
            }
        where_sql, params = self._materialized_filtered_detail_scope(
            engine,
            case_id=case_id,
            account_keys=list(account_keys),
        )
        scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
        total_rows = engine.query("SELECT COUNT(1) FROM analysis_txn_detail_idx")
        txn_total = _optional_nonnegative_int(total_rows[0][0]) if total_rows and total_rows[0] else None
        source_file_count: int | None = None
        if "file_id" in self._table_columns(engine, "analysis_txn_detail_idx"):
            rows = engine.query(f"SELECT COUNT(DISTINCT file_id) FROM analysis_txn_detail_idx WHERE {where_sql}", tuple(params))
            source_file_count = _optional_nonnegative_int(rows[0][0]) if rows and rows[0] else None
        txn_analyzed = _optional_nonnegative_int(scope_stats.get("txn_count"))
        coverage = {
            "case_id": case_id,
            "scope": "account_group" if len(account_keys) > 1 else "account" if account_keys else "case",
            "selected_accounts": list(account_keys),
            "account_count": len(account_keys) if account_keys else _optional_nonnegative_int(scope_stats.get("account_count")),
            "txn_total": txn_total,
            "txn_analyzed": txn_analyzed,
            "directed_transaction_rows": _optional_nonnegative_int(scope_stats.get("directed_txn_count")),
            "undirected_transaction_rows": _optional_nonnegative_int(scope_stats.get("undirected_txn_count")),
            "inflow_total": _optional_rounded_float(scope_stats.get("in_amount"), 2),
            "outflow_total": _optional_rounded_float(scope_stats.get("out_amount"), 2),
            "turnover_total": _optional_rounded_float(scope_stats.get("turnover_total"), 2),
            "txn_excluded": (
                max(0, txn_total - txn_analyzed)
                if txn_total is not None and txn_analyzed is not None
                else None
            ),
            "source_file_count": source_file_count,
            "date_min": _trim(scope_stats.get("first_txn_at")),
            "date_max": _trim(scope_stats.get("last_txn_at")),
            "field_quality": scope_stats.get("field_coverage") or {},
            "coverage_status": _trim(scope_stats.get("coverage_status")) or "unresolved",
        }
        return {"coverage": coverage, "scope_stats": scope_stats, "warnings": [], "query_id": "", "duration_ms": int((time.perf_counter() - started) * 1000)}

    def _field_presence_rates(
        self,
        engine: DuckDBEngine,
        *,
        table_name: str,
        columns: Sequence[str],
        available_columns: set[str],
    ) -> dict[str, Any]:
        selected_columns = [column for column in columns if column in available_columns]
        if not selected_columns:
            return {}
        expressions = [
            f"SUM(CASE WHEN {column} IS NOT NULL AND TRIM(CAST({column} AS VARCHAR)) <> '' THEN 1 ELSE 0 END) AS {column}_present"
            for column in selected_columns
        ]
        rows = self._query_dicts(engine, f"SELECT COUNT(1) AS total, {', '.join(expressions)} FROM {table_name}")
        row = rows[0] if rows else {}
        total = _optional_nonnegative_int(row.get("total"))
        result: dict[str, Any] = {"total": total}
        for column in selected_columns:
            present = _optional_nonnegative_int(row.get(f"{column}_present"))
            result[column] = {
                "present": present,
                "rate": (
                    round(present / total, 4)
                    if present is not None and total is not None and total > 0 and present <= total
                    else None
                ),
            }
        return result

    def _resolve_rank_scope_accounts(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
    ) -> dict[str, Any]:
        explicit_accounts = _trim_list([*(account_keys or []), account_key, card_no, acct_no])
        holder_accounts: list[str] = []
        warnings: list[dict[str, Any]] = []
        account_scope: dict[str, Any] = {}
        holder_scope: dict[str, Any] = {}
        account_scope_requested = bool(explicit_accounts)
        holder_scope_requested = bool(_trim(holder_name) or _trim(id_no))

        if _trim(account_key) or _trim(card_no) or _trim(acct_no):
            try:
                account_scope = self.resolve_account_scope(
                    case_id,
                    account_keys=account_keys,
                    account_key=_trim(account_key),
                    card_no=_trim(card_no),
                    acct_no=_trim(acct_no),
                )
                explicit_accounts = _trim_list((account_scope.get("account_keys") or []) or explicit_accounts)
                warnings.extend(list(account_scope.get("warnings") or []))
            except Exception as exc:
                explicit_accounts = _trim_list([*(account_keys or []), account_key, card_no, acct_no])
                warnings.append(
                    {
                        "code": "RANK_ACCOUNT_SCOPE_RESOLVE_FAILED",
                        "message": f"账户 scope 解析失败，已按输入账户键尝试精确过滤：{exc}",
                        "severity": "warning",
                    }
                )
        if holder_scope_requested:
            try:
                holder_scope = self.resolve_holder_scope(
                    case_id,
                    holder_name=_trim(holder_name),
                    id_no=_trim(id_no),
                    match_mode=_trim(match_mode) or "exact",
                )
                holder_accounts = _trim_list(holder_scope.get("account_keys") or [])
                warnings.extend(list(holder_scope.get("warnings") or []))
            except Exception as exc:
                warnings.append(
                    {
                        "code": "RANK_HOLDER_SCOPE_RESOLVE_FAILED",
                        "message": f"户名 scope 解析失败：{exc}",
                        "severity": "warning",
                    }
                )

        if holder_scope_requested and account_scope_requested:
            holder_set = set(holder_accounts)
            selected_accounts = [item for item in explicit_accounts if item in holder_set]
            scope_type = "account_holder_intersection"
            if explicit_accounts and holder_accounts and not selected_accounts:
                warnings.append(
                    {
                        "code": "RANK_SCOPE_INTERSECTION_EMPTY",
                        "message": "账户 scope 与户名 scope 没有交集，本次排名返回空结果而非回退全案。",
                        "severity": "warning",
                    }
                )
        elif holder_scope_requested:
            selected_accounts = holder_accounts
            scope_type = "holder"
        elif account_scope_requested:
            selected_accounts = explicit_accounts
            scope_type = "account_group" if len(explicit_accounts) > 1 else "account"
        else:
            selected_accounts = []
            scope_type = "case"

        return {
            "scope_type": scope_type,
            "scope_requested": account_scope_requested or holder_scope_requested,
            "selected_accounts": _unique_texts(selected_accounts),
            "account_scope": account_scope,
            "holder_scope": holder_scope,
            "holder_name": _trim(holder_name),
            "id_no_provided": bool(_trim(id_no)),
            "match_mode": _trim(match_mode) or "exact",
            "warnings": warnings,
        }

    def _rank_scope_where(
        self,
        engine: DuckDBEngine,
        *,
        case_id: str,
        selected_accounts: Sequence[str],
        scope_requested: bool,
        source_file_ids: Sequence[str],
        date_start: str,
        date_end: str,
        direction_mode: str,
        success_filter: str,
        cash_filter: str,
    ) -> tuple[str, list[Any]]:
        if scope_requested and not _trim_list(selected_accounts):
            return "1=0", []
        return self._materialized_filtered_detail_scope(
            engine,
            case_id=case_id,
            account_keys=_trim_list(selected_accounts) or None,
            file_ids=_trim_list(source_file_ids),
            date_start=_trim(date_start),
            date_end=_trim(date_end),
            direction_mode=_trim(direction_mode) or "both",
            success_filter=_trim(success_filter) or "all",
            cash_filter=_trim(cash_filter) or "all",
        )

    def _same_fact_dedupe_preflight(
        self,
        engine: DuckDBEngine,
        *,
        where_sql: str,
        params: Sequence[Any],
    ) -> dict[str, int] | None:
        eligibility_sql = _same_fact_txn_eligibility_sql()
        try:
            rows = engine.query(
                f"""
                SELECT
                  COUNT(1) AS requested_txn_count,
                  COUNT(1) FILTER (WHERE {eligibility_sql}) AS eligible_txn_count,
                  COUNT(DISTINCT CASE
                    WHEN {_valid_txn_id_sql("id")} THEN CAST(id AS VARCHAR)
                    ELSE NULL
                  END) AS distinct_row_id_count
                FROM analysis_txn_detail_idx
                WHERE {where_sql}
                """,
                tuple(params),
            )
        except Exception:
            return None
        row = _exact_single_query_row(rows, width=3)
        if row is None:
            return None
        requested = _exact_nonnegative_db_int(row[0])
        eligible = _exact_nonnegative_db_int(row[1])
        distinct_row_ids = _exact_nonnegative_db_int(row[2])
        if (
            requested is None
            or eligible is None
            or distinct_row_ids is None
            or eligible > requested
            or distinct_row_ids > requested
        ):
            return None
        return {
            "requested_txn_count": requested,
            "eligible_txn_count": eligible,
            "ineligible_txn_count": requested - eligible,
            "distinct_row_id_count": distinct_row_ids,
        }

    def rank_accounts(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        scope = self._resolve_rank_scope_accounts(
            case_id,
            account_keys=account_keys,
            account_key=account_key,
            card_no=card_no,
            acct_no=acct_no,
            holder_name=holder_name,
            id_no=id_no,
            match_mode=match_mode,
        )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            lifecycle_warnings = self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            normalized_limit = max(1, min(int(limit or 20), 200))
            normalized_metric = _normalize_rank_metric(metric)
            normalized_file_ids = _trim_list(source_file_ids or [])
            selected_accounts = _trim_list(scope.get("selected_accounts") or [])
            where_sql, params = self._rank_scope_where(
                engine,
                case_id=case_id,
                selected_accounts=selected_accounts,
                scope_requested=bool(scope.get("scope_requested")),
                source_file_ids=normalized_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            rows = self._query_dicts(
                engine,
                f"""
                SELECT
                  t.acct_key AS account_key,
                  MAX(COALESCE(NULLIF(TRIM(d.open_name), ''), NULLIF(TRIM(t.account_open_name), ''), '')) AS account_open_name,
                  MAX(COALESCE(NULLIF(TRIM(d.id_no), ''), NULLIF(TRIM(t.opener_id_no), ''), '')) AS holder_id_no,
                  COUNT(1) AS txn_count,
                  SUM(CASE WHEN t.dc_val='进' THEN 1 ELSE 0 END) AS in_txn_count,
                  SUM(CASE WHEN t.dc_val='出' THEN 1 ELSE 0 END) AS out_txn_count,
                  SUM(CASE WHEN t.dc_val='进' AND t.amount IS NOT NULL THEN ABS(t.amount) END) AS inflow_total,
                  SUM(CASE WHEN t.dc_val='出' AND t.amount IS NOT NULL THEN ABS(t.amount) END) AS outflow_total,
                  MAX(CASE WHEN t.amount IS NOT NULL THEN ABS(t.amount) END) AS max_single_amount,
                  SUM(CASE WHEN t.dc_val='进' AND t.amount IS NOT NULL THEN 1 ELSE 0 END) AS in_amount_present_count,
                  SUM(CASE WHEN t.dc_val='出' AND t.amount IS NOT NULL THEN 1 ELSE 0 END) AS out_amount_present_count,
                  SUM(CASE WHEN t.amount IS NOT NULL THEN 1 ELSE 0 END) AS amount_present_count,
                  COUNT(DISTINCT COALESCE(NULLIF(t.cp_key, ''), NULLIF(t.cp_name, ''), NULLIF(t.cp_raw, ''))) AS counterparty_count,
                  MIN(CAST(t.txn_ts AS VARCHAR)) AS first_txn_at,
                  MAX(CAST(t.txn_ts AS VARCHAR)) AS last_txn_at
                FROM analysis_txn_detail_idx t
                LEFT JOIN analysis_account_dim d ON d.account_key = t.acct_key
                WHERE {where_sql}
                  AND t.acct_key IS NOT NULL
                  AND TRIM(t.acct_key) <> ''
                GROUP BY t.acct_key
                """,
                tuple(params),
            )
            normalized_rows: list[dict[str, Any]] = []
            for row in rows:
                amount_facts = _complete_rank_amount_facts(row)
                normalized_rows.append(
                    {
                        "account_key": _trim(row.get("account_key")),
                        "account_open_name": _trim(row.get("account_open_name")),
                        "holder_id_no": _trim(row.get("holder_id_no")),
                        "id_no": _trim(row.get("holder_id_no")),
                        **amount_facts,
                        "counterparty_count": _optional_nonnegative_int(row.get("counterparty_count")),
                        "first_txn_at": _trim(row.get("first_txn_at")),
                        "last_txn_at": _trim(row.get("last_txn_at")),
                    }
                )
            eligible_rows = _sort_rank_rows(normalized_rows, metric=normalized_metric, tie_field="account_key")
            ranked_rows = eligible_rows[:normalized_limit]
            coverage_status = _rank_result_coverage_status(
                scope_stats,
                candidate_count=len(normalized_rows),
                eligible_count=len(eligible_rows),
            )
            query_id = ""
            if coverage_status == "complete":
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="rank_accounts",
                    params={
                        "case_id": case_id,
                        "scope": {
                            "scope_type": str(scope.get("scope_type") or "case"),
                            "selected_accounts": selected_accounts,
                            "holder_name": _trim(holder_name),
                            "id_no_provided": bool(_trim(id_no)),
                        },
                        "source_file_ids": normalized_file_ids,
                        "date_start": _trim(date_start),
                        "date_end": _trim(date_end),
                        "direction_mode": _trim(direction_mode) or "both",
                        "success_filter": _trim(success_filter) or "all",
                        "cash_filter": _trim(cash_filter) or "all",
                        "metric": normalized_metric,
                        "limit": normalized_limit,
                    },
                    summary={"top": ranked_rows[:3], "scope_stats": scope_stats},
                    row_count=_optional_nonnegative_int(scope_stats.get("txn_count")),
                    duration_ms=int((time.perf_counter() - started) * 1000),
                )
            group_amount_facts = _rank_scope_amount_facts(scope_stats)
            warnings = _merge_rank_scope_warnings(scope, lifecycle_warnings)
            if coverage_status != "complete":
                warnings.append(
                    {
                        "code": "RANKING_FACT_COVERAGE_INCOMPLETE",
                        "message": "排名所需事实覆盖不完整；未知值未按 0 处理，未形成全范围排名结论。",
                        "severity": "warning",
                    }
                )
            if str(scope.get("scope_type") or "") == "case":
                warnings.append(
                    {
                        "code": "RANKING_FULL_CASE_SCAN",
                        "message": "本次账户排名按全案明细索引聚合，不使用分页样本或模型侧汇总。",
                        "severity": "info",
                    }
                )
            return {
                "scope": {
                    "case_id": case_id,
                    "scope_type": str(scope.get("scope_type") or "case"),
                    "selected_accounts": selected_accounts,
                    "holder_name": _trim(holder_name),
                    "id_no_provided": bool(_trim(id_no)),
                    "match_mode": _trim(match_mode) or "exact",
                    "source_file_ids": normalized_file_ids,
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "direction_mode": _trim(direction_mode) or "both",
                    "success_filter": _trim(success_filter) or "all",
                    "cash_filter": _trim(cash_filter) or "all",
                    "metric": normalized_metric,
                    "limit": normalized_limit,
                },
                "group_summary": {
                    "txn_count": _optional_nonnegative_int(scope_stats.get("txn_count")),
                    "account_count": _optional_nonnegative_int(scope_stats.get("account_count")),
                    "counterparty_count": _optional_nonnegative_int(scope_stats.get("counterparty_count")),
                    **group_amount_facts,
                    "first_txn_at": _trim(scope_stats.get("first_txn_at")),
                    "last_txn_at": _trim(scope_stats.get("last_txn_at")),
                },
                "rankings": ranked_rows,
                "query_id": query_id,
                "warnings": warnings,
                "coverage_status": coverage_status,
                "fact_answer_allowed": False,
                "summary_text": (
                    f"已按 {normalized_metric} 对 {len(normalized_rows)} 个账户完成排名，返回 Top{len(ranked_rows)}。"
                    if coverage_status == "complete"
                    else "排名事实覆盖不完整；仅保留可核验字段，不发布全范围排名结论。"
                ),
            }
        finally:
            engine.close()

    def rank_holders(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        scope = self._resolve_rank_scope_accounts(
            case_id,
            account_keys=account_keys,
            account_key=account_key,
            card_no=card_no,
            acct_no=acct_no,
            holder_name=holder_name,
            id_no=id_no,
            match_mode=match_mode,
        )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            lifecycle_warnings = self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            normalized_limit = max(1, min(int(limit or 20), 200))
            normalized_metric = _normalize_rank_metric(metric)
            normalized_file_ids = _trim_list(source_file_ids or [])
            selected_accounts = _trim_list(scope.get("selected_accounts") or [])
            where_sql, params = self._rank_scope_where(
                engine,
                case_id=case_id,
                selected_accounts=selected_accounts,
                scope_requested=bool(scope.get("scope_requested")),
                source_file_ids=normalized_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            rows = self._query_dicts(
                engine,
                f"""
                SELECT
                  CASE
                    WHEN COALESCE(NULLIF(TRIM(d.open_name), ''), '') <> '' THEN TRIM(d.open_name)
                    ELSE '未登记户名'
                  END AS holder_name,
                  COALESCE(NULLIF(TRIM(d.id_no), ''), '') AS holder_id_no,
                  CASE
                    WHEN COALESCE(NULLIF(TRIM(d.open_name), ''), '') <> ''
                    THEN TRIM(d.open_name) || '|' || COALESCE(NULLIF(TRIM(d.id_no), ''), '无证件号')
                    ELSE 'acct:' || COALESCE(t.acct_key, '')
                  END AS holder_group_key,
                  COUNT(DISTINCT t.acct_key) AS account_count,
                  COUNT(1) AS txn_count,
                  SUM(CASE WHEN t.dc_val='进' THEN 1 ELSE 0 END) AS in_txn_count,
                  SUM(CASE WHEN t.dc_val='出' THEN 1 ELSE 0 END) AS out_txn_count,
                  SUM(CASE WHEN t.dc_val='进' AND t.amount IS NOT NULL THEN ABS(t.amount) END) AS inflow_total,
                  SUM(CASE WHEN t.dc_val='出' AND t.amount IS NOT NULL THEN ABS(t.amount) END) AS outflow_total,
                  MAX(CASE WHEN t.amount IS NOT NULL THEN ABS(t.amount) END) AS max_single_amount,
                  SUM(CASE WHEN t.dc_val='进' AND t.amount IS NOT NULL THEN 1 ELSE 0 END) AS in_amount_present_count,
                  SUM(CASE WHEN t.dc_val='出' AND t.amount IS NOT NULL THEN 1 ELSE 0 END) AS out_amount_present_count,
                  SUM(CASE WHEN t.amount IS NOT NULL THEN 1 ELSE 0 END) AS amount_present_count,
                  COUNT(DISTINCT COALESCE(NULLIF(t.cp_key, ''), NULLIF(t.cp_name, ''), NULLIF(t.cp_raw, ''))) AS counterparty_count,
                  STRING_AGG(DISTINCT t.acct_key, '|') AS account_keys_text,
                  MIN(CAST(t.txn_ts AS VARCHAR)) AS first_txn_at,
                  MAX(CAST(t.txn_ts AS VARCHAR)) AS last_txn_at
                FROM analysis_txn_detail_idx t
                LEFT JOIN analysis_account_dim d ON d.account_key = t.acct_key
                WHERE {where_sql}
                  AND t.acct_key IS NOT NULL
                  AND TRIM(t.acct_key) <> ''
                GROUP BY 1, 2, 3
                """,
                tuple(params),
            )
            normalized_rows: list[dict[str, Any]] = []
            for row in rows:
                account_key_values = _trim_list(str(row.get("account_keys_text") or "").split("|"))
                amount_facts = _complete_rank_amount_facts(row)
                normalized_rows.append(
                    {
                        "holder_name": _trim(row.get("holder_name")) or "未登记户名",
                        "holder_id_no": _trim(row.get("holder_id_no")),
                        "holder_group_key": _trim(row.get("holder_group_key")),
                        "account_count": _optional_nonnegative_int(row.get("account_count")),
                        "account_keys": account_key_values[:50],
                        "account_keys_truncated": len(account_key_values) > 50,
                        **amount_facts,
                        "counterparty_count": _optional_nonnegative_int(row.get("counterparty_count")),
                        "first_txn_at": _trim(row.get("first_txn_at")),
                        "last_txn_at": _trim(row.get("last_txn_at")),
                    }
                )
            eligible_rows = _sort_rank_rows(normalized_rows, metric=normalized_metric, tie_field="holder_name")
            ranked_rows = eligible_rows[:normalized_limit]
            coverage_status = _rank_result_coverage_status(
                scope_stats,
                candidate_count=len(normalized_rows),
                eligible_count=len(eligible_rows),
            )
            query_id = ""
            if coverage_status == "complete":
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="rank_holders",
                    params={
                        "case_id": case_id,
                        "scope": {
                            "scope_type": str(scope.get("scope_type") or "case"),
                            "selected_accounts": selected_accounts,
                            "holder_name": _trim(holder_name),
                            "id_no_provided": bool(_trim(id_no)),
                        },
                        "source_file_ids": normalized_file_ids,
                        "date_start": _trim(date_start),
                        "date_end": _trim(date_end),
                        "direction_mode": _trim(direction_mode) or "both",
                        "success_filter": _trim(success_filter) or "all",
                        "cash_filter": _trim(cash_filter) or "all",
                        "metric": normalized_metric,
                        "limit": normalized_limit,
                    },
                    summary={"top": ranked_rows[:3], "scope_stats": scope_stats},
                    row_count=_optional_nonnegative_int(scope_stats.get("txn_count")),
                    duration_ms=int((time.perf_counter() - started) * 1000),
                )
            group_amount_facts = _rank_scope_amount_facts(scope_stats)
            warnings = _merge_rank_scope_warnings(scope, lifecycle_warnings)
            if coverage_status != "complete":
                warnings.append(
                    {
                        "code": "RANKING_FACT_COVERAGE_INCOMPLETE",
                        "message": "排名所需事实覆盖不完整；未知值未按 0 处理，未形成全范围排名结论。",
                        "severity": "warning",
                    }
                )
            return {
                "scope": {
                    "case_id": case_id,
                    "scope_type": str(scope.get("scope_type") or "case"),
                    "selected_accounts": selected_accounts,
                    "holder_name": _trim(holder_name),
                    "id_no_provided": bool(_trim(id_no)),
                    "match_mode": _trim(match_mode) or "exact",
                    "source_file_ids": normalized_file_ids,
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "direction_mode": _trim(direction_mode) or "both",
                    "success_filter": _trim(success_filter) or "all",
                    "cash_filter": _trim(cash_filter) or "all",
                    "metric": normalized_metric,
                    "limit": normalized_limit,
                },
                "group_summary": {
                    "txn_count": _optional_nonnegative_int(scope_stats.get("txn_count")),
                    "holder_count": len(normalized_rows),
                    "account_count": _optional_nonnegative_int(scope_stats.get("account_count")),
                    "counterparty_count": _optional_nonnegative_int(scope_stats.get("counterparty_count")),
                    **group_amount_facts,
                    "first_txn_at": _trim(scope_stats.get("first_txn_at")),
                    "last_txn_at": _trim(scope_stats.get("last_txn_at")),
                },
                "rankings": ranked_rows,
                "query_id": query_id,
                "warnings": warnings,
                "coverage_status": coverage_status,
                "fact_answer_allowed": False,
                "summary_text": (
                    f"已按 {normalized_metric} 对 {len(normalized_rows)} 个户名集合完成排名，返回 Top{len(ranked_rows)}。"
                    if coverage_status == "complete"
                    else "排名事实覆盖不完整；仅保留可核验字段，不发布全范围排名结论。"
                ),
            }
        finally:
            engine.close()

    def rank_counterparties(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        account_key: str = "",
        card_no: str = "",
        acct_no: str = "",
        holder_name: str = "",
        id_no: str = "",
        match_mode: str = "exact",
        source_file_ids: Optional[Sequence[str]] = None,
        date_start: str = "",
        date_end: str = "",
        direction_mode: str = "both",
        success_filter: str = "all",
        cash_filter: str = "all",
        metric: str = "turnover",
        limit: int = 20,
        dedupe_same_holder_same_fact: bool = False,
        counterparty_group_mode: str = "account",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if type(dedupe_same_holder_same_fact) is not bool:
            raise ValueError("dedupe_same_holder_same_fact must be boolean")
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        scope = self._resolve_rank_scope_accounts(
            case_id,
            account_keys=account_keys,
            account_key=account_key,
            card_no=card_no,
            acct_no=acct_no,
            holder_name=holder_name,
            id_no=id_no,
            match_mode=match_mode,
        )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            lifecycle_warnings = self._cleanup_temp_scope_lifecycle(engine, case_id=case_id)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            normalized_limit = max(1, min(int(limit or 20), 200))
            normalized_metric = _normalize_rank_metric(metric)
            normalized_group_mode = _trim(counterparty_group_mode).lower() or "account"
            if normalized_group_mode not in {"account", "name"}:
                normalized_group_mode = "account"
            normalized_file_ids = _trim_list(source_file_ids or [])
            selected_accounts = _trim_list(scope.get("selected_accounts") or [])
            where_sql, params = self._rank_scope_where(
                engine,
                case_id=case_id,
                selected_accounts=selected_accounts,
                scope_requested=bool(scope.get("scope_requested")),
                source_file_ids=normalized_file_ids,
                date_start=date_start,
                date_end=date_end,
                direction_mode=direction_mode,
                success_filter=success_filter,
                cash_filter=cash_filter,
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            dedupe_requested = dedupe_same_holder_same_fact is True
            dedupe_scope_authorized = (
                dedupe_requested
                and bool(scope.get("scope_requested"))
                and bool(_trim(holder_name) or _trim(id_no))
                and bool(selected_accounts)
            )

            def dedupe_boundary(
                *,
                coverage_status: str,
                blocker: str,
                preflight: Mapping[str, Any] | None = None,
            ) -> dict[str, Any]:
                normalized_preflight = preflight if isinstance(preflight, Mapping) else {}
                warnings = _merge_rank_scope_warnings(scope, lifecycle_warnings)
                warnings.append(
                    {
                        "code": "SAME_FACT_DEDUPE_NOT_APPLIED",
                        "message": "同事实去重所需复合键覆盖不完整或范围未授权；未执行排行，也未将未知重复量解释为 0。",
                        "severity": "warning",
                    }
                )
                return {
                    "scope": {
                        "case_id": case_id,
                        "scope_type": str(scope.get("scope_type") or "case"),
                        "selected_accounts": selected_accounts,
                        "holder_name": _trim(holder_name),
                        "id_no_provided": bool(_trim(id_no)),
                        "match_mode": _trim(match_mode) or "exact",
                        "source_file_ids": normalized_file_ids,
                    },
                    "filters": {
                        "date_start": _trim(date_start),
                        "date_end": _trim(date_end),
                        "direction_mode": _trim(direction_mode) or "both",
                        "success_filter": _trim(success_filter) or "all",
                        "cash_filter": _trim(cash_filter) or "all",
                        "metric": normalized_metric,
                        "limit": normalized_limit,
                        "dedupe_same_holder_same_fact": True,
                        "counterparty_group_mode": normalized_group_mode,
                    },
                    "group_summary": {
                        "txn_count": None,
                        "directed_txn_count": None,
                        "undirected_txn_count": None,
                        "counterparty_count": None,
                        "account_count": None,
                        "inflow_total": None,
                        "outflow_total": None,
                        "turnover_total": None,
                        "max_single_amount": None,
                        "first_txn_at": "",
                        "last_txn_at": "",
                        "dedupe_same_holder_same_fact": False,
                        "dedupe_raw_txn_count": None,
                        "dedupe_txn_count": None,
                        "dedupe_candidate_duplicate_amount": None,
                    },
                    "same_fact_dedupe": {
                        "contract": "SameFactDedupeCoverageV1",
                        "key_version": SAME_FACT_KEY_VERSION,
                        "requested": True,
                        "scope_authorized": dedupe_scope_authorized,
                        "applied": False,
                        "coverage_status": coverage_status,
                        "requested_txn_count": _exact_nonnegative_db_int(
                            normalized_preflight.get("requested_txn_count")
                        ),
                        "eligible_txn_count": _exact_nonnegative_db_int(
                            normalized_preflight.get("eligible_txn_count")
                        ),
                        "ineligible_txn_count": _exact_nonnegative_db_int(
                            normalized_preflight.get("ineligible_txn_count")
                        ),
                        "distinct_row_id_count": _exact_nonnegative_db_int(
                            normalized_preflight.get("distinct_row_id_count")
                        ),
                        "raw_txn_count": None,
                        "effective_txn_count": None,
                        "duplicate_row_count": None,
                        "candidate_duplicate_amount": None,
                        "blocker": blocker,
                    },
                    "rankings": [],
                    "query_id": "",
                    "warnings": warnings,
                    "semantic_status": coverage_status,
                    "coverage_status": coverage_status,
                    "blocker": blocker,
                    "fact_answer_allowed": False,
                    "summary_text": "同事实去重证据覆盖不完整或范围未授权；未执行排行，需补齐复合键后重试。",
                }

            dedupe_preflight: dict[str, int] | None = None
            same_holder_dedupe_enabled = False
            if dedupe_requested:
                if not dedupe_scope_authorized:
                    return dedupe_boundary(
                        coverage_status="blocked",
                        blocker="same_fact_dedupe_scope_not_authorized",
                    )
                dedupe_preflight = self._same_fact_dedupe_preflight(
                    engine,
                    where_sql=where_sql,
                    params=params,
                )
                if dedupe_preflight is None:
                    return dedupe_boundary(
                        coverage_status="unresolved",
                        blocker="same_fact_dedupe_preflight_unavailable",
                    )
                requested_txn_count = _exact_nonnegative_db_int(
                    dedupe_preflight.get("requested_txn_count")
                )
                scoped_txn_count = _exact_nonnegative_db_int(scope_stats.get("txn_count"))
                if requested_txn_count == 0:
                    return dedupe_boundary(
                        coverage_status="empty_unverified",
                        blocker="same_fact_dedupe_scope_empty_unverified",
                        preflight=dedupe_preflight,
                    )
                scope_coverage_status = _trim(scope_stats.get("coverage_status")) or "unresolved"
                if scope_coverage_status != "complete":
                    return dedupe_boundary(
                        coverage_status=scope_coverage_status,
                        blocker="ranking_scope_coverage_incomplete",
                        preflight=dedupe_preflight,
                    )
                if requested_txn_count is None or scoped_txn_count != requested_txn_count:
                    return dedupe_boundary(
                        coverage_status="partial",
                        blocker="same_fact_dedupe_scope_count_mismatch",
                        preflight=dedupe_preflight,
                    )
                if (
                    _exact_nonnegative_db_int(dedupe_preflight.get("eligible_txn_count"))
                    != requested_txn_count
                    or _exact_nonnegative_db_int(dedupe_preflight.get("distinct_row_id_count"))
                    != requested_txn_count
                    or _exact_nonnegative_db_int(dedupe_preflight.get("ineligible_txn_count")) != 0
                ):
                    return dedupe_boundary(
                        coverage_status="partial",
                        blocker="same_fact_dedupe_key_coverage_incomplete",
                        preflight=dedupe_preflight,
                    )
                same_holder_dedupe_enabled = True
            same_fact_key_sql = _same_fact_txn_key_sql()
            ranking_key_sql = """
              CASE
                WHEN {group_by_name} AND NULLIF(TRIM(COALESCE(cp_name, '')), '') IS NOT NULL THEN '__name__:' || TRIM(cp_name)
                WHEN NULLIF(cp_key, '') IS NOT NULL THEN cp_key
                WHEN NULLIF(cp_name, '') IS NOT NULL THEN '__name__:' || cp_name
                WHEN NULLIF(cp_raw, '') IS NOT NULL THEN '__raw__:' || cp_raw
                ELSE '__unknown__'
              END
            """.format(group_by_name="TRUE" if normalized_group_mode == "name" else "FALSE")
            rank_from_sql = "analysis_txn_detail_idx"
            rank_where_sql = where_sql
            rank_params = tuple(params)
            dedupe_stats: dict[str, Any] = {}
            if same_holder_dedupe_enabled:
                rank_from_sql = f"""
                (
                  SELECT *
                  FROM (
                    SELECT
                      *,
                      ROW_NUMBER() OVER (
                        PARTITION BY {same_fact_key_sql}
                        ORDER BY acct_key ASC NULLS LAST, id ASC NULLS LAST
                      ) AS same_fact_rank
                    FROM analysis_txn_detail_idx
                    WHERE {where_sql}
                  ) same_holder_fact_scope
                  WHERE same_fact_rank=1
                ) same_holder_fact_deduped
                """
                rank_where_sql = "1=1"
                try:
                    dedupe_rows = engine.query(
                        f"""
                    SELECT
                      COUNT(1),
                      SUM(ABS(amount)),
                      COUNT(amount),
                      COUNT(1) FILTER (WHERE same_fact_rank=1),
                      SUM(ABS(amount)) FILTER (WHERE same_fact_rank=1),
                      COUNT(amount) FILTER (WHERE same_fact_rank=1)
                    FROM (
                      SELECT
                        *,
                        ROW_NUMBER() OVER (
                          PARTITION BY {same_fact_key_sql}
                          ORDER BY acct_key ASC NULLS LAST, id ASC NULLS LAST
                        ) AS same_fact_rank
                      FROM analysis_txn_detail_idx
                      WHERE {where_sql}
                    ) same_holder_fact_scope
                    """,
                        tuple(params),
                    )
                except Exception:
                    return dedupe_boundary(
                        coverage_status="unresolved",
                        blocker="same_fact_dedupe_aggregate_unavailable",
                        preflight=dedupe_preflight,
                    )
                dedupe_row = _exact_single_query_row(dedupe_rows, width=6)
                if dedupe_row is None:
                    return dedupe_boundary(
                        coverage_status="unresolved",
                        blocker="same_fact_dedupe_aggregate_invalid",
                        preflight=dedupe_preflight,
                    )
                raw_txn_count = _exact_nonnegative_db_int(dedupe_row[0])
                raw_abs_amount = _optional_finite_float(dedupe_row[1])
                raw_amount_present_count = _exact_nonnegative_db_int(dedupe_row[2])
                dedup_txn_count = _exact_nonnegative_db_int(dedupe_row[3])
                dedup_abs_amount = _optional_finite_float(dedupe_row[4])
                dedup_amount_present_count = _exact_nonnegative_db_int(dedupe_row[5])
                expected_raw_count = _exact_nonnegative_db_int(
                    (dedupe_preflight or {}).get("requested_txn_count")
                )
                if (
                    raw_txn_count is None
                    or raw_txn_count != expected_raw_count
                    or raw_txn_count <= 0
                    or raw_abs_amount is None
                    or raw_abs_amount < 0
                    or raw_amount_present_count != raw_txn_count
                    or dedup_txn_count is None
                    or dedup_txn_count <= 0
                    or dedup_txn_count > raw_txn_count
                    or dedup_abs_amount is None
                    or dedup_abs_amount < 0
                    or dedup_abs_amount > raw_abs_amount + 0.000001
                    or dedup_amount_present_count != dedup_txn_count
                ):
                    return dedupe_boundary(
                        coverage_status="unresolved",
                        blocker="same_fact_dedupe_aggregate_invalid",
                        preflight=dedupe_preflight,
                    )
                dedupe_stats = {
                    "raw_txn_count": raw_txn_count,
                    "raw_abs_amount": raw_abs_amount,
                    "raw_amount_present_count": raw_amount_present_count,
                    "dedup_txn_count": dedup_txn_count,
                    "dedup_abs_amount": dedup_abs_amount,
                    "dedup_amount_present_count": dedup_amount_present_count,
                }
            rows = self._query_dicts(
                engine,
                f"""
                SELECT
                  {ranking_key_sql} AS ranking_key,
                  MAX(NULLIF(cp_key, '')) AS representative_counterparty_key,
                  STRING_AGG(DISTINCT NULLIF(cp_key, ''), '|') AS counterparty_keys_text,
                  STRING_AGG(DISTINCT NULLIF(acct_key, ''), '|') AS source_accounts_text,
                  STRING_AGG(DISTINCT NULLIF(cp_name, ''), '|') AS alias_names_text,
                  COUNT(DISTINCT acct_key) AS account_count,
                  COUNT(DISTINCT NULLIF(cp_key, '')) AS counterparty_account_count,
                  COUNT(1) AS row_count,
                  SUM(CASE WHEN dc_val IN ('进','出') THEN 1 ELSE 0 END) AS txn_count,
                  SUM(CASE WHEN dc_val='进' THEN 1 ELSE 0 END) AS in_txn_count,
                  SUM(CASE WHEN dc_val='出' THEN 1 ELSE 0 END) AS out_txn_count,
                  SUM(CASE WHEN dc_val='进' AND amount IS NOT NULL THEN ABS(amount) END) AS inflow_total,
                  SUM(CASE WHEN dc_val='出' AND amount IS NOT NULL THEN ABS(amount) END) AS outflow_total,
                  MAX(CASE WHEN amount IS NOT NULL THEN ABS(amount) END) AS max_single_amount,
                  SUM(CASE WHEN dc_val='进' AND amount IS NOT NULL THEN 1 ELSE 0 END) AS in_amount_present_count,
                  SUM(CASE WHEN dc_val='出' AND amount IS NOT NULL THEN 1 ELSE 0 END) AS out_amount_present_count,
                  SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END) AS amount_present_count,
                  MIN(CAST(txn_ts AS VARCHAR)) AS first_txn_at,
                  MAX(CAST(txn_ts AS VARCHAR)) AS last_txn_at
                FROM {rank_from_sql}
                WHERE {rank_where_sql}
                GROUP BY 1
                """,
                rank_params,
            )
            cluster_rows = self._query_dicts(
                engine,
                f"""
                SELECT ranking_key, CAST(txn_date AS VARCHAR) AS txn_date, txn_count, amount_present_count, amount_total
                FROM (
                  SELECT
                    {ranking_key_sql} AS ranking_key,
                    CAST(txn_ts AS DATE) AS txn_date,
                    COUNT(1) AS txn_count,
                    SUM(CASE WHEN amount IS NOT NULL THEN 1 ELSE 0 END) AS amount_present_count,
                    SUM(ABS(amount)) AS amount_total,
                    ROW_NUMBER() OVER (
                      PARTITION BY {ranking_key_sql}
                      ORDER BY SUM(ABS(amount)) DESC NULLS LAST, COUNT(1) DESC, MIN(txn_ts) ASC
                    ) AS cluster_rank
                  FROM {rank_from_sql}
                  WHERE {rank_where_sql}
                  GROUP BY 1, 2
                ) ranked_clusters
                WHERE cluster_rank=1
                """,
                rank_params,
            )
            largest_date_clusters = {
                _trim(row.get("ranking_key")): {
                    "date": _trim(row.get("txn_date")),
                    "txn_count": _optional_nonnegative_int(row.get("txn_count")),
                    "amount": _complete_aggregate_amount(
                        row.get("amount_total"),
                        expected_count=row.get("txn_count"),
                        present_count=row.get("amount_present_count"),
                    ),
                }
                for row in cluster_rows
                if _trim(row.get("ranking_key"))
            }
            duplicate_account_rows: list[dict[str, Any]] = []
            if same_holder_dedupe_enabled:
                duplicate_account_rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      {ranking_key_sql} AS ranking_key,
                      STRING_AGG(DISTINCT CASE WHEN same_fact_rank=1 THEN NULLIF(acct_key, '') ELSE NULL END, '|') AS source_accounts_text,
                      STRING_AGG(DISTINCT CASE WHEN same_fact_rank>1 THEN NULLIF(acct_key, '') ELSE NULL END, '|') AS duplicate_source_accounts_text,
                      SUM(CASE WHEN same_fact_rank>1 THEN 1 ELSE 0 END) AS duplicate_row_count,
                      SUM(CASE WHEN same_fact_rank>1 AND amount IS NOT NULL THEN ABS(amount) END) AS duplicate_abs_amount,
                      SUM(CASE WHEN same_fact_rank>1 AND amount IS NOT NULL THEN 1 ELSE 0 END) AS duplicate_amount_present_count
                    FROM (
                      SELECT
                        *,
                        ROW_NUMBER() OVER (
                          PARTITION BY {same_fact_key_sql}
                          ORDER BY acct_key ASC NULLS LAST, id ASC NULLS LAST
                        ) AS same_fact_rank
                      FROM analysis_txn_detail_idx
                      WHERE {where_sql}
                    ) same_holder_fact_scope
                    GROUP BY 1
                    """,
                    tuple(params),
                )
            duplicate_account_summaries = {
                _trim(row.get("ranking_key")): {
                    "source_accounts": [
                        item
                        for item in _trim_list(str(row.get("source_accounts_text") or "").split("|"))
                        if item and not item.startswith("__")
                    ],
                    "duplicate_source_accounts": [
                        item
                        for item in _trim_list(str(row.get("duplicate_source_accounts_text") or "").split("|"))
                        if item and not item.startswith("__")
                    ],
                    "duplicate_row_count": _optional_nonnegative_int(row.get("duplicate_row_count")),
                    "duplicate_amount": _complete_aggregate_amount(
                        row.get("duplicate_abs_amount"),
                        expected_count=row.get("duplicate_row_count"),
                        present_count=row.get("duplicate_amount_present_count"),
                    ),
                }
                for row in duplicate_account_rows
                if _trim(row.get("ranking_key"))
            }
            normalized_rows: list[dict[str, Any]] = []
            for row in rows:
                alias_names = [item for item in _trim_list(str(row.get("alias_names_text") or "").split("|")) if item]
                ranking_key = _trim(row.get("ranking_key")) or "__unknown__"
                representative_counterparty_key = _trim(row.get("representative_counterparty_key"))
                counterparty_key = ranking_key if ranking_key.startswith("__name__:") else (representative_counterparty_key or ranking_key)
                if ranking_key.startswith("__name__:"):
                    display_name = _trim(ranking_key.replace("__name__:", "", 1)) or _normalize_counterparty_display_name(counterparty_key, alias_names)
                else:
                    display_name = _normalize_counterparty_display_name(counterparty_key, alias_names)
                counterparty_accounts = [
                    item
                    for item in _trim_list(str(row.get("counterparty_keys_text") or "").split("|"))
                    if item and not item.startswith("__")
                ]
                source_accounts = [
                    item
                    for item in _trim_list(str(row.get("source_accounts_text") or "").split("|"))
                    if item and not item.startswith("__")
                ]
                duplicate_account_summary = duplicate_account_summaries.get(ranking_key, {})
                if duplicate_account_summary.get("source_accounts"):
                    source_accounts = list(duplicate_account_summary.get("source_accounts") or [])
                duplicate_source_accounts = list(duplicate_account_summary.get("duplicate_source_accounts") or [])
                counterparty_type = _classify_counterparty_type(counterparty_key, display_name)
                amount_facts = _complete_rank_amount_facts(row)
                normalized_rows.append(
                    {
                        "counterparty_key": counterparty_key,
                        "counterparty_account": _counterparty_account_value(counterparty_key) if normalized_group_mode == "account" else (counterparty_accounts[0] if len(counterparty_accounts) == 1 else ""),
                        "counterparty_accounts": counterparty_accounts[:50],
                        "counterparty_accounts_truncated": len(counterparty_accounts) > 50,
                        "counterparty_account_count": _optional_nonnegative_int(row.get("counterparty_account_count")),
                        "source_accounts": source_accounts[:50],
                        "source_accounts_truncated": len(source_accounts) > 50,
                        "source_account_count": _optional_nonnegative_int(row.get("account_count")),
                        "duplicate_source_accounts": duplicate_source_accounts[:50],
                        "duplicate_source_accounts_truncated": len(duplicate_source_accounts) > 50,
                        "duplicate_row_count": _optional_nonnegative_int(duplicate_account_summary.get("duplicate_row_count")),
                        "duplicate_amount": _optional_rounded_float(duplicate_account_summary.get("duplicate_amount"), 2),
                        "counterparty_group_mode": normalized_group_mode,
                        "largest_date_cluster": largest_date_clusters.get(ranking_key, {}),
                        "display_name": display_name,
                        "alias_names": alias_names[:20],
                        "counterparty_type": counterparty_type,
                        "counterparty_type_label": _counterparty_type_label(counterparty_type),
                        "account_count": _optional_nonnegative_int(row.get("account_count")),
                        "row_count": _optional_nonnegative_int(row.get("row_count")),
                        **amount_facts,
                        "first_txn_at": _trim(row.get("first_txn_at")),
                        "last_txn_at": _trim(row.get("last_txn_at")),
                    }
                )
            eligible_rows = _sort_rank_rows(normalized_rows, metric=normalized_metric, tie_field="display_name")
            ranked_rows = eligible_rows[:normalized_limit]
            coverage_status = _rank_result_coverage_status(
                scope_stats,
                candidate_count=len(normalized_rows),
                eligible_count=len(eligible_rows),
            )
            query_id = ""
            if coverage_status == "complete":
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="rank_counterparties",
                    params={
                        "case_id": case_id,
                        "scope": {
                            "scope_type": str(scope.get("scope_type") or "case"),
                            "selected_accounts": selected_accounts,
                            "holder_name": _trim(holder_name),
                            "id_no_provided": bool(_trim(id_no)),
                        },
                        "source_file_ids": normalized_file_ids,
                        "date_start": _trim(date_start),
                        "date_end": _trim(date_end),
                        "direction_mode": _trim(direction_mode) or "both",
                        "success_filter": _trim(success_filter) or "all",
                        "cash_filter": _trim(cash_filter) or "all",
                        "metric": normalized_metric,
                        "limit": normalized_limit,
                    },
                    summary={"top": ranked_rows[:3], "scope_stats": scope_stats},
                    row_count=_optional_nonnegative_int(scope_stats.get("txn_count")),
                    duration_ms=int((time.perf_counter() - started) * 1000),
                )
            group_amount_facts = _rank_scope_amount_facts(scope_stats)
            warnings = _merge_rank_scope_warnings(scope, lifecycle_warnings)
            raw_txn_count = _optional_nonnegative_int(dedupe_stats.get("raw_txn_count"))
            dedup_txn_count = _optional_nonnegative_int(dedupe_stats.get("dedup_txn_count"))
            duplicate_rows = (
                max(0, raw_txn_count - dedup_txn_count)
                if raw_txn_count is not None and dedup_txn_count is not None
                else None
            )
            raw_abs_amount = _complete_aggregate_amount(
                dedupe_stats.get("raw_abs_amount"),
                expected_count=raw_txn_count,
                present_count=dedupe_stats.get("raw_amount_present_count"),
            )
            dedup_abs_amount = _complete_aggregate_amount(
                dedupe_stats.get("dedup_abs_amount"),
                expected_count=dedup_txn_count,
                present_count=dedupe_stats.get("dedup_amount_present_count"),
            )
            duplicate_amount_delta = (
                round(max(0.0, raw_abs_amount - dedup_abs_amount), 2)
                if raw_abs_amount is not None and dedup_abs_amount is not None
                else None
            )
            if coverage_status != "complete":
                warnings.append(
                    {
                        "code": "RANKING_FACT_COVERAGE_INCOMPLETE",
                        "message": "排名所需事实覆盖不完整；未知值未按 0 处理，未形成全范围排名结论。",
                        "severity": "warning",
                    }
                )
            if same_holder_dedupe_enabled and coverage_status == "complete":
                warnings.append(
                    {
                        "code": "RANK_COUNTERPARTIES_SAME_HOLDER_FACT_DEDUPED",
                        "message": (
                            "本次对手方排名在同一主体账户集合内按有效 txn_id 折叠同事实重复；"
                            f"折叠额外行 {duplicate_rows if duplicate_rows is not None else '未核验'} 行，"
                            f"候选重复金额 {format(duplicate_amount_delta, '.2f') if duplicate_amount_delta is not None else '未核验'} 元。"
                        ),
                        "severity": "info",
                    }
                )
            return {
                "scope": {
                    "case_id": case_id,
                    "scope_type": str(scope.get("scope_type") or "case"),
                    "selected_accounts": selected_accounts,
                    "holder_name": _trim(holder_name),
                    "id_no_provided": bool(_trim(id_no)),
                    "match_mode": _trim(match_mode) or "exact",
                    "source_file_ids": normalized_file_ids,
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "direction_mode": _trim(direction_mode) or "both",
                    "success_filter": _trim(success_filter) or "all",
                    "cash_filter": _trim(cash_filter) or "all",
                    "metric": normalized_metric,
                    "limit": normalized_limit,
                    "dedupe_same_holder_same_fact": bool(same_holder_dedupe_enabled),
                    "counterparty_group_mode": normalized_group_mode,
                },
                "group_summary": {
                    "txn_count": _optional_nonnegative_int(scope_stats.get("txn_count")),
                    "directed_txn_count": _optional_nonnegative_int(scope_stats.get("directed_txn_count")),
                    "undirected_txn_count": _optional_nonnegative_int(scope_stats.get("undirected_txn_count")),
                    "counterparty_count": len(normalized_rows),
                    "account_count": _optional_nonnegative_int(scope_stats.get("account_count")),
                    **group_amount_facts,
                    "first_txn_at": _trim(scope_stats.get("first_txn_at")),
                    "last_txn_at": _trim(scope_stats.get("last_txn_at")),
                    "dedupe_same_holder_same_fact": bool(same_holder_dedupe_enabled),
                    "dedupe_raw_txn_count": raw_txn_count,
                    "dedupe_txn_count": dedup_txn_count,
                    "dedupe_candidate_duplicate_amount": duplicate_amount_delta,
                },
                "same_fact_dedupe": {
                    "contract": "SameFactDedupeCoverageV1",
                    "key_version": SAME_FACT_KEY_VERSION,
                    "requested": dedupe_requested,
                    "scope_authorized": dedupe_scope_authorized,
                    "applied": same_holder_dedupe_enabled,
                    "coverage_status": "complete" if same_holder_dedupe_enabled else "not_requested",
                    "requested_txn_count": (
                        _exact_nonnegative_db_int((dedupe_preflight or {}).get("requested_txn_count"))
                        if same_holder_dedupe_enabled
                        else None
                    ),
                    "eligible_txn_count": (
                        _exact_nonnegative_db_int((dedupe_preflight or {}).get("eligible_txn_count"))
                        if same_holder_dedupe_enabled
                        else None
                    ),
                    "ineligible_txn_count": (
                        _exact_nonnegative_db_int((dedupe_preflight or {}).get("ineligible_txn_count"))
                        if same_holder_dedupe_enabled
                        else None
                    ),
                    "distinct_row_id_count": (
                        _exact_nonnegative_db_int((dedupe_preflight or {}).get("distinct_row_id_count"))
                        if same_holder_dedupe_enabled
                        else None
                    ),
                    "raw_txn_count": raw_txn_count if same_holder_dedupe_enabled else None,
                    "effective_txn_count": dedup_txn_count if same_holder_dedupe_enabled else None,
                    "duplicate_row_count": duplicate_rows if same_holder_dedupe_enabled else None,
                    "candidate_duplicate_amount": duplicate_amount_delta if same_holder_dedupe_enabled else None,
                    "blocker": None,
                },
                "rankings": ranked_rows,
                "query_id": query_id,
                "warnings": warnings,
                "coverage_status": coverage_status,
                "fact_answer_allowed": False,
                "summary_text": (
                    f"已按 {normalized_metric} 对 {len(normalized_rows)} 个对手方完成排名，返回 Top{len(ranked_rows)}。"
                    if coverage_status == "complete"
                    else "排名事实覆盖不完整；仅保留可核验字段，不发布全范围排名结论。"
                ),
            }
        finally:
            engine.close()

    def resolve_owner_scope(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = True,
        candidate_min_turnover: float = 100000.0,
        limit: int = 50,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        normalized_holder = _trim(holder_name)
        normalized_id_no = _trim(id_no)
        normalized_candidate_min_turnover = max(0.0, _as_float(candidate_min_turnover))
        direct_accounts = _trim_list(account_keys or [])
        direct_scope: dict[str, Any] = {}
        warnings: list[dict[str, Any]] = []
        if normalized_holder or normalized_id_no:
            direct_scope = self.resolve_holder_scope(
                case_id,
                holder_name=normalized_holder,
                id_no=normalized_id_no,
                match_mode="exact",
            )
            direct_accounts = _unique_texts([*direct_accounts, *list(direct_scope.get("account_keys") or [])])
            warnings.extend(list(direct_scope.get("warnings") or []))

        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)

            def _empty_owner_coverage(accounts: Sequence[str]) -> dict[str, Any]:
                total_rows = engine.query("SELECT COUNT(1) FROM analysis_txn_detail_idx")
                txn_total = _optional_nonnegative_int(total_rows[0][0]) if total_rows and total_rows[0] else None
                return {
                    "coverage": {
                        "case_id": case_id,
                        "scope": "account_group" if len(accounts) > 1 else "account",
                        "selected_accounts": list(accounts),
                        "account_count": len(accounts) if accounts else None,
                        "txn_total": txn_total,
                        "txn_analyzed": None,
                        "directed_transaction_rows": None,
                        "undirected_transaction_rows": None,
                        "inflow_total": None,
                        "outflow_total": None,
                        "turnover_total": None,
                        "txn_excluded": None,
                        "source_file_count": None,
                        "date_min": "",
                        "date_max": "",
                        "field_quality": {},
                        "coverage_status": "unresolved",
                    },
                    "scope_stats": {},
                    "warnings": [],
                    "query_id": "",
                    "duration_ms": int((time.perf_counter() - started) * 1000),
                }

            direct_coverage = (
                self._build_scope_coverage_with_engine(
                    engine,
                    case_id=case_id,
                    account_keys=direct_accounts,
                    started=started,
                )
                if direct_accounts
                else _empty_owner_coverage([])
            )
            candidate_rows: list[dict[str, Any]] = []
            if include_candidate_accounts and (normalized_holder or direct_accounts):
                direct_aliases = _account_aliases(direct_accounts)
                link_clauses: list[str] = []
                params: list[Any] = []
                if normalized_holder:
                    link_clauses.append("cp_name = ?")
                    params.append(normalized_holder)
                if direct_aliases:
                    link_clauses.append(f"cp_key IN ({','.join(['?'] * len(direct_aliases))})")
                    params.extend(direct_aliases)
                if link_clauses:
                    normalized_limit = max(1, min(int(limit or 50), 200))
                    candidate_rows = self._query_dicts(
                        engine,
                        f"""
                        SELECT
                          t.acct_key AS account_key,
                          COUNT(1) AS txn_count,
                          COUNT(t.amount) AS amount_present_count,
                          COUNT(1) FILTER (WHERE t.dc_val='进') AS in_txn_count,
                          COUNT(1) FILTER (WHERE t.dc_val='出') AS out_txn_count,
                          COUNT(t.amount) FILTER (WHERE t.dc_val='进') AS in_amount_present_count,
                          COUNT(t.amount) FILTER (WHERE t.dc_val='出') AS out_amount_present_count,
                          SUM(ABS(t.amount)) FILTER (WHERE t.dc_val='进') AS inflow_total,
                          SUM(ABS(t.amount)) FILTER (WHERE t.dc_val='出') AS outflow_total,
                          MAX(ABS(t.amount)) AS max_single_amount,
                          MIN(CAST(t.txn_ts AS VARCHAR)) AS first_txn_at,
                          MAX(CAST(t.txn_ts AS VARCHAR)) AS last_txn_at,
                          STRING_AGG(DISTINCT NULLIF(t.cp_name, ''), '|') AS linked_names_text,
                          STRING_AGG(DISTINCT NULLIF(t.cp_key, ''), '|') AS linked_accounts_text
                        FROM analysis_txn_detail_idx t
                        LEFT JOIN analysis_account_dim d ON d.account_key = t.acct_key
                        WHERE t.acct_key IS NOT NULL
                          AND TRIM(t.acct_key) <> ''
                          AND COALESCE(NULLIF(TRIM(d.open_name), ''), '') = ''
                          AND ({' OR '.join(f'({clause})' for clause in link_clauses)})
                          AND t.acct_key NOT IN ({','.join(['?'] * len(direct_accounts)) if direct_accounts else "''"})
                        GROUP BY t.acct_key
                        HAVING COUNT(1) > 0
                           AND COUNT(t.amount) = COUNT(1)
                           AND SUM(ABS(t.amount)) >= ?
                        ORDER BY SUM(ABS(t.amount)) DESC, COUNT(1) DESC, t.acct_key ASC
                        LIMIT ?
                        """,
                        tuple([*params, *direct_accounts, normalized_candidate_min_turnover, normalized_limit]),
                    )
            candidate_accounts: list[dict[str, Any]] = []
            for row in candidate_rows:
                amount_facts = _complete_rank_amount_facts(row)
                candidate_accounts.append(
                    {
                        "account_key": _trim(row.get("account_key")),
                        "provenance": "counterparty_link_missing_holder",
                        "confidence": "candidate",
                        "txn_count": amount_facts["txn_count"],
                        "inflow_total": amount_facts["inflow_total"],
                        "outflow_total": amount_facts["outflow_total"],
                        "turnover_total": amount_facts["turnover_total"],
                        "max_single_amount": amount_facts["max_single_amount"],
                        "first_txn_at": _trim(row.get("first_txn_at")),
                        "last_txn_at": _trim(row.get("last_txn_at")),
                        "linked_names": _trim_list(str(row.get("linked_names_text") or "").split("|"))[:10],
                        "linked_accounts": _trim_list(str(row.get("linked_accounts_text") or "").split("|"))[:10],
                    }
                )
            candidate_account_keys = [item["account_key"] for item in candidate_accounts if item.get("account_key")]
            expanded_accounts = _unique_texts([*direct_accounts, *candidate_account_keys])
            expanded_coverage = (
                self._build_scope_coverage_with_engine(
                    engine,
                    case_id=case_id,
                    account_keys=expanded_accounts,
                    started=started,
                )
                if expanded_accounts
                else _empty_owner_coverage([])
            )
            unresolved_rows = self._query_dicts(
                engine,
                """
                SELECT
                  t.acct_key AS account_key,
                  COUNT(1) AS txn_count,
                  COUNT(t.amount) AS amount_present_count,
                  COUNT(1) FILTER (WHERE t.dc_val='进') AS in_txn_count,
                  COUNT(1) FILTER (WHERE t.dc_val='出') AS out_txn_count,
                  COUNT(t.amount) FILTER (WHERE t.dc_val='进') AS in_amount_present_count,
                  COUNT(t.amount) FILTER (WHERE t.dc_val='出') AS out_amount_present_count,
                  SUM(ABS(t.amount)) FILTER (WHERE t.dc_val='进') AS inflow_total,
                  SUM(ABS(t.amount)) FILTER (WHERE t.dc_val='出') AS outflow_total,
                  MAX(ABS(t.amount)) AS max_single_amount
                FROM analysis_txn_detail_idx t
                LEFT JOIN analysis_account_dim d ON d.account_key = t.acct_key
                WHERE t.acct_key IS NOT NULL
                  AND TRIM(t.acct_key) <> ''
                  AND COALESCE(NULLIF(TRIM(d.open_name), ''), '') = ''
                GROUP BY t.acct_key
                ORDER BY SUM(ABS(t.amount)) DESC NULLS LAST, COUNT(1) DESC, t.acct_key ASC
                LIMIT 20
                """,
            )
            unresolved_high_value = []
            for row in unresolved_rows:
                account_key = _trim(row.get("account_key"))
                if account_key in expanded_accounts:
                    continue
                amount_facts = _complete_rank_amount_facts(row)
                unresolved_high_value.append(
                    {
                        "account_key": account_key,
                        "txn_count": amount_facts["txn_count"],
                        "inflow_total": amount_facts["inflow_total"],
                        "outflow_total": amount_facts["outflow_total"],
                        "turnover_total": amount_facts["turnover_total"],
                        "max_single_amount": amount_facts["max_single_amount"],
                    }
                )
            if candidate_accounts:
                warnings.append(
                    {
                        "code": "OWNER_SCOPE_HAS_CANDIDATE_ACCOUNTS",
                        "message": "候选账户仅表示与主体存在交易或账号线索关联，不能直接写成名下或实际控制账户。",
                        "severity": "warning",
                    }
                )
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="resolve_owner_scope",
                params={
                    "case_id": case_id,
                    "holder_name": normalized_holder,
                    "id_no_provided": bool(normalized_id_no),
                    "include_candidate_accounts": include_candidate_accounts,
                    "candidate_min_turnover": normalized_candidate_min_turnover,
                    "limit": limit,
                },
                summary={
                    "direct_account_count": len(direct_accounts),
                    "candidate_account_count": len(candidate_accounts),
                    "expanded_account_count": len(expanded_accounts),
                    "expanded_txn_analyzed": dict(expanded_coverage.get("coverage") or {}).get("txn_analyzed"),
                },
                row_count=_optional_nonnegative_int(
                    dict(expanded_coverage.get("coverage") or {}).get("txn_analyzed")
                ),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "owner_scope_id": _stable_slug("owner_scope", case_id, normalized_holder, normalized_id_no, _json_dumps(expanded_accounts)),
                "subject": {"holder_name": normalized_holder, "id_no_provided": bool(normalized_id_no)},
                "direct_account_keys": direct_accounts,
                "candidate_accounts": candidate_accounts,
                "candidate_account_keys": candidate_account_keys,
                "expanded_account_keys": expanded_accounts,
                "direct_coverage": direct_coverage.get("coverage") or {},
                "expanded_coverage": expanded_coverage.get("coverage") or {},
                "unresolved_high_value_accounts": unresolved_high_value[:20],
                "query_id": query_id,
                "warnings": warnings,
                "interpretation": "direct_account_keys 可作为直接登记归属；candidate_accounts 只能作为需复核候选归属或资金通道线索。",
            }
        finally:
            engine.close()

    def inspect_case_schema(
        self,
        case_id: str,
        *,
        table_limit: int = 200,
        column_limit: int = 80,
        include_columns: bool = True,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        request_token = _json_dumps(
            {
                "table_limit": max(1, min(_as_int(table_limit or 200), 500)),
                "column_limit": max(0, min(_as_int(column_limit or 80), 200)),
                "include_columns": bool(include_columns),
            }
        )
        return project_case_sql_capability_boundary(
            case_id=case_id,
            capability="schema_inventory",
            request_token=request_token,
        )

    def _validate_workbench_sql(
        self,
        engine: DuckDBEngine,
        sql: str,
        *,
        allow_select_star_with_limit: bool = False,
    ) -> tuple[str, list[str], list[str]]:
        stripped = _strip_sql_comments(sql).strip()
        if not stripped:
            raise ValueError("run_case_sql requires a non-empty SQL statement")
        statements = _split_sql_statements(stripped)
        if len(statements) != 1:
            raise ValueError("Controlled Case Workbench accepts exactly one SELECT/WITH statement")
        statement = statements[0].strip()
        masked = _mask_sql_literals(statement)
        normalized_masked = masked.strip().lower()
        if not (normalized_masked.startswith("select ") or normalized_masked.startswith("with ")):
            raise ValueError("Controlled Case Workbench SQL must start with SELECT or WITH")
        ast_base_tables, ast_cte_names = _validate_workbench_sql_ast(
            statement,
            allow_select_star_with_limit=allow_select_star_with_limit,
        )
        if _WORKBENCH_FORBIDDEN_SQL_RE.search(masked):
            raise ValueError("DDL/DML/extension/export statements are not allowed")
        if _WORKBENCH_EXTERNAL_SQL_RE.search(masked):
            raise ValueError("external file/network/secret access is not allowed")
        if _WORKBENCH_INCOMPLETE_SQL_RE.search(masked):
            raise ValueError("Controlled Case Workbench SQL must be complete; ellipsis/placeholders are not executable")
        if (
            not allow_select_star_with_limit
            and (
                re.search(r"\bselect\s+\*", masked, flags=re.IGNORECASE)
                or re.search(r"\b[A-Za-z_][A-Za-z0-9_]*\s*\.\s*\*", masked)
            )
        ):
            raise ValueError("SELECT * is not allowed; request explicit columns or aggregates")
        if re.search(r"\bfc_[A-Za-z0-9_]*_raw\b", masked, flags=re.IGNORECASE):
            raise ValueError("raw/source tables are not allowed in Controlled Case Workbench")

        regex_cte_names = _workbench_cte_names(masked)
        referenced_tables = _workbench_referenced_tables(masked)
        regex_base_tables = [table for table in referenced_tables if table not in regex_cte_names]
        cte_names = set(ast_cte_names) | set(regex_cte_names)
        base_tables = ast_base_tables or regex_base_tables
        if not base_tables:
            raise ValueError("Controlled Case Workbench SQL must reference at least one cleaned fc_*_norm or approved analysis table")
        blocked_tables = [table for table in base_tables if not _is_allowed_workbench_table(table)]
        if blocked_tables:
            raise ValueError(
                "Controlled Case Workbench can only read cleaned fc_*_norm and approved analysis tables: "
                + ", ".join(blocked_tables[:5])
            )
        missing_tables = [table for table in base_tables if not self._table_exists(engine, table)]
        if missing_tables:
            raise ValueError("requested workbench table is unavailable in the current case: " + ", ".join(missing_tables[:5]))
        return statement, base_tables, sorted(cte_names)

    def explain_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str,
        row_limit: int = 100,
        include_analyze: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if not _trim(purpose):
            raise ValueError("case_sql_request_rejected")
        requested_sql = _trim(sql)
        if not requested_sql:
            raise ValueError("case_sql_request_rejected")
        started = time.perf_counter()
        limit = max(1, min(_as_int(row_limit or 100), 500))

        def run(engine: DuckDBEngine) -> dict[str, Any]:
            self.ensure_analysis_schema(engine)
            sql_digest = hashlib.sha256(requested_sql.encode("utf-8")).hexdigest()[:16]
            try:
                statement, _, _ = self._validate_workbench_sql(
                    engine,
                    requested_sql,
                    allow_select_star_with_limit=True,
                )
            except ValueError as exc:
                duration_ms = int((time.perf_counter() - started) * 1000)
                error_class = _classify_case_sql_exception(exc)
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="explain_case_sql",
                    params={"sql_digest": sql_digest, "row_limit": limit, "diagnostic_mode": "explain"},
                    summary={"execution_status": "blocked_by_guardrail", "error_class": error_class},
                    row_count=0,
                    duration_ms=duration_ms,
                )
                return project_case_sql_diagnostic(
                    case_id=case_id,
                    query_id=query_id,
                    sql_digest=sql_digest,
                    execution_status="blocked_by_guardrail",
                    diagnostic_mode="explain",
                    error_class=error_class,
                )

            try:
                # Ordinary diagnostics never execute the statement.  EXPLAIN ANALYZE
                # is deliberately downgraded to EXPLAIN until a controlled artifact
                # grant exists at the host boundary.
                plan_rows = self._query_dicts(engine, f"EXPLAIN {statement}")
                plan_lines: list[str] = []
                for row in plan_rows:
                    values = [_trim(value) for value in row.values() if _trim(value)]
                    if values:
                        plan_lines.append(" | ".join(values))
                plan_text = "\n".join(plan_lines)
                plan_summary = _case_sql_plan_summary(plan_text)
                duration_ms = int((time.perf_counter() - started) * 1000)
                sql_digest = hashlib.sha256(statement.encode("utf-8")).hexdigest()[:16]
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="explain_case_sql",
                    params={"sql_digest": sql_digest, "row_limit": limit, "diagnostic_mode": "explain"},
                    summary={
                        "execution_status": "explain_succeeded",
                        "error_class": "performance_risk" if plan_summary.get("full_table_scan_possible") else "none",
                        "plan": {
                            "full_scan_possible": plan_summary.get("full_table_scan_possible"),
                            "estimated_row_upper_bound": plan_summary.get("estimated_row_upper_bound"),
                        },
                    },
                    row_count=0,
                    duration_ms=duration_ms,
                )
                return project_case_sql_diagnostic(
                    case_id=case_id,
                    query_id=query_id,
                    sql_digest=sql_digest,
                    execution_status="explain_succeeded",
                    diagnostic_mode="explain",
                    error_class="performance_risk" if plan_summary.get("full_table_scan_possible") else "none",
                    plan_summary=plan_summary,
                )
            except Exception as exc:
                duration_ms = int((time.perf_counter() - started) * 1000)
                error_class = _classify_case_sql_exception(exc)
                query_id = self._append_query_log(
                    engine,
                    case_id=case_id,
                    tool_name="explain_case_sql",
                    params={"sql_digest": sql_digest, "row_limit": limit, "diagnostic_mode": "explain"},
                    summary={"execution_status": "explain_failed", "error_class": error_class},
                    row_count=0,
                    duration_ms=duration_ms,
                )
                return project_case_sql_diagnostic(
                    case_id=case_id,
                    query_id=query_id,
                    sql_digest=sql_digest,
                    execution_status="explain_failed",
                    diagnostic_mode="explain",
                    error_class=error_class,
                )

        return self._with_case_engine_retry(case_id, run, read_only=True, max_attempts=4)

    def diagnose_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str = "",
        error_message: str = "",
        diagnostic_mode: str = "validate",
        row_limit: int = 100,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        requested_sql = _trim(sql)
        if not _trim(purpose):
            raise ValueError("case_sql_request_rejected")
        mode = _trim(diagnostic_mode).lower() or "validate"
        if mode not in {"validate", "empty_result", "performance", "error"}:
            mode = "validate"
        limit = max(1, min(_as_int(row_limit or 100), 500))

        def run(engine: DuckDBEngine) -> dict[str, Any]:
            self.ensure_analysis_schema(engine)
            sql_digest = hashlib.sha256(requested_sql.encode("utf-8")).hexdigest()[:16] if requested_sql else ""
            validation_failed = False
            source_error_present = bool(_trim(error_message))
            error_class = _classify_case_sql_error(_trim(error_message)) if source_error_present else "none"
            statement = ""
            if requested_sql:
                try:
                    statement, _, _ = self._validate_workbench_sql(
                        engine,
                        requested_sql,
                        allow_select_star_with_limit=True,
                    )
                    sql_digest = hashlib.sha256(statement.encode("utf-8")).hexdigest()[:16]
                except ValueError as exc:
                    validation_failed = True
                    source_error_present = True
                    error_class = _classify_case_sql_exception(exc)
            result_check_status = "not_checked"
            plan_summary: dict[str, Any] = {}

            if statement and not source_error_present:
                try:
                    plan_rows = self._query_dicts(engine, f"EXPLAIN {statement}")
                    plan_text = "\n".join(" | ".join(_trim(value) for value in row.values() if _trim(value)) for row in plan_rows)
                    plan_summary = _case_sql_plan_summary(plan_text)
                    if mode == "empty_result":
                        count_rows = engine.query(f"SELECT COUNT(1) FROM ({statement}) AS diagnostic_query")
                        count_row = _exact_single_query_row(count_rows, width=1)
                        count_value = _exact_nonnegative_db_int(count_row[0]) if count_row is not None else None
                        if count_value is None:
                            raise ValueError("case_sql_count_result_invalid")
                        if count_value == 0:
                            result_check_status = "no_rows_observed"
                            error_class = "empty_result"
                        else:
                            result_check_status = "rows_observed"
                    if mode == "performance" and plan_summary.get("full_table_scan_possible"):
                        error_class = "performance_risk"
                except Exception as exc:
                    source_error_present = True
                    error_class = _classify_case_sql_exception(exc)

            duration_ms = int((time.perf_counter() - started) * 1000)
            execution_status = "diagnosed"
            if validation_failed:
                execution_status = "blocked_by_guardrail"
            elif source_error_present:
                execution_status = "diagnosed_error"
            elif error_class == "none":
                execution_status = "validated"
            summary = {
                "execution_status": execution_status,
                "error_class": error_class,
                "diagnostic_mode": mode,
                "result_check_status": result_check_status,
                "plan": {
                    "full_scan_possible": plan_summary.get("full_table_scan_possible"),
                    "estimated_row_upper_bound": plan_summary.get("estimated_row_upper_bound"),
                },
            }
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="diagnose_case_sql",
                params={"sql_digest": sql_digest, "diagnostic_mode": mode, "row_limit": limit},
                summary=summary,
                row_count=0,
                duration_ms=duration_ms,
            )
            return project_case_sql_diagnostic(
                case_id=case_id,
                query_id=query_id,
                sql_digest=sql_digest,
                execution_status=execution_status,
                diagnostic_mode=mode,
                error_class=error_class,
                result_check_status=result_check_status,
                plan_summary=plan_summary,
            )

        return self._with_case_engine_retry(case_id, run, read_only=True, max_attempts=4)

    def profile_case_schema(
        self,
        case_id: str,
        *,
        tables: Optional[Sequence[str]] = None,
        table_limit: int = 8,
        column_limit: int = 24,
        enum_limit: int = 8,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        request_token = hashlib.sha256(
            _json_dumps(
                {
                    "tables": _trim_list(tables or []),
                    "table_limit": max(1, min(_as_int(table_limit or 8), 20)),
                    "column_limit": max(1, min(_as_int(column_limit or 24), 80)),
                    "enum_limit": max(1, min(_as_int(enum_limit or 8), 20)),
                }
            ).encode("utf-8")
        ).hexdigest()
        return project_case_sql_capability_boundary(
            case_id=case_id,
            capability="schema_profile",
            request_token=request_token,
        )

    def preview_case_rows(
        self,
        case_id: str,
        *,
        purpose: str,
        table_name: str,
        columns: Optional[Sequence[str]] = None,
        where_sql: str = "",
        row_limit: int = 20,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if not _trim(purpose):
            raise ValueError("case_sql_request_rejected")
        request_digest = hashlib.sha256(
            _json_dumps(
                {
                    "table": _trim(table_name),
                    "columns": _trim_list(columns or []),
                    "where": _trim(where_sql),
                    "row_limit": max(1, min(_as_int(row_limit or 20), 20)),
                }
            ).encode("utf-8")
        ).hexdigest()[:16]
        query_ref = opaque_case_bound_ref(
            prefix="sqlquery_v1",
            case_id=case_id,
            value=request_digest,
        )
        # No ordinary caller carries a host-issued controlled-artifact grant in
        # this repository contract.  The P0 boundary therefore blocks execution
        # before DuckDB is opened and never creates a citation-shaped evidence id.
        return {
            "contract": "CaseSqlOrdinaryBoundaryV2",
            "query_ref": query_ref,
            "sql_digest": request_digest,
            "execution_status": "controlled_artifact_required",
            "result_access": "controlled_artifact_required",
            "columns": [],
            "records": [],
            "row_count": None,
            "row_count_status": "not_checked",
            "raw_rows_exposed": False,
            "fact_answer_allowed": False,
            "citation_eligible": False,
            "evidence_status": "unsupported",
            "evidence_ids": [],
            "warnings": [
                {
                    "code": "CASE_SQL_CONTROLLED_ARTIFACT_GRANT_REQUIRED",
                    "severity": "warning",
                    "remediation": "request_authorized_case_bound_artifact",
                }
            ],
        }

    def inspect_workbench_history(self, case_id: str, *, limit: int = 20) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        max_items = max(1, min(_as_int(limit or 20), 50))

        def run(engine: DuckDBEngine) -> dict[str, Any]:
            self.ensure_analysis_schema(engine)
            rows = self._query_dicts(
                engine,
                """
                SELECT query_id, case_id, tool_name, params_json, summary_json, row_count, duration_ms, created_at
                FROM analysis_query_log
                WHERE case_id=?
                ORDER BY created_at DESC
                LIMIT ?
                """,
                (case_id, max_items),
            )
            items: list[dict[str, Any]] = []
            for row in rows:
                if _trim(row.get("case_id")) != _trim(case_id):
                    raise RuntimeError("diagnostic_case_binding_mismatch")
                params = _json_loads(row.get("params_json"), {})
                summary = _json_loads(row.get("summary_json"), {})
                items.append(
                    project_workbench_history_item(
                        case_id=case_id,
                        row={**row, "params_json": params, "summary_json": summary},
                        host_registry_member=True,
                    )
                )
            return {
                "contract": "CaseWorkbenchHistoryPublicV2",
                "items": items,
                "returned_count": len(items),
                "limit": max_items,
                "raw_sql_exposed": False,
                "raw_rows_exposed": False,
                "fact_answer_allowed": False,
                "citation_eligible": False,
                "evidence_ids": [],
                "warnings": [],
            }

        return self._with_case_engine_retry(case_id, run, read_only=True, max_attempts=4)

    def case_sql_recipes(self, case_id: str, *, category: str = "", recipe_id: str = "", limit: int = 50) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_category = _trim(category).lower()
        normalized_recipe_id = _trim(recipe_id)
        max_items = max(1, min(_as_int(limit or 50), 100))
        recipes: list[dict[str, Any]] = []
        for recipe in _CASE_SQL_RECIPE_DEFINITIONS:
            if normalized_recipe_id and _trim(recipe.get("recipe_id")) != normalized_recipe_id:
                continue
            if normalized_category and _trim(recipe.get("category")).lower() != normalized_category:
                continue
            recipes.append(
                {
                    **recipe,
                    "source_scope": ["current_case", "cleaned fc_*_norm", "approved analysis_*"],
                    "rendering_policy": "template_requires_host_verified_execution",
                    "replayable": False,
                    "raw_rows_exposed": False,
                }
            )
            if len(recipes) >= max_items:
                break
        return {
            "contract": "CaseSqlRecipeGuidanceV2",
            "recipes": recipes,
            "returned_count": len(recipes),
            "available_categories": sorted({_trim(item.get("category")) for item in _CASE_SQL_RECIPE_DEFINITIONS}),
            "raw_rows_exposed": False,
            "fact_answer_allowed": False,
            "citation_eligible": False,
            "evidence_ids": [],
            "warnings": [],
        }

    def run_case_sql(
        self,
        case_id: str,
        *,
        purpose: str,
        sql: str = "",
        query_request: str = "",
        parameters: Optional[Mapping[str, Any]] = None,
        date_start: str = "",
        date_end: str = "",
        row_limit: int = 100,
        result_mode: str = "preview",
        allowed_view_policy: str = _WORKBENCH_ALLOWED_VIEW_POLICY,
        include_notebook_cell: bool = False,
        include_debug: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if not _trim(purpose):
            raise ValueError("case_sql_request_rejected")
        if _trim(allowed_view_policy) != _WORKBENCH_ALLOWED_VIEW_POLICY or parameters:
            raise ValueError("case_sql_request_rejected")
        requested_sql = _trim(sql)
        sql_digest = hashlib.sha256(requested_sql.encode("utf-8")).hexdigest()[:16] if requested_sql else ""
        query_ref = opaque_case_bound_ref(
            prefix="sqlquery_v1",
            case_id=case_id,
            value=sql_digest or "sql_not_provided",
        )
        # The repository has no host-issued controlled-artifact grant parameter.
        # Fail closed before opening DuckDB and do not mint an evidence id.
        return {
            "contract": "CaseSqlOrdinaryBoundaryV2",
            "query_ref": query_ref,
            "sql_digest": sql_digest,
            "execution_status": "controlled_artifact_required",
            "result_access": "controlled_artifact_required",
            "columns": [],
            "records": [],
            "row_count": None,
            "row_count_status": "not_checked",
            "raw_rows_exposed": False,
            "fact_answer_allowed": False,
            "citation_eligible": False,
            "evidence_status": "unsupported",
            "evidence_ids": [],
            "warnings": [
                {
                    "code": "CASE_SQL_CONTROLLED_ARTIFACT_GRANT_REQUIRED",
                    "severity": "warning",
                    "remediation": "request_authorized_case_bound_artifact",
                }
            ],
        }

    def create_case_notebook(
        self,
        case_id: str,
        *,
        title: str = "",
        analysis_goal: str,
        cells: Optional[Sequence[Mapping[str, Any]]] = None,
        max_rows_per_query: int = 100,
        allowed_view_policy: str = _WORKBENCH_ALLOWED_VIEW_POLICY,
        delivery_mode: str = "notebook_artifact",
        include_debug: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        if not _trim(analysis_goal):
            raise ValueError("case_sql_request_rejected")
        if _trim(allowed_view_policy) != _WORKBENCH_ALLOWED_VIEW_POLICY:
            raise ValueError("case_sql_request_rejected")
        request_digest = hashlib.sha256(_trim(analysis_goal).encode("utf-8")).hexdigest()
        return {
            "contract": "CaseWorkbenchNotebookBoundaryV2",
            "notebook_ref": opaque_case_bound_ref(
                prefix="sqlnotebook_v1",
                case_id=case_id,
                value=request_digest,
            ),
            "execution_status": "controlled_artifact_required",
            "result_access": "controlled_artifact_required",
            "write_performed": False,
            "cells": [],
            "query_refs": [],
            "evidence_ids": [],
            "raw_rows_exposed": False,
            "fact_answer_allowed": False,
            "citation_eligible": False,
            "warnings": [
                {
                    "code": "CASE_SQL_CONTROLLED_ARTIFACT_GRANT_REQUIRED",
                    "severity": "warning",
                    "remediation": "request_authorized_case_bound_artifact",
                }
            ],
        }

    def audit_unindexed_sources(
        self,
        case_id: str,
        *,
        table_limit: int = 200,
        column_limit: int = 80,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        request_token = _json_dumps(
            {
                "table_limit": max(1, min(_as_int(table_limit or 200), 500)),
                "column_limit": max(0, min(_as_int(column_limit or 80), 200)),
            }
        )
        return project_case_sql_capability_boundary(
            case_id=case_id,
            capability="unindexed_source_audit",
            request_token=request_token,
        )

    def compare_analysis_scopes(self, case_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            raw_rows = self._query_dicts(
                engine,
                """
                SELECT
                  COUNT(1) AS txn_total,
                  COUNT(1) FILTER (WHERE dc_val IN ('进','出')) AS directional_txn_count,
                  COUNT(DISTINCT acct_key) AS account_count,
                  COUNT(1) FILTER (WHERE dc_val='进') AS in_txn_count,
                  COUNT(1) FILTER (WHERE dc_val='出') AS out_txn_count,
                  COUNT(amount) FILTER (WHERE dc_val='进') AS in_amount_present_count,
                  COUNT(amount) FILTER (WHERE dc_val='出') AS out_amount_present_count,
                  SUM(ABS(amount)) FILTER (WHERE dc_val='进') AS inflow_total,
                  SUM(ABS(amount)) FILTER (WHERE dc_val='出') AS outflow_total,
                  MIN(CAST(txn_ts AS VARCHAR)) AS first_txn_at,
                  MAX(CAST(txn_ts AS VARCHAR)) AS last_txn_at
                FROM analysis_txn_detail_idx
                """
            )
            raw = raw_rows[0] if raw_rows else {}
            group_rows = self._query_dicts(
                engine,
                """
                WITH account_dim AS (
                  SELECT
                    d.account_key,
                    COALESCE(NULLIF(TRIM(d.open_name), ''), '') AS open_name,
                    COALESCE(NULLIF(TRIM(d.id_no), ''), '') AS id_no
                  FROM analysis_account_dim d
                ),
                tx AS (
                  SELECT
                    acct_key,
                    COUNT(1) AS txn_count,
                    COUNT(amount) AS amount_present_count,
                    SUM(ABS(amount)) AS turnover_total
                  FROM analysis_txn_detail_idx
                  GROUP BY acct_key
                ),
                groups AS (
                  SELECT
                    account_key,
                    CASE
                      WHEN open_name <> ''
                      THEN open_name || '|' || COALESCE(NULLIF(id_no, ''), '无证件号')
                      ELSE 'acct:' || COALESCE(account_key, '')
                    END AS by_name_group,
                    CASE
                      WHEN open_name <> '' THEN open_name
                      ELSE '未登记户名'
                    END AS holder_label,
                    CASE WHEN open_name = '' THEN 1 ELSE 0 END AS is_unregistered,
                    tx.txn_count AS txn_count,
                    tx.amount_present_count AS amount_present_count,
                    tx.turnover_total AS turnover_total
                  FROM account_dim
                  LEFT JOIN tx ON tx.acct_key = account_dim.account_key
                )
                SELECT
                  COUNT(DISTINCT by_name_group) AS ui_by_name_group_count,
                  COUNT(DISTINCT holder_label) AS holder_label_count,
                  COUNT(DISTINCT account_key) AS tree_account_count,
                  COUNT(DISTINCT account_key) FILTER (WHERE is_unregistered=1) AS unresolved_account_count,
                  SUM(txn_count) FILTER (WHERE is_unregistered=1) AS unresolved_txn_count,
                  SUM(amount_present_count) FILTER (WHERE is_unregistered=1) AS unresolved_amount_present_count,
                  SUM(turnover_total) FILTER (WHERE is_unregistered=1) AS unresolved_turnover_total
                FROM groups
                """
            )
            groups = group_rows[0] if group_rows else {}
            dedupe_rows = self._query_dicts(
                engine,
                """
                SELECT COUNT(DISTINCT
                  COALESCE(account_open_name, '') || '|' ||
                  COALESCE(opener_id_no, '') || '|' ||
                  COALESCE(dc_val, '') || '|' ||
                  COALESCE(CAST(txn_ts AS VARCHAR), '') || '|' ||
                  COALESCE(CAST(amount AS VARCHAR), '') || '|' ||
                  COALESCE(CAST(balance AS VARCHAR), '') || '|' ||
                  COALESCE(cp_key, '') || '|' ||
                  COALESCE(cp_name, '') || '|' ||
                  COALESCE(summary, '') || '|' ||
                  COALESCE(remark, '') || '|' ||
                  COALESCE(txn_type, '')
                ) AS report_dedupe_txn_count
                FROM analysis_txn_detail_idx
                WHERE dc_val IN ('进','出')
                """
            )
            dedupe_count = _optional_nonnegative_int(
                (dedupe_rows[0] if dedupe_rows else {}).get("report_dedupe_txn_count")
            )
            txn_total = _optional_nonnegative_int(raw.get("txn_total"))
            directional_txn_count = _optional_nonnegative_int(raw.get("directional_txn_count"))
            account_count = _optional_nonnegative_int(raw.get("account_count"))
            inflow_total = _complete_aggregate_amount(
                raw.get("inflow_total"),
                expected_count=raw.get("in_txn_count"),
                present_count=raw.get("in_amount_present_count"),
            )
            outflow_total = _complete_aggregate_amount(
                raw.get("outflow_total"),
                expected_count=raw.get("out_txn_count"),
                present_count=raw.get("out_amount_present_count"),
            )
            unresolved_txn_count = _optional_nonnegative_int(groups.get("unresolved_txn_count"))
            unresolved_turnover_total = _complete_aggregate_amount(
                groups.get("unresolved_turnover_total"),
                expected_count=unresolved_txn_count,
                present_count=groups.get("unresolved_amount_present_count"),
            )
            scopes = [
                {
                    "scope_id": "detail_index_all_rows",
                    "label": "分析索引全量行",
                    "txn_count": txn_total,
                    "account_count": account_count,
                    "inflow_total": inflow_total,
                    "outflow_total": outflow_total,
                    "first_txn_at": _trim(raw.get("first_txn_at")),
                    "last_txn_at": _trim(raw.get("last_txn_at")),
                    "allowed_wording": "全案分析索引覆盖",
                },
                {
                    "scope_id": "directional_amount_rows",
                    "label": "进出方向金额行",
                    "txn_count": directional_txn_count,
                    "account_count": account_count,
                    "inflow_total": inflow_total,
                    "outflow_total": outflow_total,
                    "allowed_wording": "进出方向收支口径",
                },
                {
                    "scope_id": "by_name_holder_aggregate",
                    "legacy_scope_id": "ui_by_name_tree",
                    "label": "账户维表 byName 户名聚合统计口径",
                    "group_count": _optional_nonnegative_int(groups.get("ui_by_name_group_count")),
                    "holder_label_count": _optional_nonnegative_int(groups.get("holder_label_count")),
                    "account_count": _optional_nonnegative_int(groups.get("tree_account_count")),
                    "unresolved_account_count": _optional_nonnegative_int(groups.get("unresolved_account_count")),
                    "unresolved_txn_count": unresolved_txn_count,
                    "unresolved_turnover_total": unresolved_turnover_total,
                    "allowed_wording": "按 analysis_account_dim 账户维表户名聚合统计；不得用交易明细空户名字段替代账户归属",
                },
                {
                    "scope_id": "report_dedupe_candidate",
                    "label": "疑似补卡/换卡同事实去重候选口径",
                    "txn_count": dedupe_count,
                    "account_count": account_count,
                    "allowed_wording": "报告去重候选，需说明去重规则",
                },
            ]
            warnings: list[dict[str, Any]] = []
            unresolved_account_count = _optional_nonnegative_int(groups.get("unresolved_account_count"))
            if unresolved_account_count is not None and unresolved_account_count > 0:
                warnings.append(
                    {
                        "code": "SCOPE_HAS_UNRESOLVED_HOLDER_ACCOUNTS",
                        "message": f"账户维表 byName 口径存在 {unresolved_account_count} 个未登记户名账户；人物名下全量分析必须以 analysis_account_dim / resolve_owner_scope 为准。",
                        "severity": "warning",
                    }
                )
            if dedupe_count is not None and directional_txn_count is not None and dedupe_count != directional_txn_count:
                warnings.append(
                    {
                        "code": "REPORT_DEDUPE_DIFFERS_FROM_DETAIL_INDEX",
                        "message": "报告去重候选口径与进出方向明细口径不同，最终报告必须显式说明采用哪一套口径。",
                        "severity": "warning",
                    }
                )
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="compare_analysis_scopes",
                params={"case_id": case_id},
                summary={"scopes": scopes, "warning_count": len(warnings)},
                row_count=_optional_nonnegative_int(raw.get("txn_total")),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {"case_id": case_id, "scopes": scopes, "query_id": query_id, "warnings": warnings}
        finally:
            engine.close()

    def fund_analysis_probe(
        self,
        case_id: str,
        *,
        probe_type: str = "discovery",
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        keywords: Optional[Sequence[str]] = None,
        min_amount: float = 0.0,
        limit: int = 20,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_probe = _trim(probe_type).lower() or "discovery"
        typed_source_required = normalized_probe in {"project_litigation_asset", "topic_leads"}
        return {
            "case_id": case_id,
            "probe_type": normalized_probe,
            "capability_status": "unresolved",
            "semantic_status": "unresolved",
            "blocker": "host_evidence_receipt_required",
            "fact_answer_allowed": False,
            "scope": {},
            "filters": {},
            "scope_stats": {},
            "category_summary": [],
            "pattern_summary": {},
            "findings": [],
            "followup_candidates": [],
            "evidence_ids": [],
            "query_id": "",
            "warnings": [
                {
                    "code": (
                        "TYPED_SOURCE_CAPABILITY_REQUIRED"
                        if typed_source_required
                        else "HOST_EVIDENCE_RECEIPT_REQUIRED"
                    ),
                    "message": (
                        "当前没有可验证的类型化来源能力，主题探针保持未解析。"
                        if typed_source_required
                        else "资金探针尚未接入宿主签发的同案、同轮、同快照证据回执，保持未解析。"
                    ),
                    "severity": "warning",
                }
            ],
            "interpretation": "未解析结果不能生成案件场景、关系、用途、金额排名或法律结论。",
        }

    def trace_subject_top_outflows(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        top_n: int = 20,
        min_amount: float = 0.0,
        dedupe_seed_txn_id: bool = True,
        trace_depth: int = 1,
        time_window_hours: float = 72.0,
        amount_tolerance_ratio: float = 0.05,
        amount_tolerance_abs: float = 100.0,
        downstream_limit_per_seed: int = 5,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        normalized_holder = _trim(holder_name)
        normalized_id_no = _trim(id_no)
        normalized_limit = max(1, min(int(top_n or 20), 100))
        normalized_downstream_limit = max(0, min(int(downstream_limit_per_seed or 5), 20))
        normalized_depth = max(1, min(int(trace_depth or 1), 3))
        normalized_window_hours = max(1.0, min(_as_float(time_window_hours), 24.0 * 30))
        tolerance_ratio = max(0.0, min(_as_float(amount_tolerance_ratio), 1.0))
        tolerance_abs = max(0.0, _as_float(amount_tolerance_abs))
        min_amount_value = max(0.0, _as_float(min_amount))
        normalized_candidate_min_turnover = max(0.0, _as_float(candidate_min_turnover))
        selected_accounts = _trim_list(account_keys or [])
        owner_scope: dict[str, Any] = {}
        if normalized_holder or normalized_id_no:
            owner_scope = self.resolve_owner_scope(
                case_id,
                holder_name=normalized_holder,
                id_no=normalized_id_no,
                account_keys=selected_accounts,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=normalized_candidate_min_turnover,
                limit=max(normalized_limit, 50),
            )
            selected_accounts = _trim_list(
                owner_scope.get("expanded_account_keys" if include_candidate_accounts else "direct_account_keys") or []
            )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            if (normalized_holder or normalized_id_no) and not selected_accounts:
                where_sql, params = "1=0", []
            else:
                where_sql, params = self._materialized_filtered_detail_scope(
                    engine,
                    case_id=case_id,
                    account_keys=selected_accounts or None,
                    date_start=date_start,
                    date_end=date_end,
                    direction_mode="out",
                    success_filter="all",
                    cash_filter="all",
                )
            if min_amount_value:
                where_sql = f"({where_sql}) AND amount IS NOT NULL AND ABS(amount) >= ?"
                params.append(min_amount_value)
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=where_sql, params=params)
            category_sql = _fund_business_category_sql()
            seed_rows_raw = self._query_dicts(
                engine,
                f"""
                SELECT
                  txn_id,
                  CAST(txn_ts AS VARCHAR) AS txn_time,
                  acct_key,
                  account_open_name,
                  ABS(amount) AS amount,
                  dc_val,
                  cp_key,
                  cp_name,
                  cp_raw,
                  summary,
                  txn_type,
                  branch_name,
                  cash_flag,
                  remark,
                  file_id,
                  {category_sql} AS business_category
                FROM analysis_txn_detail_idx
                WHERE {where_sql}
                  AND dc_val='出'
                ORDER BY ABS(amount) DESC NULLS LAST, txn_ts ASC, txn_id ASC
                LIMIT ?
                """,
                tuple([*params, normalized_limit * 3 if dedupe_seed_txn_id else normalized_limit]),
            )
            unresolved_seed_amount_count = 0
            normalized_seed_rows_raw: list[dict[str, Any]] = []
            for row in seed_rows_raw:
                seed_amount = _optional_rounded_float(row.get("amount"), 2)
                if seed_amount is None:
                    unresolved_seed_amount_count += 1
                    continue
                normalized_seed_rows_raw.append({**row, "amount": seed_amount})
            duplicate_seed_txn_ids: list[str] = []
            if dedupe_seed_txn_id:
                seen_txn_ids: set[str] = set()
                seed_rows: list[dict[str, Any]] = []
                for row in normalized_seed_rows_raw:
                    txn_id = _trim(row.get("txn_id"))
                    if txn_id and txn_id in seen_txn_ids:
                        if txn_id not in duplicate_seed_txn_ids:
                            duplicate_seed_txn_ids.append(txn_id)
                        continue
                    if txn_id:
                        seen_txn_ids.add(txn_id)
                    seed_rows.append(row)
                    if len(seed_rows) >= normalized_limit:
                        break
            else:
                seed_rows = normalized_seed_rows_raw[:normalized_limit]
            target_accounts = _trim_list(row.get("cp_key") for row in seed_rows)
            target_account_rows: dict[str, dict[str, Any]] = {}
            if target_accounts:
                placeholders = ",".join(["?"] * len(target_accounts))
                rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      acct_key,
                      MAX(COALESCE(account_open_name, '')) AS account_open_name,
                      COUNT(1) AS txn_count,
                      COUNT(amount) AS amount_present_count,
                      COUNT(1) FILTER (WHERE dc_val='进') AS in_txn_count,
                      COUNT(1) FILTER (WHERE dc_val='出') AS out_txn_count,
                      COUNT(amount) FILTER (WHERE dc_val='进') AS in_amount_present_count,
                      COUNT(amount) FILTER (WHERE dc_val='出') AS out_amount_present_count,
                      SUM(ABS(amount)) FILTER (WHERE dc_val='进') AS inflow_total,
                      SUM(ABS(amount)) FILTER (WHERE dc_val='出') AS outflow_total,
                      MAX(ABS(amount)) AS max_single_amount
                    FROM analysis_txn_detail_idx
                    WHERE acct_key IN ({placeholders})
                    GROUP BY acct_key
                    """,
                    tuple(target_accounts),
                )
                target_account_rows = {_trim(row.get("acct_key")): dict(row) for row in rows if _trim(row.get("acct_key"))}

            top_outflows: list[dict[str, Any]] = []
            terminal_summary: dict[str, dict[str, Any]] = {}
            category_summary: dict[str, dict[str, Any]] = {}
            followup_requests: list[dict[str, Any]] = []

            def _bump_summary(bucket: dict[str, dict[str, Any]], key: str, label: str, amount: float) -> None:
                current = bucket.setdefault(key, {"code": key, "label": label, "txn_count": 0, "amount_total": 0.0})
                current["txn_count"] = _as_int(current.get("txn_count")) + 1
                current["amount_total"] = round(_as_float(current.get("amount_total")) + amount, 2)

            def _terminal_category(row: dict[str, Any], target_info: dict[str, Any], downstream: Sequence[dict[str, Any]]) -> tuple[str, str]:
                category = _trim(row.get("business_category"))
                cp_key = _trim(row.get("cp_key"))
                cp_name = _trim(row.get("cp_name")) or _trim(row.get("cp_raw"))
                if downstream:
                    return "matched_downstream_candidate", "案内账户存在后续出账候选"
                if category == "financial_product":
                    return "financial_product_or_institution", "理财/基金/证券/保险类交易，不能误归为现金断点"
                if category == "cash_breakpoint":
                    return "cash_or_counter_breakpoint", "现金、柜面、ATM/POS 或户名缺失断点"
                if category == "third_party_payment":
                    return "third_party_channel_terminal", "三方支付通道需补调平台明细"
                if not cp_key and not cp_name:
                    return "missing_counterparty", "对手账号和户名缺失，需补调回单或原始流水字段"
                if target_info:
                    return "target_account_no_matched_downstream", "对手账号在案内存在，但本工具窗口内未匹配到后续出账"
                return "external_or_unavailable", "对手账号未在本案账户集合中出现，需补调下游账户流水"

            for row in seed_rows:
                seed_amount = _optional_rounded_float(row.get("amount"), 2)
                if seed_amount is None:
                    unresolved_seed_amount_count += 1
                    continue
                business_category = _trim(row.get("business_category")) or "transfer_unclassified"
                seed_time = _coerce_datetime(row.get("txn_time"))
                cp_key = _trim(row.get("cp_key"))
                target_info = target_account_rows.get(cp_key, {}) if cp_key else {}
                downstream_rows: list[dict[str, Any]] = []
                if cp_key and normalized_downstream_limit > 0 and seed_time is not None:
                    tolerance = max(tolerance_abs, seed_amount * tolerance_ratio)
                    lower_amount = max(0.0, seed_amount - tolerance)
                    upper_amount = seed_amount + tolerance
                    downstream_raw = self._query_dicts(
                        engine,
                        f"""
                        SELECT
                          txn_id,
                          CAST(txn_ts AS VARCHAR) AS txn_time,
                          acct_key,
                          account_open_name,
                          ABS(amount) AS amount,
                          dc_val,
                          cp_key,
                          cp_name,
                          cp_raw,
                          summary,
                          txn_type,
                          branch_name,
                          cash_flag,
                          remark,
                          file_id,
                          {category_sql} AS business_category
                        FROM analysis_txn_detail_idx
                        WHERE acct_key=?
                          AND dc_val='出'
                          AND txn_ts >= ?
                          AND txn_ts <= ?
                          AND amount IS NOT NULL
                          AND ABS(amount) BETWEEN ? AND ?
                        ORDER BY txn_ts ASC, ABS(amount) DESC, txn_id ASC
                        LIMIT ?
                        """,
                        (
                            cp_key,
                            seed_time,
                            seed_time + timedelta(hours=normalized_window_hours),
                            lower_amount,
                            upper_amount,
                            normalized_downstream_limit,
                        ),
                    )
                    downstream_rows = []
                    for item in downstream_raw:
                        downstream_amount = _optional_rounded_float(item.get("amount"), 2)
                        if downstream_amount is None:
                            unresolved_seed_amount_count += 1
                            continue
                        downstream_rows.append(
                            {
                                "txn_id": _trim(item.get("txn_id")),
                                "txn_time": _trim(item.get("txn_time")),
                                "account_key": _trim(item.get("acct_key")),
                                "account_open_name": _trim(item.get("account_open_name")),
                                "amount": downstream_amount,
                                "counterparty_key": _trim(item.get("cp_key")),
                                "counterparty_name": _trim(item.get("cp_name")) or _trim(item.get("cp_raw")),
                                "summary": _trim(item.get("summary")),
                                "txn_type": _trim(item.get("txn_type")),
                                "business_category": _trim(item.get("business_category")) or "transfer_unclassified",
                                "business_category_label": _fund_business_category_label(item.get("business_category")),
                                "file_id": _trim(item.get("file_id")),
                            }
                        )
                terminal_code, terminal_label = _terminal_category(row, target_info, downstream_rows)
                _bump_summary(terminal_summary, terminal_code, terminal_label, seed_amount)
                _bump_summary(category_summary, business_category, _fund_business_category_label(business_category), seed_amount)
                if terminal_code != "matched_downstream_candidate":
                    followup_requests.append(
                        {
                            "priority": "high" if seed_amount >= 1000000 else "medium" if seed_amount >= 100000 else "normal",
                            "target_account": cp_key,
                            "target_name": _trim(row.get("cp_name")) or _trim(row.get("cp_raw")),
                            "seed_txn_id": _trim(row.get("txn_id")),
                            "amount": seed_amount,
                            "terminal_category": terminal_code,
                            "reason": terminal_label,
                        }
                    )
                top_outflows.append(
                    {
                        "rank": len(top_outflows) + 1,
                        "seed_txn": {
                            "txn_id": _trim(row.get("txn_id")),
                            "txn_time": _trim(row.get("txn_time")),
                            "account_key": _trim(row.get("acct_key")),
                            "account_open_name": _trim(row.get("account_open_name")),
                            "amount": seed_amount,
                            "counterparty_key": cp_key,
                            "counterparty_name": _trim(row.get("cp_name")) or _trim(row.get("cp_raw")),
                            "summary": _trim(row.get("summary")),
                            "txn_type": _trim(row.get("txn_type")),
                            "business_category": business_category,
                            "business_category_label": _fund_business_category_label(business_category),
                            "file_id": _trim(row.get("file_id")),
                        },
                        "target_account_in_case": bool(target_info),
                        "target_account_summary": (
                            {
                                "account_key": cp_key,
                                "account_open_name": _trim(target_info.get("account_open_name")),
                                "txn_count": target_amount_facts["txn_count"],
                                "inflow_total": target_amount_facts["inflow_total"],
                                "outflow_total": target_amount_facts["outflow_total"],
                            }
                            if target_info
                            and (target_amount_facts := _complete_rank_amount_facts(target_info))
                            else {}
                        ),
                        "terminal_category": terminal_code,
                        "terminal_category_label": terminal_label,
                        "downstream_candidates": downstream_rows,
                        "downstream_truncated": len(downstream_rows) >= normalized_downstream_limit > 0,
                    }
                )
            warnings: list[dict[str, Any]] = []
            if owner_scope.get("candidate_accounts"):
                warnings.append(
                    {
                        "code": "TRACE_USED_OWNER_CANDIDATE_OPTION",
                        "message": "本次续查可包含候选归属账户；报告中必须区分直接归属与候选归属。",
                        "severity": "warning",
                    }
                )
            if duplicate_seed_txn_ids:
                warnings.append(
                    {
                        "code": "TRACE_TOP_OUTFLOW_DUPLICATE_TXN_IDS_DEDUPED",
                        "message": "Top 出账中发现重复 txn_id，已默认按 txn_id 去重；如需核查补卡/换卡重复，请结合 compare_analysis_scopes 的报告去重候选口径。",
                        "severity": "warning",
                        "txn_ids": duplicate_seed_txn_ids[:20],
                    }
                )
            if unresolved_seed_amount_count:
                warnings.append(
                    {
                        "code": "TRACE_AMOUNT_COVERAGE_PARTIAL",
                        "message": f"有 {unresolved_seed_amount_count} 条种子或下游候选金额未核验，已排除且未按 0 参与排序、汇总或追踪。",
                        "severity": "warning",
                    }
                )
            if normalized_depth > 1:
                warnings.append(
                    {
                        "code": "TRACE_DEPTH_BOUNDED_NEXT_HOP",
                        "message": "本工具当前返回 Top 出账的一跳案内后续候选；更深链路应对具体 seed_txn_id 继续调用 trace_fund。",
                        "severity": "info",
                    }
                )
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="trace_subject_top_outflows",
                params={
                    "case_id": case_id,
                    "holder_name": normalized_holder,
                    "id_no_provided": bool(normalized_id_no),
                    "account_count": len(selected_accounts),
                    "include_candidate_accounts": include_candidate_accounts,
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "top_n": normalized_limit,
                    "min_amount": min_amount_value,
                    "dedupe_seed_txn_id": dedupe_seed_txn_id,
                    "time_window_hours": normalized_window_hours,
                    "amount_tolerance_ratio": tolerance_ratio,
                    "amount_tolerance_abs": tolerance_abs,
                },
                summary={"scope_stats": scope_stats, "top_outflow_count": len(top_outflows), "terminal_summary": list(terminal_summary.values())},
                row_count=len(top_outflows),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "scope": {
                    "holder_name": normalized_holder,
                    "id_no_provided": bool(normalized_id_no),
                    "account_keys": selected_accounts,
                    "include_candidate_accounts": include_candidate_accounts,
                    "owner_scope": owner_scope,
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "top_n": normalized_limit,
                    "min_amount": min_amount_value,
                    "dedupe_seed_txn_id": dedupe_seed_txn_id,
                    "trace_depth": normalized_depth,
                    "time_window_hours": normalized_window_hours,
                    "amount_tolerance_ratio": tolerance_ratio,
                    "amount_tolerance_abs": tolerance_abs,
                    "downstream_limit_per_seed": normalized_downstream_limit,
                },
                "scope_stats": scope_stats,
                "top_outflows": top_outflows,
                "terminal_summary": sorted(terminal_summary.values(), key=lambda item: (-_as_float(item.get("amount_total")), str(item.get("code") or ""))),
                "business_category_summary": sorted(category_summary.values(), key=lambda item: (-_as_float(item.get("amount_total")), str(item.get("code") or ""))),
                "followup_requests": followup_requests[:50],
                "duplicate_seed_txn_ids": duplicate_seed_txn_ids,
                "query_id": query_id,
                "warnings": warnings,
                "interpretation": "downstream_candidates 是案内同账号、时间窗和金额容差下的一跳候选，不等于最终资金归属认定；未匹配终点应形成补调清单。",
            }
        finally:
            engine.close()

    def classify_missing_counterparty_business(
        self,
        case_id: str,
        *,
        holder_name: str = "",
        id_no: str = "",
        account_keys: Optional[Sequence[str]] = None,
        include_candidate_accounts: bool = False,
        candidate_min_turnover: float = 100000.0,
        date_start: str = "",
        date_end: str = "",
        missing_kind: str = "both",
        min_amount: float = 0.0,
        limit: int = 50,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        normalized_holder = _trim(holder_name)
        normalized_id_no = _trim(id_no)
        normalized_kind = _trim(missing_kind).lower() or "both"
        if normalized_kind not in {"holder", "counterparty", "both"}:
            normalized_kind = "both"
        normalized_limit = max(1, min(int(limit or 50), 200))
        min_amount_value = max(0.0, _as_float(min_amount))
        normalized_candidate_min_turnover = max(0.0, _as_float(candidate_min_turnover))
        selected_accounts = _trim_list(account_keys or [])
        owner_scope: dict[str, Any] = {}
        if normalized_holder or normalized_id_no:
            owner_scope = self.resolve_owner_scope(
                case_id,
                holder_name=normalized_holder,
                id_no=normalized_id_no,
                account_keys=selected_accounts,
                include_candidate_accounts=include_candidate_accounts,
                candidate_min_turnover=normalized_candidate_min_turnover,
                limit=max(normalized_limit, 50),
            )
            selected_accounts = _trim_list(
                owner_scope.get("expanded_account_keys" if include_candidate_accounts else "direct_account_keys") or []
            )
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            if (normalized_holder or normalized_id_no) and not selected_accounts:
                where_sql, params = "1=0", []
            else:
                where_sql, params = self._materialized_filtered_detail_scope(
                    engine,
                    case_id=case_id,
                    account_keys=selected_accounts or None,
                    date_start=date_start,
                    date_end=date_end,
                    direction_mode="both",
                    success_filter="all",
                    cash_filter="all",
                )
            holder_missing_sql = "(account_open_name IS NULL OR TRIM(account_open_name)='' OR account_open_name='未识别户名')"
            counterparty_missing_sql = "(" \
                "(cp_key IS NULL OR TRIM(cp_key)='') " \
                "AND (cp_name IS NULL OR TRIM(cp_name)='') " \
                "AND (cp_raw IS NULL OR TRIM(cp_raw)='')" \
                ")"
            if normalized_kind == "holder":
                missing_condition = holder_missing_sql
            elif normalized_kind == "counterparty":
                missing_condition = counterparty_missing_sql
            else:
                missing_condition = f"({holder_missing_sql}) OR ({counterparty_missing_sql})"
            if min_amount_value:
                where_sql = f"({where_sql}) AND amount IS NOT NULL AND ABS(amount) >= ?"
                params.append(min_amount_value)
            category_sql = _fund_business_category_sql()
            missing_kind_sql = (
                "CASE "
                f"WHEN ({holder_missing_sql}) AND ({counterparty_missing_sql}) THEN 'holder_and_counterparty' "
                f"WHEN ({holder_missing_sql}) THEN 'holder' "
                f"WHEN ({counterparty_missing_sql}) THEN 'counterparty' "
                "ELSE 'none' END"
            )
            scope_stats = self._scope_stats_from_materialized(engine, where_sql=f"({where_sql}) AND ({missing_condition})", params=params)
            summary_rows = self._query_dicts(
                engine,
                f"""
                WITH flagged AS (
                  SELECT
                    {missing_kind_sql} AS detected_missing_kind,
                    {category_sql} AS business_category,
                    dc_val,
                    amount,
                    acct_key,
                    cp_key,
                    cp_name,
                    cp_raw,
                    summary,
                    remark,
                    txn_type,
                    cash_flag
                  FROM analysis_txn_detail_idx
                  WHERE {where_sql}
                    AND ({missing_condition})
                )
                SELECT
                  detected_missing_kind,
                  business_category,
                  dc_val AS direction,
                  COUNT(1) AS txn_count,
                  COUNT(DISTINCT acct_key) AS account_count,
                  COUNT(DISTINCT COALESCE(NULLIF(cp_key, ''), NULLIF(cp_name, ''), NULLIF(cp_raw, ''))) AS counterparty_count,
                  COUNT(amount) AS amount_present_count,
                  SUM(ABS(amount)) AS amount_total,
                  MAX(ABS(amount)) AS max_single_amount
                FROM flagged
                GROUP BY 1,2,3
                ORDER BY amount_total DESC, txn_count DESC, detected_missing_kind ASC, business_category ASC
                """,
                tuple(params),
            )
            category_summary = []
            for row in summary_rows:
                txn_count = _optional_nonnegative_int(row.get("txn_count"))
                amount_total = _complete_aggregate_amount(
                    row.get("amount_total"),
                    expected_count=txn_count,
                    present_count=row.get("amount_present_count"),
                )
                max_single_amount = (
                    _optional_rounded_float(row.get("max_single_amount"), 2)
                    if amount_total is not None
                    else None
                )
                category_summary.append(
                    {
                    "missing_kind": _trim(row.get("detected_missing_kind")),
                    "business_category": _trim(row.get("business_category")) or "transfer_unclassified",
                    "business_category_label": _fund_business_category_label(row.get("business_category")),
                    "direction": "in" if _trim(row.get("direction")) == "进" else ("out" if _trim(row.get("direction")) == "出" else "unknown"),
                    "txn_count": txn_count,
                    "account_count": _optional_nonnegative_int(row.get("account_count")),
                    "counterparty_count": _optional_nonnegative_int(row.get("counterparty_count")),
                    "amount_total": amount_total,
                    "max_single_amount": max_single_amount,
                    }
                )
            top_rows_raw = self._query_dicts(
                engine,
                f"""
                SELECT
                  txn_id,
                  CAST(txn_ts AS VARCHAR) AS txn_time,
                  acct_key,
                  account_open_name,
                  ABS(amount) AS amount,
                  dc_val,
                  cp_key,
                  cp_name,
                  cp_raw,
                  summary,
                  txn_type,
                  branch_name,
                  cash_flag,
                  remark,
                  file_id,
                  {missing_kind_sql} AS detected_missing_kind,
                  {category_sql} AS business_category
                FROM analysis_txn_detail_idx
                WHERE {where_sql}
                  AND ({missing_condition})
                ORDER BY ABS(amount) DESC NULLS LAST, txn_ts ASC, txn_id ASC
                LIMIT ?
                """,
                tuple([*params, normalized_limit]),
            )
            top_rows = [
                {
                    "txn_id": _trim(row.get("txn_id")),
                    "txn_time": _trim(row.get("txn_time")),
                    "account_key": _trim(row.get("acct_key")),
                    "account_open_name": _trim(row.get("account_open_name")),
                    "amount": _optional_rounded_float(row.get("amount"), 2),
                    "direction": "in" if _trim(row.get("dc_val")) == "进" else ("out" if _trim(row.get("dc_val")) == "出" else "unknown"),
                    "counterparty_key": _trim(row.get("cp_key")),
                    "counterparty_name": _trim(row.get("cp_name")) or _trim(row.get("cp_raw")),
                    "summary": _trim(row.get("summary")),
                    "txn_type": _trim(row.get("txn_type")),
                    "cash_flag": _trim(row.get("cash_flag")),
                    "missing_kind": _trim(row.get("detected_missing_kind")),
                    "business_category": _trim(row.get("business_category")) or "transfer_unclassified",
                    "business_category_label": _fund_business_category_label(row.get("business_category")),
                    "file_id": _trim(row.get("file_id")),
                }
                for row in top_rows_raw
            ]
            correction_rules = [
                {
                    "rule_id": "financial_product_over_cash",
                    "message": "摘要、备注、对手方出现基金/理财/证券/保险/申购/赎回时优先归入理财金融产品，不得仅因对手户名缺失或现金字段异常归为现金。",
                },
                {
                    "rule_id": "missing_holder_is_scope_risk",
                    "message": "账户户名缺失只说明归属字段缺失，不能自动推断为目标人员名下账户。",
                },
                {
                    "rule_id": "missing_counterparty_requires_followup",
                    "message": "对手账号户名同时缺失时，结论只能写成断点或补证对象。",
                },
            ]
            warnings: list[dict[str, Any]] = []
            if owner_scope.get("candidate_accounts"):
                warnings.append(
                    {
                        "code": "CLASSIFY_USED_OWNER_CANDIDATE_OPTION",
                        "message": "本次分类可包含候选归属账户；报告中必须区分直接归属与候选归属。",
                        "severity": "warning",
                    }
                )
            unresolved_amount_count = sum(
                1 for item in category_summary if item.get("amount_total") is None
            ) + sum(1 for item in top_rows if item.get("amount") is None)
            if unresolved_amount_count:
                warnings.append(
                    {
                        "code": "CLASSIFY_AMOUNT_COVERAGE_PARTIAL",
                        "message": f"有 {unresolved_amount_count} 项分类聚合或明细金额未核验，已保留为空且未按 0 处理。",
                        "severity": "warning",
                    }
                )
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="classify_missing_counterparty_business",
                params={
                    "case_id": case_id,
                    "holder_name": normalized_holder,
                    "id_no_provided": bool(normalized_id_no),
                    "account_count": len(selected_accounts),
                    "include_candidate_accounts": include_candidate_accounts,
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "missing_kind": normalized_kind,
                    "min_amount": min_amount_value,
                    "limit": normalized_limit,
                },
                summary={"scope_stats": scope_stats, "category_summary": category_summary[:12]},
                row_count=_as_int(scope_stats.get("txn_count")),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "scope": {
                    "holder_name": normalized_holder,
                    "id_no_provided": bool(normalized_id_no),
                    "account_keys": selected_accounts,
                    "include_candidate_accounts": include_candidate_accounts,
                    "owner_scope": owner_scope,
                },
                "filters": {
                    "date_start": _trim(date_start),
                    "date_end": _trim(date_end),
                    "missing_kind": normalized_kind,
                    "min_amount": min_amount_value,
                    "limit": normalized_limit,
                },
                "scope_stats": scope_stats,
                "category_summary": category_summary,
                "top_rows": top_rows,
                "correction_rules": correction_rules,
                "query_id": query_id,
                "warnings": warnings,
                "interpretation": "分类是字段缺失条件下的业务标签复核，用于避免把理财/基金误归为现金，也不能把缺失户名自动认定为主体归属。",
            }
        finally:
            engine.close()

    def validate_continuation_list(
        self,
        case_id: str,
        *,
        rows: Optional[Sequence[Mapping[str, Any]]] = None,
        required_fields: Optional[Sequence[str]] = None,
        amount_unit: str = "yuan",
        expected_total_amount: float = 0.0,
        expected_txn_count: int = 0,
        amount_tolerance: float = 0.01,
        strict_db_match: bool = True,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        input_rows = [dict(row or {}) for row in list(rows or []) if isinstance(row, Mapping)]
        normalized_required = _trim_list(required_fields or ["txn_id", "account_key", "txn_time", "amount", "direction"])
        normalized_unit = _trim(amount_unit).lower() or "yuan"
        unit_multiplier = 10000.0 if normalized_unit in {"wan", "万元", "w"} else 1.0
        parsed_expected_total = _optional_finite_float(expected_total_amount)
        expected_total = (
            round(parsed_expected_total * unit_multiplier, 2)
            if parsed_expected_total is not None and parsed_expected_total > 0
            else None
        )
        parsed_expected_count = _optional_nonnegative_int(expected_txn_count)
        expected_count = parsed_expected_count if parsed_expected_count is not None and parsed_expected_count > 0 else None
        parsed_tolerance = _optional_finite_float(amount_tolerance)
        if parsed_tolerance is None or parsed_tolerance < 0:
            raise ValueError("continuation_amount_tolerance_invalid")
        tolerance = parsed_tolerance

        def _row_txn_id(row: Mapping[str, Any]) -> str:
            return _trim(row.get("txn_id") or row.get("transaction_id") or row.get("交易ID") or row.get("流水号"))

        def _row_account_key(row: Mapping[str, Any]) -> str:
            return _trim(row.get("account_key") or row.get("acct_key") or row.get("account") or row.get("卡号") or row.get("账户"))

        def _has_value(row: Mapping[str, Any], key: str) -> bool:
            value = row.get(key)
            return value is not None and not (isinstance(value, str) and not value.strip())

        def _row_amount(row: Mapping[str, Any]) -> float | None:
            value = (
                row.get("amount_yuan")
                if _has_value(row, "amount_yuan")
                else row.get("amount")
                if _has_value(row, "amount")
                else row.get("amount_val")
                if _has_value(row, "amount_val")
                else row.get("金额")
            )
            amount = _optional_finite_float(value)
            if amount is None:
                return None
            multiplier = 1.0 if _has_value(row, "amount_yuan") else unit_multiplier
            return round(amount * multiplier, 2)

        def _row_field_value(row: Mapping[str, Any], field: str) -> str:
            if field == "txn_id":
                return _row_txn_id(row)
            if field == "account_key":
                return _row_account_key(row)
            if field == "amount":
                amount = _row_amount(row)
                return str(amount) if amount is not None else ""
            if field == "direction":
                return _trim(row.get("direction") or row.get("dc_val") or row.get("收付标志"))
            if field == "txn_time":
                return _trim(row.get("txn_time") or row.get("txn_ts") or row.get("交易时间"))
            return _trim(row.get(field))

        issues: list[dict[str, Any]] = []
        txn_ids = [_row_txn_id(row) for row in input_rows]
        duplicate_txn_ids = sorted({txn_id for txn_id in txn_ids if txn_id and txn_ids.count(txn_id) > 1})
        for duplicate in duplicate_txn_ids:
            issues.append(
                {
                    "code": "DUPLICATE_TXN_ID",
                    "severity": "error",
                    "txn_id": duplicate,
                    "message": f"续调清单中交易 {duplicate} 重复出现。",
                }
            )
        blank_field_counts: dict[str, int] = defaultdict(int)
        for index, row in enumerate(input_rows, start=1):
            for field in normalized_required:
                if not _row_field_value(row, field):
                    blank_field_counts[field] += 1
                    issues.append(
                        {
                            "code": "BLANK_REQUIRED_FIELD",
                            "severity": "error",
                            "row_index": index,
                            "field": field,
                            "txn_id": _row_txn_id(row),
                            "message": f"第 {index} 行缺少必填字段 {field}。",
                        }
                    )
        amount_total = _complete_amount_sum(_row_amount(row) for row in input_rows)
        if expected_count is not None and expected_count != len(input_rows):
            issues.append(
                {
                    "code": "EXPECTED_TXN_COUNT_MISMATCH",
                    "severity": "error",
                    "expected": expected_count,
                    "actual": len(input_rows),
                    "message": "续调清单行数与期望笔数不一致。",
                }
            )
        if expected_total is not None and amount_total is None:
            issues.append(
                {
                    "code": "EXPECTED_AMOUNT_UNRESOLVED",
                    "severity": "error",
                    "expected": expected_total,
                    "actual": None,
                    "message": "续调清单存在未核验金额，不能与期望金额执行闭合校验。",
                }
            )
        elif expected_total is not None and amount_total is not None and abs(amount_total - expected_total) > max(tolerance, expected_total * 0.000001):
            issues.append(
                {
                    "code": "EXPECTED_AMOUNT_MISMATCH",
                    "severity": "error",
                    "expected": round(expected_total, 2),
                    "actual": amount_total,
                    "difference": round(amount_total - expected_total, 2),
                    "message": "续调清单金额合计与期望金额不一致，需检查元/万元单位和漏列重复。",
                }
            )
        matched_txn_ids: list[str] = []
        missing_txn_ids: list[str] = []
        ambiguous_txn_ids: list[str] = []
        db_rows_by_txn: dict[str, list[dict[str, Any]]] = {}
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            unique_txn_ids = _unique_texts(txn_ids)
            if strict_db_match and unique_txn_ids:
                placeholders = ",".join(["?"] * len(unique_txn_ids))
                db_rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT
                      txn_id,
                      CAST(txn_ts AS VARCHAR) AS txn_time,
                      acct_key,
                      account_open_name,
                      ABS(amount) AS amount,
                      dc_val,
                      cp_key,
                      cp_name,
                      cp_raw,
                      summary,
                      txn_type,
                      file_id
                    FROM analysis_txn_detail_idx
                    WHERE txn_id IN ({placeholders})
                    """,
                    tuple(unique_txn_ids),
                )
                for db_row in db_rows:
                    db_txn_id = _trim(db_row.get("txn_id"))
                    if not db_txn_id:
                        continue
                    db_rows_by_txn.setdefault(db_txn_id, []).append(dict(db_row))
                for txn_id in unique_txn_ids:
                    if txn_id in db_rows_by_txn:
                        matched_txn_ids.append(txn_id)
                        account_candidates = _unique_texts([row.get("acct_key") for row in db_rows_by_txn.get(txn_id, [])])
                        if len(account_candidates) > 1:
                            ambiguous_txn_ids.append(txn_id)
                    else:
                        missing_txn_ids.append(txn_id)
                        issues.append(
                            {
                                "code": "TXN_ID_NOT_FOUND_IN_CASE",
                                "severity": "error",
                                "txn_id": txn_id,
                                "message": "续调清单交易 ID 未在当前案件明细索引中找到。",
                            }
                        )
                for index, row in enumerate(input_rows, start=1):
                    txn_id = _row_txn_id(row)
                    db_candidates = db_rows_by_txn.get(txn_id) or []
                    if not db_candidates:
                        continue
                    row_amount = _row_amount(row)
                    parsed_db_amounts = [_optional_rounded_float(candidate.get("amount"), 2) for candidate in db_candidates]
                    db_amounts = sorted({amount for amount in parsed_db_amounts if amount is not None})
                    comparison_ready = (
                        row_amount is not None
                        and bool(db_amounts)
                        and all(amount is not None for amount in parsed_db_amounts)
                    )
                    if not comparison_ready:
                        issues.append(
                            {
                                "code": "TXN_AMOUNT_UNRESOLVED",
                                "severity": "error",
                                "row_index": index,
                                "txn_id": txn_id,
                                "expected_from_db": db_amounts[0] if len(db_amounts) == 1 else db_amounts,
                                "actual": row_amount,
                                "message": "续调清单或案件明细金额未核验，不能按 0 执行一致性比较。",
                            }
                        )
                        amount_matched = False
                    else:
                        amount_matched = any(
                            abs(row_amount - db_amount) <= max(tolerance, db_amount * 0.000001)
                            for db_amount in db_amounts
                        )
                    if comparison_ready and not amount_matched:
                        issues.append(
                            {
                                "code": "TXN_AMOUNT_MISMATCH",
                                "severity": "error",
                                "row_index": index,
                                "txn_id": txn_id,
                                "expected_from_db": db_amounts[0] if len(db_amounts) == 1 else db_amounts,
                                "actual": row_amount,
                                "message": "续调清单金额与案件明细索引不一致。",
                            }
                        )
                    row_direction = _normalize_direction(row.get("direction") or row.get("dc_val") or row.get("收付标志"))
                    db_directions = sorted(
                        {
                            "in"
                            if _trim(candidate.get("dc_val")) == "进"
                            else ("out" if _trim(candidate.get("dc_val")) == "出" else "unknown")
                            for candidate in db_candidates
                        }
                    )
                    if row_direction != "unknown" and row_direction not in db_directions:
                        issues.append(
                            {
                                "code": "TXN_DIRECTION_MISMATCH",
                                "severity": "error",
                                "row_index": index,
                                "txn_id": txn_id,
                                "expected_from_db": db_directions[0] if len(db_directions) == 1 else db_directions,
                                "actual": row_direction,
                                "message": "续调清单收付方向与案件明细索引不一致。",
                            }
                        )
                    row_account = _row_account_key(row)
                    db_accounts = _unique_texts([candidate.get("acct_key") for candidate in db_candidates])
                    if len(db_accounts) > 1 and (not row_account or row_account in db_accounts):
                        issues.append(
                            {
                                "code": "TXN_ID_MULTIPLE_ACCOUNT_CANDIDATES",
                                "severity": "warning",
                                "row_index": index,
                                "txn_id": txn_id,
                                "account_candidates": db_accounts,
                                "actual": row_account,
                                "message": "同一交易 ID 在案件明细中对应多个账户，可能是换卡/补卡/系统重复记录；当前账户可匹配候选，但最终附件应标注复核口径。",
                            }
                        )
                    if row_account and row_account not in db_accounts:
                        issues.append(
                            {
                                "code": "TXN_ACCOUNT_MISMATCH",
                                "severity": "error",
                                "row_index": index,
                                "txn_id": txn_id,
                                "expected_from_db": db_accounts[0] if len(db_accounts) == 1 else db_accounts,
                                "actual": row_account,
                                "message": "续调清单账户与案件明细索引不一致。",
                            }
                        )
            if not input_rows:
                issues.append(
                    {
                        "code": "CONTINUATION_LIST_EMPTY",
                        "severity": "warning",
                        "message": "未提供续调清单行，本工具仅返回校验规则模板。",
                    }
                )
            validation_status = "failed" if any(item.get("severity") == "error" for item in issues) else ("warning" if issues else "passed")
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="validate_continuation_list",
                params={
                    "case_id": case_id,
                    "row_count": len(input_rows),
                    "required_fields": normalized_required,
                    "amount_unit": normalized_unit,
                    "expected_total_amount": expected_total,
                    "expected_txn_count": expected_count,
                    "strict_db_match": strict_db_match,
                },
                summary={
                    "validation_status": validation_status,
                    "issue_count": len(issues),
                    "amount_total": amount_total,
                    "matched_txn_count": len(matched_txn_ids),
                    "missing_txn_count": len(missing_txn_ids),
                    "ambiguous_txn_count": len(ambiguous_txn_ids),
                },
                row_count=len(input_rows),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "case_id": case_id,
                "validation_status": validation_status,
                "row_count": len(input_rows),
                "unique_txn_count": len(_unique_texts(txn_ids)),
                "amount_total_yuan": amount_total,
                "expected_total_yuan": expected_total,
                "duplicate_txn_ids": duplicate_txn_ids,
                "ambiguous_txn_ids": ambiguous_txn_ids,
                "blank_field_counts": dict(blank_field_counts),
                "matched_txn_ids": matched_txn_ids,
                "missing_txn_ids": missing_txn_ids,
                "issues": issues,
                "query_id": query_id,
                "checklist": [
                    "交易 ID、账户、时间、金额、方向不得为空。",
                    "金额单位必须明确，万元清单需按 10000 换算后闭合。",
                    "同一交易不得重复列入续调清单。",
                    "清单金额、方向和账户应与案件明细索引一致。",
                    "同一交易 ID 如对应多个账户，应按换卡/补卡/系统重复记录候选标注复核口径。",
                    "对手户名缺失、三方支付、理财产品等终点必须标注补证边界。",
                ],
            }
        finally:
            engine.close()

    def build_account_counterparty_rankings(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        success_filter: str = "all",
        cash_filter: str = "all",
        limit: int = 10,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        return project_account_fact_boundary(case_id, "account_counterparty_rankings")

    def build_account_behavior_profile(
        self,
        case_id: str,
        *,
        account_keys: Optional[Sequence[str]] = None,
        source_file_ids: Optional[Sequence[str]] = None,
        temp_scope_id: str = "",
        scope_source: str = "db",
        date_start: str = "",
        date_end: str = "",
        success_filter: str = "all",
        cash_filter: str = "all",
        large_amount_threshold: float = 20000.0,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        return project_account_fact_boundary(case_id, "account_behavior_profile")

    def get_materialized_next_hop_candidates(
        self,
        case_id: str,
        *,
        seed_txn_id: str = "",
        seed_account_id: str = "",
        file_ids: Optional[Sequence[str]] = None,
        time_window_sec: int = 7200,
        tolerance_rate: float = 0.03,
        max_candidates: int = 12,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        effective_tolerance = max(0.0, _as_float(tolerance_rate))
        self.sync_case_baseline(case_id)
        engine = self.open_case_engine(case_id)
        owns_engine = True
        try:
            self.ensure_analysis_schema(engine)
            self._daily_agg.ensure_materialized(case_id, engine=engine)
            if not self._table_exists(engine, "analysis_txn_detail_idx"):
                return self._next_hop_unresolved("materialized_source_unavailable")

            normalized_file_ids = _trim_list(file_ids or [])
            effective_window_sec = max(300, int(time_window_sec or 7200))
            candidate_limit = max(3, min(int(max_candidates or 12), 24))
            seed_context: dict[str, Any] = {}
            time_filter_sql = ""
            file_filter_sql = ""
            if normalized_file_ids:
                file_filter_sql = f" AND COALESCE(NULLIF(file_id, ''), '__none__') IN ({','.join(['?'] * len(normalized_file_ids))})"

            if _trim(seed_txn_id):
                seed_rows = self._query_dicts(
                    engine,
                    f"""
                    SELECT txn_id, acct_key, cp_key, cp_name, dc_val, txn_ts, ABS(amount) AS amount,
                           account_open_name, file_id
                    FROM analysis_txn_detail_idx
                    WHERE txn_id=?
                      {file_filter_sql}
                    ORDER BY txn_ts ASC
                    LIMIT 1
                    """,
                    (_trim(seed_txn_id), *normalized_file_ids) if normalized_file_ids else (_trim(seed_txn_id),),
                )
                if not seed_rows:
                    return self._next_hop_unresolved(
                        "seed_transaction_unresolved",
                        seed={"seed_txn_id": _trim(seed_txn_id)},
                    )
                seed_context = dict(seed_rows[0] or {})
                seed_direction = _trim(seed_context.get("dc_val"))
                target_account = _trim(seed_context.get("acct_key"))
                if seed_direction == "出" and _trim(seed_context.get("cp_key")):
                    target_account = _trim(seed_context.get("cp_key"))
                if not target_account:
                    target_account = _trim(seed_context.get("acct_key"))
                seed_context["target_account"] = target_account
                seed_context["candidate_direction"] = "出"
                if seed_context.get("txn_ts"):
                    time_filter_sql = " AND txn_ts >= ? AND txn_ts <= ?"
            else:
                target_account = _trim(seed_account_id)
                if not target_account:
                    return self._next_hop_unresolved("seed_account_unresolved")
                seed_context = {
                    "seed_account_id": target_account,
                    "target_account": target_account,
                    "candidate_direction": "出",
                    "amount": None,
                    "txn_ts": None,
                }

            candidate_rows = self._query_dicts(
                engine,
                f"""
                SELECT txn_id, CAST(txn_ts AS VARCHAR) AS txn_time, txn_ts, acct_key, cp_key, cp_name, dc_val,
                       ABS(amount) AS amount, summary, branch_name, remark, file_id
                FROM analysis_txn_detail_idx
                WHERE acct_key=?
                  AND dc_val=?
                  {file_filter_sql}
                  {time_filter_sql}
                  {" AND txn_id <> ?" if _trim(seed_txn_id) else ""}
                ORDER BY txn_ts ASC, (amount IS NULL) ASC, ABS(amount) DESC, txn_id ASC
                LIMIT ?
                """,
                tuple(
                    [
                        _trim(seed_context.get("target_account")),
                        _trim(seed_context.get("candidate_direction")) or "出",
                        *normalized_file_ids,
                        *([seed_context.get("txn_ts"), seed_context.get("txn_ts") + timedelta(seconds=effective_window_sec)] if seed_context.get("txn_ts") else []),
                        *([_trim(seed_txn_id)] if _trim(seed_txn_id) else []),
                        candidate_limit * 3,
                    ]
                ),
            )

            target_amount = _optional_finite_float(seed_context.get("amount"))
            if target_amount is None or target_amount <= 0:
                return self._next_hop_unresolved(
                    "seed_amount_unresolved",
                    seed={
                        "seed_txn_id": _trim(seed_txn_id),
                        "seed_account_id": _trim(seed_account_id),
                        "target_account": _trim(seed_context.get("target_account")),
                    },
                )
            candidate_amounts = [_optional_finite_float(row.get("amount")) for row in candidate_rows]
            if any(amount is None for amount in candidate_amounts):
                return self._next_hop_unresolved(
                    "candidate_amount_unresolved",
                    seed={
                        "seed_txn_id": _trim(seed_txn_id),
                        "seed_account_id": _trim(seed_account_id),
                        "target_account": _trim(seed_context.get("target_account")),
                        "target_amount": round(target_amount, 2),
                    },
                )
            enriched_rows: list[dict[str, Any]] = []
            for row in candidate_rows:
                amount_value = _optional_finite_float(row.get("amount"))
                if amount_value is None:
                    return self._next_hop_unresolved("candidate_amount_unresolved")
                amount = round(amount_value, 2)
                gap_minutes = 0
                if seed_context.get("txn_ts") and row.get("txn_ts"):
                    gap_minutes = max(
                        0,
                        int(round((row.get("txn_ts") - seed_context.get("txn_ts")).total_seconds() / 60)),
                    )
                amount_match_rate = 0.0
                amount_match_rate = min(amount, target_amount) / max(amount, target_amount)
                time_score = 1.0
                if seed_context.get("txn_ts"):
                    time_score = max(0.0, 1.0 - (gap_minutes * 60 / effective_window_sec))
                feature_score = round((amount_match_rate * 0.72) + (time_score * 0.28), 4)
                enriched_rows.append(
                    {
                        "txn_id": _trim(row.get("txn_id")),
                        "txn_time": _trim(row.get("txn_time")),
                        "account_key": _trim(row.get("acct_key")),
                        "counterparty_key": _trim(row.get("cp_key")),
                        "counterparty_name": _trim(row.get("cp_name")),
                        "summary": _trim(row.get("summary")),
                        "branch_name": _trim(row.get("branch_name")),
                        "remark": _trim(row.get("remark")),
                        "amount": amount,
                        "direction": "out",
                        "gap_minutes": gap_minutes,
                        "amount_match_rate": round(amount_match_rate, 4),
                        "path_score": feature_score,
                        "file_id": _trim(row.get("file_id")),
                        "source": "materialized_feature",
                        "evidence_ids": [],
                    }
                )

            selected_rows: list[dict[str, Any]] = []
            candidate_type = "unclear"
            if enriched_rows:
                direct_rows = [
                    row for row in enriched_rows
                    if abs(float(row["amount"]) - target_amount) <= max(1.0, target_amount * effective_tolerance)
                ]
                if direct_rows:
                    direct_rows.sort(
                        key=lambda row: (
                            row.get("gap_minutes") or 0,
                            -float(row["amount"]),
                            str(row.get("txn_time") or ""),
                        )
                    )
                    selected_rows = [direct_rows[0]]
                    candidate_type = "single"
                else:
                    running_total = 0.0
                    split_rows: list[dict[str, Any]] = []
                    for row in enriched_rows:
                        split_rows.append(row)
                        running_total += float(row["amount"])
                        if len(split_rows) >= 2 and abs(running_total - target_amount) <= max(1.0, target_amount * effective_tolerance):
                            selected_rows = split_rows
                            candidate_type = "split"
                            break
                        if running_total > target_amount * (1 + effective_tolerance):
                            break
                if not selected_rows:
                    selected_rows = sorted(
                        enriched_rows,
                        key=lambda row: (
                            -float(row["path_score"]),
                            row.get("gap_minutes") or 0,
                            -float(row["amount"]),
                        ),
                    )[: min(3, len(enriched_rows))]
                    candidate_type = "unclear" if len(selected_rows) > 1 else "single"
            result = {
                "semantic_status": "unresolved",
                "fact_answer_allowed": False,
                "coverage_status": "partial",
                "strategy": "materialized_feature",
                "candidate_type": candidate_type,
                "seed": {
                    "seed_txn_id": _trim(seed_txn_id),
                    "seed_account_id": _trim(seed_account_id),
                    "target_account": _trim(seed_context.get("target_account")),
                    "target_amount": round(target_amount, 2),
                    "target_time": str(seed_context.get("txn_ts") or ""),
                    "account_open_name": _trim(seed_context.get("account_open_name")),
                },
                "candidates": selected_rows[:candidate_limit],
                "supporting_txn_ids": _unique_texts(row.get("txn_id") for row in selected_rows),
            }
            return result
        finally:
            if owns_engine:
                engine.close()

    @staticmethod
    def _next_hop_unresolved(blocker: str, *, seed: Mapping[str, Any] | None = None) -> dict[str, Any]:
        return {
            "semantic_status": "unresolved",
            "blocker": _trim(blocker) or "next_hop_inputs_unresolved",
            "fact_answer_allowed": False,
            "coverage_status": "unresolved",
            "strategy": "materialized_feature",
            "candidate_type": "unresolved",
            "seed": dict(seed or {}),
            "candidates": [],
            "supporting_txn_ids": [],
            "evidence_ids": [],
        }

    def get_seed_mixed_fund_tracking(
        self,
        case_id: str,
        *,
        seed_txn_id: str,
        candidate_txn_ids: Optional[Sequence[str]] = None,
        file_ids: Optional[Sequence[str]] = None,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_seed_txn_id = _trim(seed_txn_id)
        if not normalized_seed_txn_id:
            return self._mixed_fund_unresolved("", "seed_transaction_unresolved")
        self.sync_case_baseline(case_id)
        normalized_file_ids = _trim_list(file_ids or [])
        normalized_candidate_txn_ids = _trim_list(candidate_txn_ids or [])
        engine = self.open_case_engine(case_id)
        owns_engine = True
        try:
            self.ensure_analysis_schema(engine)
            ordered_rows = self._load_transaction_rows(engine, case_id, file_ids=normalized_file_ids or None)
            if not ordered_rows:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "transaction_scope_unresolved")
            seed_index = next(
                (
                    index
                    for index, row in enumerate(ordered_rows)
                    if _trim(row.get("txn_id")) == normalized_seed_txn_id
                ),
                -1,
            )
            if seed_index < 0:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "seed_transaction_unresolved")
            seed_row = dict(ordered_rows[seed_index] or {})
            account_key = _trim(seed_row.get("account_key"))
            if not account_key:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "seed_account_unresolved")
            seed_direction = _trim(seed_row.get("direction_norm"))
            seed_amount = _optional_rounded_float(seed_row.get("amount_val"), 2)
            seed_balance = _optional_rounded_float(seed_row.get("balance_val"), 2)
            if seed_amount is None or seed_balance is None:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "seed_amount_or_balance_unresolved")
            if seed_amount <= 0:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "seed_amount_not_positive")
            preexisting_balance = round(max(0.0, seed_balance - seed_amount), 2) if seed_direction == "in" else None
            account_rows = [
                dict(row or {})
                for row in ordered_rows[seed_index:]
                if _trim(row.get("account_key")) == account_key
            ]
            if not account_rows:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "transaction_scope_unresolved")

            candidate_id_set = {_trim(item) for item in normalized_candidate_txn_ids if _trim(item)}
            if not candidate_id_set:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "direct_outflow_scope_unresolved")
            direct_rows = [
                row
                for row in account_rows
                if _trim(row.get("txn_id")) in candidate_id_set and _trim(row.get("direction_norm")) == "out"
            ]
            direct_rows.sort(
                key=lambda row: (
                    row.get("txn_ts") or datetime.min,
                    _trim(row.get("txn_id")),
                )
            )
            if {_trim(row.get("txn_id")) for row in direct_rows} != candidate_id_set:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "direct_outflow_scope_unresolved")
            direct_amounts = [_optional_finite_float(row.get("amount_val")) for row in direct_rows]
            if any(amount is None for amount in direct_amounts):
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "direct_outflow_amount_unresolved")
            direct_outflow_amount = round(
                sum(float(amount) for amount in direct_amounts if amount is not None),
                2,
            )
            direct_outflow_count = len(direct_rows)
            mixed_residual_amount = round(max(0.0, seed_amount - direct_outflow_amount), 2)

            mix_start_index = 0
            if direct_rows:
                last_direct_txn_id = _trim(direct_rows[-1].get("txn_id"))
                for index, row in enumerate(account_rows):
                    if _trim(row.get("txn_id")) == last_direct_txn_id:
                        mix_start_index = index
                        break
            post_mix_rows = account_rows[mix_start_index:] if account_rows[mix_start_index:] else account_rows
            recognition_rows = list(post_mix_rows)
            replenishment_row: dict[str, Any] = {}
            for index, row in enumerate(post_mix_rows[1:], start=1):
                if _trim(row.get("direction_norm")) != "in":
                    continue
                if _is_passive_mixed_fund_replenishment(row):
                    continue
                replenishment_row = dict(row or {})
                recognition_rows = post_mix_rows[:index]
                break
            if not recognition_rows:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "recognition_scope_unresolved")
            recognition_amounts = [_optional_finite_float(row.get("amount_val")) for row in recognition_rows]
            recognition_balances = [_optional_finite_float(row.get("balance_val")) for row in recognition_rows]
            if any(amount is None for amount in recognition_amounts) or any(
                balance is None for balance in recognition_balances
            ):
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "recognition_amount_or_balance_unresolved")
            balance_rows = list(recognition_rows)
            lowest_row = min(
                balance_rows,
                key=lambda row: (
                    float(_optional_finite_float(row.get("balance_val"))),
                    row.get("txn_ts") or datetime.max,
                    _trim(row.get("txn_id")),
                ),
            ) if balance_rows else {}
            lowest_balance_value = _optional_finite_float(lowest_row.get("balance_val")) if lowest_row else None
            if lowest_balance_value is None:
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "lowest_balance_unresolved")
            lowest_balance = round(lowest_balance_value, 2)
            traceable_residual_amount = round(min(mixed_residual_amount, max(0.0, lowest_balance)), 2)
            consumed_amount = round(max(0.0, mixed_residual_amount - traceable_residual_amount), 2)
            latest_row = recognition_rows[-1] if recognition_rows else (post_mix_rows[-1] if post_mix_rows else account_rows[-1])
            lowest_txn_id = _trim(lowest_row.get("txn_id")) if lowest_row else ""
            timeline_rows: list[dict[str, Any]] = []
            for row in recognition_rows:
                row_amount = _optional_rounded_float(row.get("amount_val"), 2)
                row_balance = _optional_rounded_float(row.get("balance_val"), 2)
                if row_amount is None or row_balance is None:
                    return self._mixed_fund_unresolved(
                        normalized_seed_txn_id,
                        "recognition_amount_or_balance_unresolved",
                    )
                timeline_rows.append(
                    {
                        "txn_id": _trim(row.get("txn_id")),
                        "txn_time": _trim(row.get("txn_time")),
                        "direction": _trim(row.get("direction_norm")),
                        "amount": row_amount,
                        "balance": row_balance,
                        "counterparty_key": _trim(row.get("counterparty_key")),
                        "counterparty_name": _trim(row.get("counterparty_name")),
                        "summary": _trim(row.get("summary")),
                        "txn_type": _trim(row.get("txn_type")),
                        "branch_name": _trim(row.get("branch_name")),
                        "remark": _trim(row.get("remark")),
                    }
                )
                if lowest_txn_id and _trim(row.get("txn_id")) == lowest_txn_id:
                    break

            if replenishment_row:
                cutoff_basis = "before_material_replenishment"
                cutoff_label = "截至后续实质性新入账前的最低余额时点"
            else:
                cutoff_basis = "scope_lowest_balance"
                cutoff_label = "截至当前可见后续流水中的最低余额时点"
            latest_balance = _optional_rounded_float(latest_row.get("balance_val"), 2) if latest_row else None
            replenishment_amount = (
                _optional_rounded_float(replenishment_row.get("amount_val"), 2)
                if replenishment_row
                else None
            )
            if latest_balance is None or (replenishment_row and replenishment_amount is None):
                return self._mixed_fund_unresolved(normalized_seed_txn_id, "cutoff_amount_or_balance_unresolved")

            result = {
                "rule_basis": "最低余额法/混同资金认定",
                "seed_txn_id": normalized_seed_txn_id,
                "seed_direction": seed_direction,
                "account_key": account_key,
                "seed_time": _trim(seed_row.get("txn_time")),
                "seed_amount": seed_amount,
                "seed_balance": seed_balance,
                "preexisting_balance": preexisting_balance,
                "direct_outflow_amount": direct_outflow_amount,
                "direct_outflow_count": direct_outflow_count,
                "direct_outflow_txn_ids": [_trim(row.get("txn_id")) for row in direct_rows if _trim(row.get("txn_id"))],
                "mixed_residual_amount": mixed_residual_amount,
                "mix_start_time": _trim((direct_rows[-1] if direct_rows else seed_row).get("txn_time")),
                "lowest_balance": lowest_balance,
                "lowest_balance_time": _trim(lowest_row.get("txn_time")) if lowest_row else _trim(seed_row.get("txn_time")),
                "traceable_residual_amount": traceable_residual_amount,
                "consumed_amount": consumed_amount,
                "cutoff_basis": cutoff_basis,
                "cutoff_label": cutoff_label,
                "cutoff_time": _trim(latest_row.get("txn_time")) if latest_row else "",
                "latest_balance": latest_balance,
                "latest_balance_time": _trim(latest_row.get("txn_time")) if latest_row else "",
                "recognition_cutoff_time": _trim(lowest_row.get("txn_time")) if lowest_row else _trim(seed_row.get("txn_time")),
                "replenishment_time": _trim(replenishment_row.get("txn_time")) if replenishment_row else "",
                "replenishment_amount": replenishment_amount,
                "timeline_rows": timeline_rows[:24],
                "notes": [
                    "先按已锁定的直接转出金额冲减种子入账，再对剩余混同部分适用最低余额法。",
                    "后续新增入账不回补已被消耗的混同剩余可追踪金额。",
                ],
            }
            return result
        finally:
            if owns_engine:
                engine.close()

    @staticmethod
    def _mixed_fund_unresolved(seed_txn_id: str, blocker: str) -> dict[str, Any]:
        return {
            "semantic_status": "unresolved",
            "blocker": _trim(blocker) or "mixed_fund_inputs_unresolved",
            "fact_answer_allowed": False,
            "coverage_status": "unresolved",
            "seed_txn_id": _trim(seed_txn_id),
            "seed_amount": None,
            "seed_balance": None,
            "direct_outflow_amount": None,
            "mixed_residual_amount": None,
            "lowest_balance": None,
            "traceable_residual_amount": None,
            "consumed_amount": None,
            "latest_balance": None,
            "replenishment_amount": None,
            "timeline_rows": [],
            "evidence_ids": [],
            "conclusion_text": "",
        }

    def get_entity_graph(
        self,
        case_id: str,
        *,
        entity_id: str,
        hops: int = 2,
        edge_types: Optional[Sequence[str]] = None,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            txn_rows = self._load_transaction_rows(engine, case_id)
            self._ensure_materialized_graph(engine, case_id, txn_rows, force_refresh=force_refresh)
            requested_entity_id = _trim(entity_id)
            if requested_entity_id and not requested_entity_id.startswith(("acct_", "person_", "device_", "branch_", "voucher_", "teller_", "cp_", "loc_")):
                requested_entity_id = _build_entity_id("account", self._parse_entity_account(requested_entity_id))
            node_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_entity_node WHERE case_id=?",
                (case_id,),
            )
            edge_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_relation_edge WHERE case_id=?",
                (case_id,),
            )
            allowed_edge_types = {_trim(item) for item in edge_types or [] if _trim(item)}
            selected_node_ids = {requested_entity_id} if requested_entity_id else set()
            selected_edge_ids: set[str] = set()
            frontier = set(selected_node_ids)
            remaining_hops = max(1, min(int(hops or 2), 4))
            for _ in range(remaining_hops):
                next_frontier: set[str] = set()
                for row in edge_rows:
                    relation_type = _trim(row.get("relation_type"))
                    if allowed_edge_types and relation_type not in allowed_edge_types:
                        continue
                    src = _trim(row.get("src_entity_id"))
                    dst = _trim(row.get("dst_entity_id"))
                    if src in frontier or dst in frontier:
                        selected_edge_ids.add(_trim(row.get("edge_id")))
                        if src:
                            next_frontier.add(src)
                        if dst:
                            next_frontier.add(dst)
                frontier = next_frontier - selected_node_ids
                selected_node_ids.update(next_frontier)
                if not frontier:
                    break
            selected_nodes = [row for row in node_rows if _trim(row.get("entity_id")) in selected_node_ids]
            selected_edges = [row for row in edge_rows if _trim(row.get("edge_id")) in selected_edge_ids]
            evidence_ready_edges: list[dict[str, Any]] = []
            for row in selected_edges:
                edge_attrs = _json_loads(row.get("attrs_json"), {})
                amount_coverage = edge_attrs.get("amount_coverage") if isinstance(edge_attrs, dict) else None
                if (
                    isinstance(amount_coverage, dict)
                    and _trim(amount_coverage.get("status")) == "complete"
                    and _optional_nonnegative_int(row.get("txn_count")) is not None
                    and _optional_finite_float(row.get("amount_sum")) is not None
                    and _optional_finite_float(row.get("weight")) is not None
                ):
                    evidence_ready_edges.append(row)
            evidence_ids = self._evidence_ids_for_refs(
                engine,
                case_id,
                [
                    *[("analysis_entity_node", _trim(row.get("entity_id"))) for row in selected_nodes],
                    *[("analysis_relation_edge", _trim(row.get("edge_id"))) for row in evidence_ready_edges],
                ],
            )
            stats = {
                "node_count": len(selected_nodes),
                "edge_count": len(selected_edges),
                "hops": remaining_hops,
                "unresolved_amount_edge_count": len(selected_edges) - len(evidence_ready_edges),
            }
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="get_entity_graph",
                params={"case_id": case_id, "entity_id": entity_id, "hops": remaining_hops, "edge_types": list(edge_types or [])},
                summary=stats,
                row_count=len(selected_edges),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "query_id": query_id,
                "nodes": [
                    {
                        "entity_id": _trim(row.get("entity_id")),
                        "entity_type": _trim(row.get("entity_type")),
                        "entity_key": _trim(row.get("entity_key")),
                        "display_name": _trim(row.get("display_name")),
                        "first_seen_at": _trim(row.get("first_seen_at")),
                        "last_seen_at": _trim(row.get("last_seen_at")),
                        "attrs_json": _json_loads(row.get("attrs_json"), {}),
                    }
                    for row in selected_nodes
                ],
                "edges": [
                    {
                        "edge_id": _trim(row.get("edge_id")),
                        "src_entity_id": _trim(row.get("src_entity_id")),
                        "dst_entity_id": _trim(row.get("dst_entity_id")),
                        "relation_type": _trim(row.get("relation_type")),
                        "weight": _optional_rounded_float(row.get("weight"), 2),
                        "txn_count": _optional_nonnegative_int(row.get("txn_count")),
                        "amount_sum": _optional_rounded_float(row.get("amount_sum"), 2),
                        "attrs_json": _json_loads(row.get("attrs_json"), {}),
                    }
                    for row in selected_edges
                ],
                "stats": stats,
                "evidence_ids": evidence_ids,
            }
        finally:
            engine.close()

    def get_device_links(
        self,
        case_id: str,
        *,
        ip: str = "",
        mac: str = "",
        person_id: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            if self._table_exists(engine, "analysis_txn_detail_idx"):
                return self._get_device_links_from_detail_idx(
                    engine,
                    case_id,
                    ip=ip,
                    mac=mac,
                    person_id=person_id,
                    cursor=cursor,
                    limit=limit,
                    started=started,
                )
            txn_rows = self._load_transaction_rows(engine, case_id)
            if force_refresh:
                self._ensure_materialized_graph(engine, case_id, txn_rows, force_refresh=True)
            normalized_person = self._parse_entity_person(person_id) if person_id else ""
            normalized_ip = _trim(ip)
            normalized_mac = _normalize_mac(mac)
            grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
            for row in txn_rows:
                device_entity_id = _trim(row.get("device_entity_id"))
                if not device_entity_id:
                    continue
                if normalized_ip and normalized_ip not in {_trim(row.get("ip_addr")), _trim(row.get("ip_key"))}:
                    continue
                if normalized_mac and normalized_mac not in {_trim(row.get("mac_key")), _normalize_mac(row.get("mac_addr"))}:
                    continue
                if normalized_person and normalized_person not in {_trim(row.get("opener_id_no")), self._parse_entity_person(_trim(row.get("person_entity_id")))}:
                    continue
                grouped[device_entity_id].append(row)
            items: list[dict[str, Any]] = []
            for device_entity_id, rows in grouped.items():
                rows.sort(key=lambda item: (item.get("txn_ts") or datetime.min, _trim(item.get("txn_id"))))
                device_value = _trim(rows[0].get("mac_addr")) or _trim(rows[0].get("ip_addr")) or _trim(rows[0].get("device_key"))
                txn_evidence_ids: list[str] = []
                for row in rows[:6]:
                    title, snippet, payload = self._txn_evidence_payload(row)
                    txn_evidence_ids.append(
                        self._upsert_evidence_ref(
                            engine,
                            case_id=case_id,
                            evidence_type="txn",
                            ref_table="fc_transaction_norm",
                            ref_pk=_trim(row.get("txn_id")),
                            title=title,
                            snippet=snippet,
                            payload=payload,
                        )
                    )
                evidence_ids = _unique_texts(txn_evidence_ids)
                items.append(
                    {
                        "link_id": _stable_slug("device_link", case_id, device_entity_id),
                        "device_entity_id": device_entity_id,
                        "device_label": "MAC" if _trim(rows[0].get("mac_addr")) else "IP",
                        "device_value": device_value,
                        "account_ids": _unique_texts(row.get("account_entity_id") for row in rows),
                        "person_ids": _unique_texts(row.get("person_entity_id") for row in rows),
                        "txn_ids": _clip_items((row.get("txn_id") for row in rows), 16),
                        "txn_count": len(rows),
                        "first_seen_at": _trim(rows[0].get("txn_time")),
                        "last_seen_at": _trim(rows[-1].get("txn_time")),
                        "summary": f"设备 {device_value or device_entity_id} 关联 {len(_unique_texts(row.get('account_key') for row in rows))} 个账户。",
                        "evidence_ids": evidence_ids,
                        "detail_json": {
                            "ip_addrs": _unique_texts(row.get("ip_addr") for row in rows),
                            "mac_addrs": _unique_texts(row.get("mac_addr") for row in rows),
                            "account_keys": _unique_texts(row.get("account_key") for row in rows),
                            "persons": _unique_texts(row.get("opener_id_no") for row in rows),
                        },
                    }
                )
            items.sort(key=lambda item: (-_as_int(item.get("txn_count")), _trim(item.get("device_value"))))
            offset = _decode_offset_cursor(cursor)
            page_limit = max(1, min(int(limit or 20), 100))
            page_items = items[offset : offset + page_limit]
            has_more = offset + page_limit < len(items)
            next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
            stats = {
                "matched_links": len(items),
                "returned_links": len(page_items),
                "account_count": len({_trim(account_id) for item in items for account_id in item.get("account_ids", [])}),
            }
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="get_device_links",
                params={"case_id": case_id, "ip": ip, "mac": mac, "person_id": person_id, "cursor": cursor, "limit": page_limit},
                summary=stats,
                row_count=len(page_items),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "query_id": query_id,
                "items": page_items,
                "stats": stats,
                "evidence_ids": _unique_texts(evidence_id for item in page_items for evidence_id in item.get("evidence_ids", [])),
                "cursor": next_cursor,
                "has_more": has_more,
                "truncated": False,
            }
        finally:
            engine.close()

    def _detail_index_sample_rows(
        self,
        engine: DuckDBEngine,
        case_id: str,
        where_sql: str,
        params: Sequence[Any],
        *,
        limit: int = 16,
    ) -> list[dict[str, Any]]:
        rows = self._query_dicts(
            engine,
            f"""
            SELECT
                acct_key AS account_key,
                COALESCE(NULLIF(TRIM(account_open_name), ''), acct_key) AS account_name,
                txn_id,
                id AS row_id,
                txn_time,
                txn_ts AS txn_ts_val,
                amount AS amount_val,
                balance AS balance_val,
                dc_val AS direction_raw,
                is_success AS success_raw,
                query_feedback_reason AS reason_raw,
                opener_id_no,
                ip_addr,
                mac_addr,
                counterparty_acct,
                counterparty_name,
                counterparty_id_no,
                counterparty_bank,
                summary,
                currency,
                branch_name,
                location,
                cash_flag AS cash_raw,
                voucher_no,
                '' AS receipt_no,
                log_id AS log_no,
                voucher_type,
                voucher_id,
                teller_no,
                remark,
                txn_type,
                file_id
            FROM analysis_txn_detail_idx
            WHERE {where_sql}
            ORDER BY (txn_ts IS NULL) ASC, txn_ts ASC, COALESCE(txn_id, CAST(id AS VARCHAR), acct_key) ASC
            LIMIT ?
            """,
            [*params, max(1, int(limit or 16))],
        )
        return self._normalize_transaction_query_rows(case_id, rows)

    @staticmethod
    def _split_agg_values(value: Any) -> list[str]:
        if value is None:
            return []
        return _unique_texts(str(value).split("\x1f"))

    def _detail_index_distinct_values(
        self,
        engine: DuckDBEngine,
        where_sql: str,
        params: Sequence[Any],
    ) -> dict[str, list[str]]:
        row = (
            self._query_dicts(
                engine,
                f"""
                SELECT
                    STRING_AGG(DISTINCT CASE WHEN TRIM(COALESCE(acct_key, '')) <> '' THEN acct_key ELSE NULL END, '\x1f') AS account_keys,
                    STRING_AGG(DISTINCT CASE WHEN TRIM(COALESCE(opener_id_no, '')) <> '' THEN opener_id_no ELSE NULL END, '\x1f') AS opener_ids,
                    STRING_AGG(DISTINCT CASE WHEN TRIM(COALESCE(ip_addr, '')) <> '' THEN ip_addr ELSE NULL END, '\x1f') AS ip_addrs,
                    STRING_AGG(DISTINCT CASE WHEN TRIM(COALESCE(mac_addr, '')) <> '' THEN mac_addr ELSE NULL END, '\x1f') AS mac_addrs
                FROM analysis_txn_detail_idx
                WHERE {where_sql}
                """,
                params,
            )
            or [{}]
        )[0]
        return {
            "account_keys": self._split_agg_values(row.get("account_keys")),
            "opener_ids": self._split_agg_values(row.get("opener_ids")),
            "ip_addrs": self._split_agg_values(row.get("ip_addrs")),
            "mac_addrs": self._split_agg_values(row.get("mac_addrs")),
        }

    def _get_device_links_from_detail_idx(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        ip: str,
        mac: str,
        person_id: str,
        cursor: str,
        limit: int,
        started: float,
    ) -> dict[str, Any]:
        normalized_person = self._parse_entity_person(person_id) if person_id else ""
        normalized_ip = _trim(ip)
        normalized_mac = _normalize_mac(mac)
        device_expr = "COALESCE(NULLIF(TRIM(mac_addr), ''), NULLIF(TRIM(ip_addr), ''))"
        label_expr = "CASE WHEN TRIM(COALESCE(mac_addr, '')) <> '' THEN 'MAC' ELSE 'IP' END"
        where = [f"{device_expr} IS NOT NULL"]
        params: list[Any] = []
        if normalized_ip:
            where.append("TRIM(COALESCE(ip_addr, '')) = ?")
            params.append(normalized_ip)
        if normalized_mac:
            where.append("REGEXP_REPLACE(LOWER(COALESCE(mac_addr, '')), '[^0-9a-f]', '', 'g') = ?")
            params.append(normalized_mac)
        if normalized_person:
            where.append("TRIM(COALESCE(opener_id_no, '')) = ?")
            params.append(normalized_person)
        where_sql = " AND ".join(where)
        offset = _decode_offset_cursor(cursor)
        page_limit = max(1, min(int(limit or 20), 100))
        total_row = (
            self._query_dicts(
                engine,
                f"""
                SELECT COUNT(1) AS matched_links
                  FROM (
                    SELECT {device_expr} AS device_value, {label_expr} AS device_label
                      FROM analysis_txn_detail_idx
                     WHERE {where_sql}
                     GROUP BY 1, 2
                  ) groups
                """,
                params,
            )
            or [{}]
        )[0]
        group_rows = self._query_dicts(
            engine,
            f"""
            SELECT
                {device_expr} AS device_value,
                {label_expr} AS device_label,
                COUNT(1) AS txn_count,
                MIN(txn_ts) AS first_seen_at,
                MAX(txn_ts) AS last_seen_at
              FROM analysis_txn_detail_idx
             WHERE {where_sql}
             GROUP BY 1, 2
             ORDER BY txn_count DESC, device_value ASC
             LIMIT ? OFFSET ?
            """,
            [*params, page_limit, offset],
        )
        items: list[dict[str, Any]] = []
        for row in group_rows:
            device_value = _trim(row.get("device_value"))
            device_label = _trim(row.get("device_label")) or "设备"
            group_where = f"{where_sql} AND {device_expr} = ? AND {label_expr} = ?"
            group_params = [*params, device_value, device_label]
            distinct_values = self._detail_index_distinct_values(engine, group_where, group_params)
            sample_rows = self._detail_index_sample_rows(engine, case_id, group_where, group_params, limit=16)
            txn_evidence_ids: list[str] = []
            for sample in sample_rows[:6]:
                title, snippet, payload = self._txn_evidence_payload(sample)
                txn_evidence_ids.append(
                    self._upsert_evidence_ref(
                        engine,
                        case_id=case_id,
                        evidence_type="txn",
                        ref_table="fc_transaction_norm",
                        ref_pk=_trim(sample.get("txn_id")),
                        title=title,
                        snippet=snippet,
                        payload=payload,
                    )
                )
            device_entity_id = _build_entity_id("device", device_value) if device_value else ""
            items.append(
                {
                    "link_id": _stable_slug("device_link", case_id, device_entity_id or device_value),
                    "device_entity_id": device_entity_id,
                    "device_label": device_label,
                    "device_value": device_value,
                    "account_ids": [_build_entity_id("account", item) for item in distinct_values["account_keys"]],
                    "person_ids": [_build_entity_id("person", item) for item in distinct_values["opener_ids"]],
                    "txn_ids": _clip_items((sample.get("txn_id") for sample in sample_rows), 16),
                    "txn_count": _optional_nonnegative_int(row.get("txn_count")),
                    "first_seen_at": _datetime_text(row.get("first_seen_at")),
                    "last_seen_at": _datetime_text(row.get("last_seen_at")),
                    "summary": f"设备 {device_value or device_entity_id} 关联 {len(distinct_values['account_keys'])} 个账户。",
                    "evidence_ids": _unique_texts(txn_evidence_ids),
                    "detail_json": {
                        "ip_addrs": distinct_values["ip_addrs"],
                        "mac_addrs": distinct_values["mac_addrs"],
                        "account_keys": distinct_values["account_keys"],
                        "persons": distinct_values["opener_ids"],
                    },
                }
            )
        matched_links = _optional_nonnegative_int(total_row.get("matched_links"))
        has_more = matched_links is not None and offset + page_limit < matched_links
        next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
        stats = {
            "matched_links": matched_links,
            "returned_links": len(items),
            "account_count": len({_trim(account_id) for item in items for account_id in item.get("account_ids", [])}),
        }
        query_id = self._append_query_log(
            engine,
            case_id=case_id,
            tool_name="get_device_links",
            params={"case_id": case_id, "ip": ip, "mac": mac, "person_id": person_id, "cursor": cursor, "limit": page_limit, "source": "detail_idx"},
            summary=stats,
            row_count=len(items),
            duration_ms=int((time.perf_counter() - started) * 1000),
        )
        return {
            "query_id": query_id,
            "items": items,
            "stats": stats,
            "evidence_ids": _unique_texts(evidence_id for item in items for evidence_id in item.get("evidence_ids", [])),
            "cursor": next_cursor,
            "has_more": has_more,
            "truncated": False,
        }

    def get_branch_voucher_links(
        self,
        case_id: str,
        *,
        voucher_no: str = "",
        teller_no: str = "",
        log_no: str = "",
        cursor: str = "",
        limit: int = 20,
        force_refresh: bool = False,
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            if self._table_exists(engine, "analysis_txn_detail_idx"):
                return self._get_branch_voucher_links_from_detail_idx(
                    engine,
                    case_id,
                    voucher_no=voucher_no,
                    teller_no=teller_no,
                    log_no=log_no,
                    cursor=cursor,
                    limit=limit,
                    started=started,
                )
            txn_rows = self._load_transaction_rows(engine, case_id)
            if force_refresh:
                self._ensure_materialized_graph(engine, case_id, txn_rows, force_refresh=True)
            normalized_voucher = _trim(voucher_no)
            normalized_teller = _trim(teller_no)
            normalized_log = _trim(log_no)
            grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
            for row in txn_rows:
                if normalized_voucher and _trim(row.get("voucher_no")) != normalized_voucher:
                    continue
                if normalized_teller and _trim(row.get("teller_no")) != normalized_teller:
                    continue
                if normalized_log and _trim(row.get("log_no")) != normalized_log:
                    continue
                if not any((_trim(row.get("branch_name")), _trim(row.get("voucher_no")), _trim(row.get("teller_no")), _trim(row.get("log_no")))):
                    continue
                key = _stable_slug(
                    "bvl",
                    case_id,
                    _trim(row.get("branch_name")),
                    _trim(row.get("voucher_type")),
                    _trim(row.get("voucher_no")),
                    _trim(row.get("teller_no")),
                    _trim(row.get("log_no")),
                )
                grouped[key].append(row)
            items: list[dict[str, Any]] = []
            for link_id, rows in grouped.items():
                rows.sort(key=lambda item: (item.get("txn_ts") or datetime.min, _trim(item.get("txn_id"))))
                branch_entity_id = _trim(rows[0].get("branch_entity_id"))
                voucher_entity_id = _trim(rows[0].get("voucher_entity_id"))
                teller_entity_id = _trim(rows[0].get("teller_entity_id"))
                txn_evidence_ids: list[str] = []
                for row in rows[:6]:
                    title, snippet, payload = self._txn_evidence_payload(row)
                    txn_evidence_ids.append(
                        self._upsert_evidence_ref(
                            engine,
                            case_id=case_id,
                            evidence_type="txn",
                            ref_table="fc_transaction_norm",
                            ref_pk=_trim(row.get("txn_id")),
                            title=title,
                            snippet=snippet,
                            payload=payload,
                        )
                    )
                evidence_ids = _unique_texts(txn_evidence_ids)
                items.append(
                    {
                        "link_id": link_id,
                        "branch_entity_id": branch_entity_id,
                        "branch_name": _trim(rows[0].get("branch_name")),
                        "voucher_entity_id": voucher_entity_id,
                        "voucher_no": _trim(rows[0].get("voucher_no")),
                        "voucher_type": _trim(rows[0].get("voucher_type")),
                        "teller_entity_id": teller_entity_id,
                        "teller_no": _trim(rows[0].get("teller_no")),
                        "log_no": _trim(rows[0].get("log_no")),
                        "account_ids": _unique_texts(row.get("account_entity_id") for row in rows),
                        "txn_ids": _clip_items((row.get("txn_id") for row in rows), 16),
                        "txn_count": len(rows),
                        "summary": (
                            f"网点 {_trim(rows[0].get('branch_name')) or '未知网点'} / 柜员 {_trim(rows[0].get('teller_no')) or '未知柜员'} "
                            f"关联 {len(_unique_texts(row.get('account_key') for row in rows))} 个账户。"
                        ),
                        "evidence_ids": evidence_ids,
                        "detail_json": {
                            "voucher_no": _trim(rows[0].get("voucher_no")),
                            "voucher_type": _trim(rows[0].get("voucher_type")),
                            "receipt_no": _unique_texts(row.get("receipt_no") for row in rows),
                            "log_no": _trim(rows[0].get("log_no")),
                            "account_keys": _unique_texts(row.get("account_key") for row in rows),
                        },
                    }
                )
            items.sort(key=lambda item: (-_as_int(item.get("txn_count")), _trim(item.get("branch_name"))))
            offset = _decode_offset_cursor(cursor)
            page_limit = max(1, min(int(limit or 20), 100))
            page_items = items[offset : offset + page_limit]
            has_more = offset + page_limit < len(items)
            next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
            stats = {
                "matched_links": len(items),
                "returned_links": len(page_items),
                "account_count": len({_trim(account_id) for item in items for account_id in item.get("account_ids", [])}),
            }
            query_id = self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="get_branch_voucher_links",
                params={
                    "case_id": case_id,
                    "voucher_no": voucher_no,
                    "teller_no": teller_no,
                    "log_no": log_no,
                    "cursor": cursor,
                    "limit": page_limit,
                },
                summary=stats,
                row_count=len(page_items),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            return {
                "query_id": query_id,
                "items": page_items,
                "stats": stats,
                "evidence_ids": _unique_texts(evidence_id for item in page_items for evidence_id in item.get("evidence_ids", [])),
                "cursor": next_cursor,
                "has_more": has_more,
                "truncated": False,
            }
        finally:
            engine.close()

    def _get_branch_voucher_links_from_detail_idx(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        voucher_no: str,
        teller_no: str,
        log_no: str,
        cursor: str,
        limit: int,
        started: float,
    ) -> dict[str, Any]:
        normalized_voucher = _trim(voucher_no)
        normalized_teller = _trim(teller_no)
        normalized_log = _trim(log_no)
        populated_expr = (
            "TRIM(COALESCE(branch_name, '')) <> '' OR TRIM(COALESCE(voucher_no, '')) <> '' "
            "OR TRIM(COALESCE(teller_no, '')) <> '' OR TRIM(COALESCE(log_id, '')) <> ''"
        )
        where = [f"({populated_expr})"]
        params: list[Any] = []
        if normalized_voucher:
            where.append("TRIM(COALESCE(voucher_no, '')) = ?")
            params.append(normalized_voucher)
        if normalized_teller:
            where.append("TRIM(COALESCE(teller_no, '')) = ?")
            params.append(normalized_teller)
        if normalized_log:
            where.append("TRIM(COALESCE(log_id, '')) = ?")
            params.append(normalized_log)
        where_sql = " AND ".join(where)
        offset = _decode_offset_cursor(cursor)
        page_limit = max(1, min(int(limit or 20), 100))
        group_select = """
            COALESCE(TRIM(branch_name), '') AS branch_name,
            COALESCE(TRIM(voucher_type), '') AS voucher_type,
            COALESCE(TRIM(voucher_no), '') AS voucher_no,
            COALESCE(TRIM(teller_no), '') AS teller_no,
            COALESCE(TRIM(log_id), '') AS log_no
        """
        total_row = (
            self._query_dicts(
                engine,
                f"""
                SELECT COUNT(1) AS matched_links
                  FROM (
                    SELECT {group_select}
                      FROM analysis_txn_detail_idx
                     WHERE {where_sql}
                     GROUP BY 1, 2, 3, 4, 5
                  ) groups
                """,
                params,
            )
            or [{}]
        )[0]
        group_rows = self._query_dicts(
            engine,
            f"""
            SELECT
                {group_select},
                COUNT(1) AS txn_count
              FROM analysis_txn_detail_idx
             WHERE {where_sql}
             GROUP BY 1, 2, 3, 4, 5
             ORDER BY txn_count DESC, branch_name ASC, teller_no ASC, voucher_no ASC, log_no ASC
             LIMIT ? OFFSET ?
            """,
            [*params, page_limit, offset],
        )
        items: list[dict[str, Any]] = []
        for row in group_rows:
            branch_name = _trim(row.get("branch_name"))
            voucher_type = _trim(row.get("voucher_type"))
            current_voucher_no = _trim(row.get("voucher_no"))
            current_teller_no = _trim(row.get("teller_no"))
            current_log_no = _trim(row.get("log_no"))
            group_where_parts = [where_sql]
            group_params: list[Any] = [*params]
            for column_name, value in (
                ("branch_name", branch_name),
                ("voucher_type", voucher_type),
                ("voucher_no", current_voucher_no),
                ("teller_no", current_teller_no),
                ("log_id", current_log_no),
            ):
                if value:
                    group_where_parts.append(f"TRIM(COALESCE({column_name}, '')) = ?")
                    group_params.append(value)
                else:
                    group_where_parts.append(f"TRIM(COALESCE({column_name}, '')) = ''")
            group_where = " AND ".join(group_where_parts)
            distinct_values = self._detail_index_distinct_values(engine, group_where, group_params)
            sample_rows = self._detail_index_sample_rows(engine, case_id, group_where, group_params, limit=16)
            txn_evidence_ids: list[str] = []
            for sample in sample_rows[:6]:
                title, snippet, payload = self._txn_evidence_payload(sample)
                txn_evidence_ids.append(
                    self._upsert_evidence_ref(
                        engine,
                        case_id=case_id,
                        evidence_type="txn",
                        ref_table="fc_transaction_norm",
                        ref_pk=_trim(sample.get("txn_id")),
                        title=title,
                        snippet=snippet,
                        payload=payload,
                    )
                )
            branch_entity_id = _build_entity_id("branch", branch_name) if branch_name else ""
            voucher_key = ":".join(_unique_texts((voucher_type, current_voucher_no or current_log_no)))
            voucher_entity_id = _build_entity_id("voucher", voucher_key) if voucher_key else ""
            teller_entity_id = _build_entity_id("teller", current_teller_no) if current_teller_no else ""
            link_id = _stable_slug("bvl", case_id, branch_name, voucher_type, current_voucher_no, current_teller_no, current_log_no)
            items.append(
                {
                    "link_id": link_id,
                    "branch_entity_id": branch_entity_id,
                    "branch_name": branch_name,
                    "voucher_entity_id": voucher_entity_id,
                    "voucher_no": current_voucher_no,
                    "voucher_type": voucher_type,
                    "teller_entity_id": teller_entity_id,
                    "teller_no": current_teller_no,
                    "log_no": current_log_no,
                    "account_ids": [_build_entity_id("account", item) for item in distinct_values["account_keys"]],
                    "txn_ids": _clip_items((sample.get("txn_id") for sample in sample_rows), 16),
                    "txn_count": _optional_nonnegative_int(row.get("txn_count")),
                    "summary": (
                        f"网点 {branch_name or '未知网点'} / 柜员 {current_teller_no or '未知柜员'} "
                        f"关联 {len(distinct_values['account_keys'])} 个账户。"
                    ),
                    "evidence_ids": _unique_texts(txn_evidence_ids),
                    "detail_json": {
                        "voucher_no": current_voucher_no,
                        "voucher_type": voucher_type,
                        "receipt_no": [],
                        "log_no": current_log_no,
                        "account_keys": distinct_values["account_keys"],
                    },
                }
            )
        matched_links = _optional_nonnegative_int(total_row.get("matched_links"))
        has_more = matched_links is not None and offset + page_limit < matched_links
        next_cursor = _encode_offset_cursor(offset + page_limit) if has_more else ""
        stats = {
            "matched_links": matched_links,
            "returned_links": len(items),
            "account_count": len({_trim(account_id) for item in items for account_id in item.get("account_ids", [])}),
        }
        query_id = self._append_query_log(
            engine,
            case_id=case_id,
            tool_name="get_branch_voucher_links",
            params={
                "case_id": case_id,
                "voucher_no": voucher_no,
                "teller_no": teller_no,
                "log_no": log_no,
                "cursor": cursor,
                "limit": page_limit,
                "source": "detail_idx",
            },
            summary=stats,
            row_count=len(items),
            duration_ms=int((time.perf_counter() - started) * 1000),
        )
        return {
            "query_id": query_id,
            "items": items,
            "stats": stats,
            "evidence_ids": _unique_texts(evidence_id for item in items for evidence_id in item.get("evidence_ids", [])),
            "cursor": next_cursor,
            "has_more": has_more,
            "truncated": False,
        }

    @staticmethod
    def _trace_path_detail_is_complete(
        path_row: Mapping[str, Any],
        path_detail: Mapping[str, Any],
        *,
        hop_rows: Sequence[Mapping[str, Any]] | None = None,
    ) -> bool:
        path_rank = _optional_positive_int(
            _first_known_value(path_detail.get("path_rank"), path_row.get("path_index"))
        )
        compared_path_count = _optional_positive_int(path_detail.get("compared_path_count"))
        if path_rank is None or compared_path_count is None or path_rank > compared_path_count:
            return False
        required_numbers = (
            path_row.get("path_score"),
            path_row.get("amount_match_rate"),
            path_detail.get("conservation_score"),
            path_detail.get("cycle_risk_score"),
            path_detail.get("merge_support_ratio"),
            path_detail.get("amount_gap"),
            path_detail.get("root_amount"),
            path_detail.get("sink_amount"),
        )
        required_bools = (
            path_detail.get("cycle_detected"),
            path_detail.get("round_trip_detected"),
            path_detail.get("merge_detected"),
            path_detail.get("split_detected"),
        )
        risk_level = _trim(path_detail.get("cycle_risk_level"))
        if any(_optional_finite_float(value) is None for value in required_numbers):
            return False
        if any(_optional_bool(value) is None for value in required_bools):
            return False
        if risk_level not in {"low", "medium", "high", "critical"}:
            return False
        breakdown = path_detail.get("merge_support_breakdown")
        replay_steps = path_detail.get("replay_steps")
        if not isinstance(breakdown, list) or not isinstance(replay_steps, list):
            return False
        for item in breakdown:
            if not isinstance(item, Mapping) or any(
                _optional_finite_float(item.get(key)) is None
                for key in ("amount", "share_rate", "allocated_amount")
            ):
                return False
        for item in replay_steps:
            if not isinstance(item, Mapping) or any(
                _optional_finite_float(item.get(key)) is None
                for key in ("amount", "allocated_amount", "amount_share_rate")
            ):
                return False
        if hop_rows is not None:
            if not hop_rows:
                return False
            if any(
                _optional_positive_int(row.get("hop_index")) is None
                or _optional_finite_float(row.get("amount")) is None
                for row in hop_rows
            ):
                return False
        return True

    @staticmethod
    def _trace_private_unresolved_summary() -> str:
        return "路径金额或评分字段未核验，不能作为案件事实或风险结论。"

    def create_trace_run_shell(
        self,
        case_id: str,
        *,
        seed_type: str,
        seed_value: str,
        depth: int,
        time_window_sec: int,
        tolerance_rate: float,
        trace_id: Optional[str] = None,
        status: str = "queued",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_tolerance_rate = max(0.0, _as_float(tolerance_rate))
        trace_key = _trim(trace_id) or _slug("tr")
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            existing = self._query_dicts(engine, "SELECT trace_id FROM analysis_trace_run WHERE trace_id=? LIMIT 1", (trace_key,))
            if existing:
                engine.execute(
                    """
                    UPDATE analysis_trace_run
                       SET seed_type=?, seed_value=?, depth=?, time_window_sec=?, tolerance_rate=?, status=?, summary_json=?, updated_at=?
                     WHERE trace_id=?
                    """,
                    (
                        _trim(seed_type),
                        _trim(seed_value),
                        max(1, int(depth)),
                        max(60, int(time_window_sec)),
                        normalized_tolerance_rate,
                        _trim(status) or "queued",
                        "{}",
                        _now_text(),
                        trace_key,
                    ),
                )
            else:
                engine.execute(
                    """
                    INSERT INTO analysis_trace_run(
                        trace_id, case_id, seed_type, seed_value, depth, time_window_sec, tolerance_rate,
                        status, summary_json, created_at, updated_at
                    ) VALUES (?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        trace_key,
                        case_id,
                        _trim(seed_type),
                        _trim(seed_value),
                        max(1, int(depth)),
                        max(60, int(time_window_sec)),
                        normalized_tolerance_rate,
                        _trim(status) or "queued",
                        "{}",
                        _now_text(),
                        _now_text(),
                    ),
                )
            return {"trace_id": trace_key, "status": _trim(status) or "queued"}
        finally:
            engine.close()

    def update_trace_run_status(
        self,
        case_id: str,
        *,
        trace_id: str,
        status: str,
        summary: Optional[dict[str, Any]] = None,
    ) -> None:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            engine.execute(
                "UPDATE analysis_trace_run SET status=?, summary_json=?, updated_at=? WHERE trace_id=? AND case_id=?",
                (_trim(status), _json_dumps(summary or {}), _now_text(), _trim(trace_id), case_id),
            )
        finally:
            engine.close()

    def _trace_seed_rows(self, txn_rows: Sequence[dict[str, Any]], seed_type: str, seed_value: str) -> list[dict[str, Any]]:
        normalized_seed_type = _trim(seed_type).lower()
        normalized_seed_value = _trim(seed_value)
        if normalized_seed_type == "txn_id":
            return [row for row in txn_rows if _trim(row.get("txn_id")) == normalized_seed_value][:1]
        account_key = self._parse_entity_account(normalized_seed_value)
        candidate_rows = [row for row in txn_rows if _trim(row.get("account_key")) == account_key]
        inbound_first = [row for row in candidate_rows if row.get("direction_norm") == "in"] or candidate_rows

        def amount_desc_key(row: Mapping[str, Any]) -> tuple[Any, ...]:
            amount = _trace_amount_value(row)
            return (
                amount is None,
                -amount if amount is not None else math.inf,
                row.get("txn_ts") or datetime.min,
                _trim(row.get("txn_id")),
            )

        inbound_first.sort(key=amount_desc_key)
        return inbound_first[:3]

    def _trace_candidate_out_rows(
        self,
        outgoing_rows: Sequence[dict[str, Any]],
        *,
        anchor_time: Optional[datetime],
        anchor_amount: float,
        time_window_sec: int,
        tolerance_rate: float,
        used_txn_ids: set[str],
    ) -> tuple[list[dict[str, Any]], int]:
        if anchor_time is None or not math.isfinite(anchor_amount) or anchor_amount <= 0:
            return [], 0
        candidates: list[dict[str, Any]] = []
        for row in outgoing_rows:
            row_amount = _trace_amount_value(row)
            if (
                row_amount is None
                or row.get("txn_ts") is None
                or not anchor_time < row["txn_ts"] <= anchor_time + timedelta(seconds=max(60, int(time_window_sec)))
                or _trim(row.get("txn_id")) in used_txn_ids
                or row_amount > anchor_amount * (1 + max(0.0, tolerance_rate))
                or row_amount < max(anchor_amount * 0.15, 1.0)
            ):
                continue
            candidates.append(row)

        def amount_gap_key(row: Mapping[str, Any]) -> tuple[Any, ...]:
            amount = _trace_amount_value(row)
            return (
                abs(amount - anchor_amount) if amount is not None else math.inf,
                row.get("txn_ts") or datetime.max,
                _trim(row.get("txn_id")),
            )

        candidates.sort(key=amount_gap_key)
        bundle: list[dict[str, Any]] = []
        current_sum = 0.0
        for row in candidates[:8]:
            row_amount = _trace_amount_value(row)
            if row_amount is None:
                continue
            if current_sum + row_amount > anchor_amount * (1 + max(0.0, tolerance_rate)):
                continue
            bundle.append(row)
            current_sum += row_amount
            if current_sum >= anchor_amount * max(0.55, 1 - max(0.0, tolerance_rate)):
                break
        return bundle or candidates[:1], len(candidates)

    def _trace_follow_chain(
        self,
        outgoing_by_account: dict[str, list[dict[str, Any]]],
        *,
        anchor_row: dict[str, Any],
        depth_remaining: int,
        time_window_sec: int,
        tolerance_rate: float,
        used_txn_ids: set[str],
    ) -> list[dict[str, Any]]:
        if depth_remaining <= 0:
            return []
        next_account = _trim(anchor_row.get("counterparty_acct"))
        anchor_amount = _trace_amount_value(anchor_row)
        if not next_account or anchor_amount is None:
            return []
        next_candidates, _ = self._trace_candidate_out_rows(
            outgoing_by_account.get(next_account, []),
            anchor_time=anchor_row.get("txn_ts"),
            anchor_amount=anchor_amount,
            time_window_sec=time_window_sec,
            tolerance_rate=tolerance_rate,
            used_txn_ids=used_txn_ids,
        )
        if not next_candidates:
            return []
        best = next_candidates[0]
        next_used = set(used_txn_ids)
        next_used.add(_trim(best.get("txn_id")))
        return [best, *self._trace_follow_chain(
            outgoing_by_account,
            anchor_row=best,
            depth_remaining=depth_remaining - 1,
            time_window_sec=time_window_sec,
            tolerance_rate=tolerance_rate,
            used_txn_ids=next_used,
        )]

    def _trace_expand_paths(
        self,
        outgoing_by_account: dict[str, list[dict[str, Any]]],
        *,
        anchor_row: dict[str, Any],
        depth_remaining: int,
        time_window_sec: int,
        tolerance_rate: float,
        used_txn_ids: set[str],
        max_branch: int = 2,
    ) -> list[list[dict[str, Any]]]:
        if depth_remaining <= 0:
            return [[]]
        next_account = _trim(anchor_row.get("counterparty_acct"))
        anchor_amount = _trace_amount_value(anchor_row)
        if not next_account or anchor_amount is None:
            return [[]]
        next_candidates, _ = self._trace_candidate_out_rows(
            outgoing_by_account.get(next_account, []),
            anchor_time=anchor_row.get("txn_ts"),
            anchor_amount=anchor_amount,
            time_window_sec=time_window_sec,
            tolerance_rate=tolerance_rate,
            used_txn_ids=used_txn_ids,
        )
        if not next_candidates:
            return [[]]
        paths: list[list[dict[str, Any]]] = []
        seen_signatures: set[str] = set()
        for candidate in next_candidates[: max(1, int(max_branch))]:
            txn_id = _trim(candidate.get("txn_id"))
            next_used = set(used_txn_ids)
            if txn_id:
                next_used.add(txn_id)
            tails = self._trace_expand_paths(
                outgoing_by_account,
                anchor_row=candidate,
                depth_remaining=depth_remaining - 1,
                time_window_sec=time_window_sec,
                tolerance_rate=tolerance_rate,
                used_txn_ids=next_used,
                max_branch=max_branch,
            )
            if not tails:
                tails = [[]]
            for tail in tails:
                path = [candidate, *tail]
                signature = "|".join(_trim(row.get("txn_id")) for row in path if _trim(row.get("txn_id")))
                if signature and signature in seen_signatures:
                    continue
                if signature:
                    seen_signatures.add(signature)
                paths.append(path)
        return paths or [[]]

    def _trace_path_amount_profile(
        self,
        hops: Sequence[dict[str, Any]],
        *,
        seed_account: str,
    ) -> tuple[float, float, float | None] | None:
        amounts = [_trace_amount_value(row) for row in hops]
        if not hops or any(amount is None for amount in amounts):
            return None
        known_amounts = [float(amount) for amount in amounts if amount is not None]
        root_amount = known_amounts[0]
        if len(hops) <= 1:
            return root_amount, root_amount, 1.0 if root_amount > 0 else None
        first_wave = [
            row for row in hops[1:]
            if _trim(row.get("account_key")) == seed_account
        ]
        sink_amount = (
            sum(float(_trace_amount_value(row)) for row in first_wave)
            if first_wave
            else known_amounts[-1]
        )
        if root_amount <= 0:
            return root_amount, sink_amount, None
        pair_scores: list[float] = []
        if first_wave:
            pair_scores.append(max(0.0, 1 - abs(1 - (sink_amount / root_amount))))
        previous_amount = root_amount
        for row in hops[1:]:
            amount = _trace_amount_value(row)
            if amount is None:
                return None
            if previous_amount <= 0:
                return root_amount, sink_amount, None
            pair_scores.append(max(0.0, 1 - abs(1 - (amount / previous_amount))))
            previous_amount = amount
        conservation_score = round(sum(pair_scores) / len(pair_scores), 4) if pair_scores else None
        return root_amount, sink_amount, conservation_score

    def _trace_merge_support_rows(
        self,
        txn_rows: Sequence[dict[str, Any]],
        *,
        target_account: str,
        before_time: Optional[datetime],
        time_window_sec: int,
    ) -> list[dict[str, Any]]:
        if not target_account or before_time is None:
            return []
        window_start = before_time - timedelta(seconds=max(60, int(time_window_sec)))
        rows = [
            row
            for row in txn_rows
            if _trim(row.get("account_key")) == target_account
            and row.get("direction_norm") == "in"
            and row.get("txn_ts") is not None
            and window_start <= row["txn_ts"] <= before_time
        ]
        rows.sort(key=lambda row: (row.get("txn_ts") or datetime.min, _trim(row.get("txn_id"))))
        counterparties = _unique_texts(row.get("counterparty_acct") or row.get("counterparty_name") for row in rows)
        if len(counterparties) < 2:
            return []
        return rows

    def _trace_merge_support_breakdown(
        self,
        support_rows: Sequence[dict[str, Any]],
        *,
        target_amount: float | None,
    ) -> list[dict[str, Any]]:
        allocated_total = _optional_finite_float(target_amount)
        support_amounts = [_trace_amount_value(row) for row in support_rows]
        if (
            allocated_total is None
            or allocated_total < 0
            or not support_rows
            or any(amount is None for amount in support_amounts)
        ):
            return []
        total_amount = sum(float(amount) for amount in support_amounts if amount is not None)
        if total_amount <= 0:
            return []
        breakdown: list[dict[str, Any]] = []
        for row, source_amount in zip(support_rows, support_amounts):
            if source_amount is None:
                return []
            amount = round(source_amount, 2)
            share_rate = round(amount / total_amount, 4)
            allocated_amount = round(min(amount, allocated_total * share_rate), 2)
            breakdown.append(
                {
                    "txn_id": _trim(row.get("txn_id")),
                    "account_key": _trim(row.get("account_key")),
                    "counterparty_acct": _trim(row.get("counterparty_acct")),
                    "counterparty_name": _trim(row.get("counterparty_name")),
                    "amount": amount,
                    "share_rate": share_rate,
                    "allocated_amount": allocated_amount,
                    "explanation": (
                        f"按汇聚前补给占比 {format(share_rate * 100, '.1f')}% 分摊至目标转出 "
                        f"{format(allocated_total, '.2f')} 元。"
                    ),
                }
            )
        return breakdown

    def _trace_cycle_risk_level(
        self,
        *,
        cycle_risk_score: float | None,
        cycle_detected: bool | None,
        round_trip_detected: bool | None,
        merge_detected: bool | None,
    ) -> str:
        score = _optional_finite_float(cycle_risk_score)
        if score is None or any(value is None for value in (cycle_detected, round_trip_detected, merge_detected)):
            return "unresolved"
        if score >= 0.85 or (round_trip_detected and merge_detected):
            return "critical"
        if score >= 0.65 or round_trip_detected:
            return "high"
        if score >= 0.4 or cycle_detected:
            return "medium"
        return "low"

    def _trace_allocation_explanation(
        self,
        *,
        root_amount: float | None,
        sink_amount: float | None,
        amount_gap: float | None,
        conservation_score: float | None,
        merge_support_breakdown: Sequence[dict[str, Any]],
        target_amount: float | None,
    ) -> str:
        numeric_values = [
            _optional_finite_float(root_amount),
            _optional_finite_float(sink_amount),
            _optional_finite_float(amount_gap),
            _optional_finite_float(conservation_score),
            _optional_finite_float(target_amount),
        ]
        if any(value is None for value in numeric_values):
            return "金额或评分字段未核验，无法生成资金分摊说明。"
        if merge_support_breakdown:
            support_amounts = [_optional_finite_float(item.get("amount")) for item in merge_support_breakdown]
            if any(amount is None for amount in support_amounts):
                return "金额或评分字段未核验，无法生成资金分摊说明。"
            support_total = round(sum(float(amount) for amount in support_amounts if amount is not None), 2)
            route_count = len(merge_support_breakdown)
            return (
                f"末跳目标转出 {format(max(target_amount, 0.0), '.2f')} 元；归集前共有 {route_count} 路补给，"
                f"合计 {format(support_total, '.2f')} 元，已按各补给占比拆分分摊。"
            )
        return (
            f"种子金额 {format(max(root_amount, 0.0), '.2f')} 元，路径末端承接 {format(max(sink_amount, 0.0), '.2f')} 元，"
            f"金额缺口 {format(max(amount_gap, 0.0), '.2f')} 元，守恒评分 {format(max(conservation_score, 0.0), '.2f')}。"
        )

    def _trace_replay_steps(self, hops: Sequence[dict[str, Any]]) -> list[dict[str, Any]]:
        if not hops:
            return []
        amounts = [_trace_amount_value(row) for row in hops]
        if any(amount is None for amount in amounts):
            return []
        replay_steps: list[dict[str, Any]] = []
        previous_allocated = float(amounts[0])
        for hop_index, (row, source_amount) in enumerate(zip(hops, amounts), start=1):
            if source_amount is None:
                return []
            amount = round(source_amount, 2)
            if hop_index == 1:
                allocated_amount = amount
                amount_share_rate = 1.0
            else:
                allocated_amount = round(min(previous_allocated, max(amount, 0.0)), 2)
                amount_share_rate = round(allocated_amount / previous_allocated, 4) if previous_allocated > 0 else None
            replay_steps.append(
                {
                    "hop_index": hop_index,
                    "txn_id": _trim(row.get("txn_id")),
                    "account_key": _trim(row.get("account_key")),
                    "counterparty_acct": _trim(row.get("counterparty_acct")),
                    "counterparty_name": _trim(row.get("counterparty_name")),
                    "amount": amount,
                    "allocated_amount": allocated_amount,
                    "amount_share_rate": amount_share_rate,
                    "replay_label": (
                        f"{_trim(row.get('account_key')) or '未知账户'} -> "
                        f"{_trim(row.get('counterparty_name')) or _trim(row.get('counterparty_acct')) or '未知对手'}"
                    ),
                }
            )
            previous_allocated = allocated_amount
        return replay_steps

    def _trace_path_features(
        self,
        hops: Sequence[dict[str, Any]],
        *,
        txn_rows: Sequence[dict[str, Any]],
        seed_account: str,
        time_window_sec: int,
        tolerance_rate: float,
        effective_depth: int,
    ) -> dict[str, Any] | None:
        amount_profile = self._trace_path_amount_profile(hops, seed_account=seed_account)
        if amount_profile is None:
            return None
        root_amount, sink_amount, conservation_score = amount_profile
        if root_amount <= 0 or conservation_score is None:
            return {
                "amount_integrity": "known_zero",
                "root_amount": round(root_amount, 2),
                "sink_amount": round(sink_amount, 2),
                "path_score": None,
            }
        amount_match_rate = round(min(1.5, sink_amount / root_amount), 4)
        time_span_sec = 0
        if hops and hops[0].get("txn_ts") and hops[-1].get("txn_ts"):
            time_span_sec = int(((hops[-1].get("txn_ts") or hops[0]["txn_ts"]) - hops[0]["txn_ts"]).total_seconds())
        visited_accounts: list[str] = []
        cycle_detected = False
        round_trip_detected = False
        split_detected = False
        merge_detected = False
        merge_support_rows: list[dict[str, Any]] = []
        fan_out_counterparties = _unique_texts(
            row.get("counterparty_acct") for row in hops[1:] if _trim(row.get("account_key")) == seed_account
        )
        if len(fan_out_counterparties) >= 2:
            split_detected = True
        for index, row in enumerate(hops):
            account_key = _trim(row.get("account_key"))
            counterparty_acct = _trim(row.get("counterparty_acct"))
            if account_key:
                if visited_accounts and account_key == visited_accounts[-1]:
                    pass
                elif account_key in visited_accounts:
                    cycle_detected = True
                    visited_accounts.append(account_key)
                else:
                    visited_accounts.append(account_key)
            if counterparty_acct:
                if counterparty_acct == seed_account and index > 0:
                    round_trip_detected = True
                if counterparty_acct in visited_accounts:
                    cycle_detected = True
            if index > 1 and account_key != seed_account:
                support_rows = self._trace_merge_support_rows(
                    txn_rows,
                    target_account=account_key,
                    before_time=row.get("txn_ts"),
                    time_window_sec=time_window_sec,
                )
                support_amounts = [_trace_amount_value(item) for item in support_rows]
                row_amount = _trace_amount_value(row)
                if support_rows and (row_amount is None or any(amount is None for amount in support_amounts)):
                    return None
                support_amount = sum(float(amount) for amount in support_amounts if amount is not None)
                if support_rows and row_amount is not None and support_amount >= row_amount * max(0.6, 1 - max(0.0, tolerance_rate)):
                    merge_detected = True
                    merge_support_rows.extend(support_rows)
        if cycle_detected and round_trip_detected:
            path_mode = "round_trip_trace"
        elif cycle_detected:
            path_mode = "cycle_trace"
        elif merge_detected:
            path_mode = "merge_trace"
        elif split_detected:
            path_mode = "split_trace"
        elif len(hops) > 2:
            path_mode = "multi_hop_trace"
        elif len(hops) == 2:
            path_mode = "single_chain"
        else:
            path_mode = "seed_only"
        depth_score = min(len(hops) / max(effective_depth + 1, 2), 1.0)
        time_score = max(0.0, 1 - min(time_span_sec / max(time_window_sec, 1), 1.0))
        risk_bonus = 0.0
        if merge_detected:
            risk_bonus += 0.06
        if split_detected:
            risk_bonus += 0.04
        if cycle_detected:
            risk_bonus += 0.08
        merge_support_amounts = [_trace_amount_value(item) for item in merge_support_rows]
        if any(amount is None for amount in merge_support_amounts):
            return None
        merge_support_amount = sum(float(amount) for amount in merge_support_amounts if amount is not None)
        last_amount = _trace_amount_value(hops[-1]) if hops else None
        if last_amount is None:
            return None
        merge_support_ratio = round(min(2.0, merge_support_amount / max(last_amount, 1.0)), 4)
        cycle_risk_score = _clamp_score(
            (0.55 if cycle_detected else 0.0)
            + (0.2 if round_trip_detected else 0.0)
            + (0.1 if split_detected else 0.0)
            + (0.1 if merge_detected else 0.0)
            + min(max(time_span_sec, 0) / max(time_window_sec, 1), 1.0) * 0.05,
            high=0.99,
        )
        cycle_risk_level = self._trace_cycle_risk_level(
            cycle_risk_score=cycle_risk_score,
            cycle_detected=cycle_detected,
            round_trip_detected=round_trip_detected,
            merge_detected=merge_detected,
        )
        amount_gap = round(abs(root_amount - sink_amount), 2)
        path_score = _clamp_score(
            0.34
            + min(amount_match_rate, 1.0) * 0.2
            + conservation_score * 0.18
            + depth_score * 0.1
            + time_score * 0.1
            + risk_bonus,
            high=0.99,
        )
        return {
            "amount_integrity": "complete",
            "root_amount": round(root_amount, 2),
            "sink_amount": round(sink_amount, 2),
            "amount_match_rate": amount_match_rate,
            "conservation_score": conservation_score,
            "time_span_sec": time_span_sec,
            "path_mode": path_mode,
            "cycle_detected": cycle_detected,
            "round_trip_detected": round_trip_detected,
            "merge_detected": merge_detected,
            "split_detected": split_detected,
            "fan_out_count": len(fan_out_counterparties),
            "merge_support_rows": merge_support_rows,
            "support_txn_ids": _unique_texts(row.get("txn_id") for row in merge_support_rows),
            "merge_support_ratio": merge_support_ratio,
            "cycle_risk_score": cycle_risk_score,
            "cycle_risk_level": cycle_risk_level,
            "amount_gap": amount_gap,
            "path_score": path_score,
        }

    def _trace_hop_entities(self, row: dict[str, Any]) -> tuple[str, str]:
        direction = _trim(row.get("direction_norm"))
        account_entity_id = _trim(row.get("account_entity_id"))
        counterparty_entity_id = _trim(row.get("counterparty_entity_id"))
        if direction == "in":
            return counterparty_entity_id or "unknown_src", account_entity_id or "unknown_dst"
        if direction == "out":
            return account_entity_id or "unknown_src", counterparty_entity_id or "unknown_dst"
        return account_entity_id or "unknown_src", counterparty_entity_id or "unknown_dst"

    def _build_trace_path_summary(self, hops: Sequence[dict[str, Any]], features: Optional[dict[str, Any]] = None) -> str:
        if not hops:
            return self._trace_private_unresolved_summary()
        if any(_trace_amount_value(row) is None for row in hops):
            return self._trace_private_unresolved_summary()
        feature_map = dict(features or {})
        root = hops[0]
        span_sec = 0
        if hops[0].get("txn_ts") and hops[-1].get("txn_ts"):
            span_sec = int(((hops[-1].get("txn_ts") or hops[0]["txn_ts"]) - hops[0]["txn_ts"]).total_seconds())
        mode = _trim(feature_map.get("path_mode"))
        if len(hops) == 1:
            return (
                f"账户 {_trim(root.get('account_key'))} 围绕种子交易 {_trim(root.get('txn_id'))} "
                f"暂未发现后续可穿透链路。"
            )
        if mode == "round_trip_trace":
            merge_hint = "，且中途存在多路汇聚" if _optional_bool(feature_map.get("merge_detected")) is True else ""
            return (
                f"账户 {_trim(root.get('account_key'))} 的种子资金在 {max(1, span_sec // 60)} 分钟内发生回流，"
                f"形成返回原账户或既有账户组的闭环{merge_hint}。"
            )
        if mode == "cycle_trace":
            return (
                f"账户 {_trim(root.get('account_key'))} 的路径存在账户重复访问，"
                f"{max(1, span_sec // 60)} 分钟内形成疑似环流。"
            )
        if mode == "merge_trace":
            return (
                f"资金链路在 {_trim(hops[-1].get('account_key')) or '中间账户'} 出现多路汇聚后再转出，"
                f"整体时间跨度约 {max(1, span_sec // 60)} 分钟。"
            )
        if mode == "split_trace":
            fan_out_count = _optional_positive_int(feature_map.get("fan_out_count"))
            if fan_out_count is None or fan_out_count < 2:
                return self._trace_private_unresolved_summary()
            return (
                f"账户 {_trim(root.get('account_key'))} 的种子资金在 {max(1, span_sec // 60)} 分钟内"
                f"向 {fan_out_count} 个下游对手分散。"
            )
        if len(hops) > 2:
            root_amount = _trace_amount_value(root)
            if root_amount is None:
                return self._trace_private_unresolved_summary()
            return (
                f"账户 {_trim(root.get('account_key'))} 收到 {format(root_amount, '.2f')} 元后，"
                f"{max(1, span_sec // 60)} 分钟内形成 {len(hops) - 1} 跳路径。"
            )
        return (
            f"账户 {_trim(root.get('account_key'))} 的种子资金在 {max(1, span_sec // 60)} 分钟内"
            f"流向 {_trim(hops[-1].get('counterparty_name')) or _trim(hops[-1].get('counterparty_acct')) or '下游账户'}。"
        )

    def run_trace(
        self,
        case_id: str,
        *,
        seed_type: str,
        seed_value: str,
        depth: int,
        time_window_sec: int,
        tolerance_rate: float,
        file_ids: Optional[Sequence[str]] = None,
        trace_id: str = "",
    ) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        self.sync_case_baseline(case_id)
        started = time.perf_counter()
        effective_depth = max(1, int(depth or RULE_PARAM_DEFAULTS["trace_default_depth"]))
        effective_window = max(60, int(time_window_sec or RULE_PARAM_DEFAULTS["trace_default_time_window_sec"]))
        effective_tolerance = max(0.0, _as_float(tolerance_rate))
        engine = self.open_case_engine(case_id)
        owns_engine = True
        try:
            self.ensure_analysis_schema(engine)
            normalized_file_ids = _trim_list(file_ids or [])
            txn_rows = self._load_transaction_rows(engine, case_id, file_ids=normalized_file_ids or None)
            source_signature = _stable_slug(
                "trace",
                self._trace_txn_source_signature(txn_rows),
                _trim(seed_type),
                _trim(seed_value),
                effective_depth,
                effective_window,
                round(effective_tolerance, 4),
                _json_dumps(sorted(normalized_file_ids)),
            )
            trace_key = _trim(trace_id) or _slug("tr")
            existing_trace_rows = self._query_dicts(engine, "SELECT trace_id FROM analysis_trace_run WHERE trace_id=? LIMIT 1", (trace_key,))
            if existing_trace_rows:
                engine.execute(
                    """
                    UPDATE analysis_trace_run
                       SET seed_type=?, seed_value=?, depth=?, time_window_sec=?, tolerance_rate=?, status='running', summary_json=?, updated_at=?
                     WHERE trace_id=? AND case_id=?
                    """,
                    (
                        _trim(seed_type),
                        _trim(seed_value),
                        effective_depth,
                        effective_window,
                        effective_tolerance,
                        "{}",
                        _now_text(),
                        trace_key,
                        case_id,
                    ),
                )
            else:
                engine.execute(
                    """
                    INSERT INTO analysis_trace_run(
                        trace_id, case_id, seed_type, seed_value, depth, time_window_sec, tolerance_rate,
                        status, summary_json, created_at, updated_at
                    ) VALUES (?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        trace_key,
                        case_id,
                        _trim(seed_type),
                        _trim(seed_value),
                        effective_depth,
                        effective_window,
                        effective_tolerance,
                        "running",
                        "{}",
                        _now_text(),
                        _now_text(),
                    ),
                )
            outgoing_by_account: dict[str, list[dict[str, Any]]] = defaultdict(list)
            for row in txn_rows:
                if row.get("direction_norm") == "out" and row.get("txn_ts") is not None:
                    outgoing_by_account[_trim(row.get("account_key"))].append(row)
            for rows in outgoing_by_account.values():
                rows.sort(key=lambda item: (item.get("txn_ts") or datetime.max, _trim(item.get("txn_id"))))

            seed_rows = self._trace_seed_rows(txn_rows, seed_type, seed_value)
            candidate_txn_count = 0
            raw_paths: list[list[dict[str, Any]]] = []
            for seed_row in seed_rows:
                seed_txn_id = _trim(seed_row.get("txn_id"))
                used_txn_ids = {seed_txn_id}
                if seed_row.get("direction_norm") == "in":
                    seed_amount = _trace_amount_value(seed_row)
                    if seed_amount is None or seed_amount <= 0:
                        raw_paths.append([seed_row])
                        continue
                    bundle, considered = self._trace_candidate_out_rows(
                        outgoing_by_account.get(_trim(seed_row.get("account_key")), []),
                        anchor_time=seed_row.get("txn_ts"),
                        anchor_amount=seed_amount,
                        time_window_sec=effective_window,
                        tolerance_rate=effective_tolerance,
                        used_txn_ids=used_txn_ids,
                    )
                    candidate_txn_count += considered
                    if bundle:
                        raw_paths.append([seed_row, *bundle[: max(1, min(effective_depth + 1, 6))]])
                        for next_row in bundle[:3]:
                            chains = self._trace_expand_paths(
                                outgoing_by_account,
                                anchor_row=next_row,
                                depth_remaining=effective_depth - 1,
                                time_window_sec=effective_window,
                                tolerance_rate=effective_tolerance,
                                used_txn_ids={seed_txn_id, _trim(next_row.get("txn_id"))},
                                max_branch=2,
                            )
                            for chain in chains:
                                if chain:
                                    raw_paths.append([seed_row, next_row, *chain])
                                else:
                                    raw_paths.append([seed_row, next_row])
                    else:
                        raw_paths.append([seed_row])
                else:
                    chains = self._trace_expand_paths(
                        outgoing_by_account,
                        anchor_row=seed_row,
                        depth_remaining=effective_depth,
                        time_window_sec=effective_window,
                        tolerance_rate=effective_tolerance,
                        used_txn_ids=used_txn_ids,
                        max_branch=2,
                    )
                    for chain in chains:
                        raw_paths.append([seed_row, *chain] if chain else [seed_row])

            deduped_paths: dict[str, list[dict[str, Any]]] = {}
            for hops in raw_paths:
                signature = "|".join(_trim(row.get("txn_id")) for row in hops if _trim(row.get("txn_id")))
                if signature and signature not in deduped_paths:
                    deduped_paths[signature] = hops
            path_features_by_signature: dict[str, dict[str, Any]] = {}
            ordered_paths: list[list[dict[str, Any]]] = []
            unresolved_amount_path_count = 0
            explicit_zero_path_count = 0
            for hops in deduped_paths.values():
                signature = "|".join(_trim(row.get("txn_id")) for row in hops if _trim(row.get("txn_id")))
                features = self._trace_path_features(
                    hops,
                    txn_rows=txn_rows,
                    seed_account=_trim(hops[0].get("account_key")) if hops else "",
                    time_window_sec=effective_window,
                    tolerance_rate=effective_tolerance,
                    effective_depth=effective_depth,
                )
                if features is None:
                    unresolved_amount_path_count += 1
                    continue
                if features.get("path_score") is None:
                    explicit_zero_path_count += 1
                    continue
                path_features_by_signature[signature] = features
                ordered_paths.append(hops)
            ordered_paths.sort(
                key=lambda hops: (
                    -float(path_features_by_signature["|".join(_trim(row.get("txn_id")) for row in hops if _trim(row.get("txn_id")))]["path_score"]),
                    -float(path_features_by_signature["|".join(_trim(row.get("txn_id")) for row in hops if _trim(row.get("txn_id")))]["amount_match_rate"]),
                    len(hops),
                    _trim(hops[0].get("txn_id")) if hops else "",
                )
            )
            engine.execute("DELETE FROM analysis_trace_path_hop WHERE path_id IN (SELECT path_id FROM analysis_trace_path WHERE trace_id=?)", (trace_key,))
            engine.execute("DELETE FROM analysis_trace_path WHERE trace_id=?", (trace_key,))
            engine.execute("DELETE FROM analysis_evidence_ref WHERE case_id=? AND ref_table='analysis_trace_path' AND ref_pk LIKE ?", (case_id, f"{trace_key}:%"))
            top_path_ids: list[str] = []
            top_score: float | None = None
            best_match_mode = "seed_only"
            mode_counts: dict[str, int] = defaultdict(int)
            feature_counts: dict[str, int] = defaultdict(int)
            ranked_paths = ordered_paths[:12]
            best_ranked_score = (
                _optional_finite_float(
                    path_features_by_signature.get(
                        "|".join(_trim(row.get("txn_id")) for row in ranked_paths[0] if _trim(row.get("txn_id"))),
                        {},
                    ).get("path_score")
                )
                if ranked_paths
                else None
            )
            second_ranked_score = (
                _optional_finite_float(
                    path_features_by_signature.get(
                        "|".join(_trim(row.get("txn_id")) for row in ranked_paths[1] if _trim(row.get("txn_id"))),
                        {},
                    ).get("path_score")
                )
                if len(ranked_paths) > 1
                else None
            )
            for path_index, hops in enumerate(ranked_paths, start=1):
                signature = "|".join(_trim(row.get("txn_id")) for row in hops if _trim(row.get("txn_id")))
                features = path_features_by_signature.get(signature, {})
                hop_amounts = [_trace_amount_value(row) for row in hops]
                amount_match_rate = _optional_finite_float(features.get("amount_match_rate"))
                path_score = _optional_finite_float(features.get("path_score"))
                if any(amount is None for amount in hop_amounts) or amount_match_rate is None or path_score is None:
                    unresolved_amount_path_count += 1
                    continue
                path_id = _stable_slug("path", trace_key, path_index, "|".join(_trim(row.get("txn_id")) for row in hops))
                next_best_path_id = ""
                next_score: float | None = None
                if path_index < len(ranked_paths):
                    next_hops = ranked_paths[path_index]
                    next_signature = "|".join(_trim(row.get("txn_id")) for row in next_hops if _trim(row.get("txn_id")))
                    next_best_path_id = _stable_slug("path", trace_key, path_index + 1, "|".join(_trim(row.get("txn_id")) for row in next_hops))
                    next_score = _optional_finite_float(path_features_by_signature.get(next_signature, {}).get("path_score"))
                time_span_sec = _optional_nonnegative_int(features.get("time_span_sec"))
                if time_span_sec is None:
                    unresolved_amount_path_count += 1
                    continue
                summary_text = self._build_trace_path_summary(hops, features)
                txn_evidence_ids: list[str] = []
                path_txn_ids = _unique_texts(row.get("txn_id") for row in hops)
                for hop_index, (row, source_amount) in enumerate(zip(hops, hop_amounts), start=1):
                    if source_amount is None:
                        raise RuntimeError("trace amount integrity changed before persistence")
                    src_entity_id, dst_entity_id = self._trace_hop_entities(row)
                    title, snippet, payload = self._txn_evidence_payload(row)
                    txn_evidence_ids.append(
                        self._upsert_evidence_ref(
                            engine,
                            case_id=case_id,
                            evidence_type="txn",
                            ref_table="fc_transaction_norm",
                            ref_pk=_trim(row.get("txn_id")),
                            title=title,
                            snippet=snippet,
                            payload=payload,
                        )
                    )
                    engine.execute(
                        """
                        INSERT INTO analysis_trace_path_hop(
                            path_hop_id, path_id, hop_index, txn_id, src_entity_id, dst_entity_id, txn_time, amount, direction, attrs_json
                        ) VALUES (?,?,?,?,?,?,?,?,?,?)
                        """,
                        (
                            _stable_slug("hop", path_id, hop_index, _trim(row.get("txn_id"))),
                            path_id,
                            hop_index,
                            _trim(row.get("txn_id")),
                            src_entity_id,
                            dst_entity_id,
                            _trim(row.get("txn_time")),
                            round(source_amount, 2),
                            _trim(row.get("direction_norm")),
                            _json_dumps(
                                {
                                    "account_key": _trim(row.get("account_key")),
                                    "counterparty_acct": _trim(row.get("counterparty_acct")),
                                    "counterparty_name": _trim(row.get("counterparty_name")),
                                }
                            ),
                        ),
                    )
                support_txn_ids = _unique_texts(features.get("support_txn_ids") or [])
                for support_row in features.get("merge_support_rows", []) or []:
                    title, snippet, payload = self._txn_evidence_payload(support_row)
                    txn_evidence_ids.append(
                        self._upsert_evidence_ref(
                            engine,
                            case_id=case_id,
                            evidence_type="txn",
                            ref_table="fc_transaction_norm",
                            ref_pk=_trim(support_row.get("txn_id")),
                            title=title,
                            snippet=snippet,
                            payload=payload,
                        )
                    )
                target_amount = hop_amounts[-1] if hop_amounts else None
                merge_support_breakdown = self._trace_merge_support_breakdown(
                    features.get("merge_support_rows", []) or [],
                    target_amount=target_amount,
                )
                replay_steps = self._trace_replay_steps(hops)
                allocation_explanation = self._trace_allocation_explanation(
                    root_amount=_optional_finite_float(features.get("root_amount")),
                    sink_amount=_optional_finite_float(features.get("sink_amount")),
                    amount_gap=_optional_finite_float(features.get("amount_gap")),
                    conservation_score=_optional_finite_float(features.get("conservation_score")),
                    merge_support_breakdown=merge_support_breakdown,
                    target_amount=target_amount,
                )
                path_detail = {
                    "path_rank": path_index,
                    "compared_path_count": len(ranked_paths),
                    "next_best_path_id": next_best_path_id,
                    "path_mode": _trim(features.get("path_mode")) or "seed_only",
                    "conservation_score": _optional_rounded_float(features.get("conservation_score"), 4),
                    "cycle_detected": _optional_bool(features.get("cycle_detected")),
                    "round_trip_detected": _optional_bool(features.get("round_trip_detected")),
                    "merge_detected": _optional_bool(features.get("merge_detected")),
                    "split_detected": _optional_bool(features.get("split_detected")),
                    "cycle_risk_score": _optional_rounded_float(features.get("cycle_risk_score"), 4),
                    "cycle_risk_level": _trim(features.get("cycle_risk_level")) or "unresolved",
                    "merge_support_ratio": _optional_rounded_float(features.get("merge_support_ratio"), 4),
                    "amount_gap": _optional_rounded_float(features.get("amount_gap"), 2),
                    "score_gap_to_best": (
                        round(max(best_ranked_score - path_score, 0.0), 4)
                        if best_ranked_score is not None
                        else None
                    ),
                    "score_gap_to_next": (
                        round(max(path_score - next_score, 0.0), 4)
                        if next_score is not None
                        else None
                    ),
                    "root_amount": _optional_rounded_float(features.get("root_amount"), 2),
                    "sink_amount": _optional_rounded_float(features.get("sink_amount"), 2),
                    "fan_out_count": _optional_nonnegative_int(features.get("fan_out_count")),
                    "txn_ids": path_txn_ids,
                    "support_txn_ids": support_txn_ids,
                    "allocation_explanation": allocation_explanation,
                    "replay_steps": replay_steps,
                    "merge_support_breakdown": merge_support_breakdown,
                }
                path_evidence_id = self._upsert_evidence_ref(
                    engine,
                    case_id=case_id,
                    evidence_type="trace_path",
                    ref_table="analysis_trace_path",
                    ref_pk=f"{trace_key}:{path_id}",
                    title=f"资金穿透路径 {path_index}",
                    snippet=summary_text[:240],
                    payload={
                        "trace_id": trace_key,
                        "path_id": path_id,
                        "path_score": path_score,
                        "amount_match_rate": round(amount_match_rate, 4),
                        **path_detail,
                    },
                )
                evidence_ids = _unique_texts([*txn_evidence_ids, path_evidence_id])
                engine.execute(
                    """
                    INSERT INTO analysis_trace_path(
                        path_id, trace_id, case_id, path_index, hop_count, path_score, amount_match_rate,
                        time_span_sec, sink_type, summary, evidence_ids_json, detail_json
                    ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
                    """,
                    (
                        path_id,
                        trace_key,
                        case_id,
                        path_index,
                        len(hops),
                        path_score,
                        round(amount_match_rate, 4),
                        time_span_sec,
                        _trim(features.get("path_mode")) or _trim(hops[-1].get("counterparty_entity_type")) or "account",
                        summary_text,
                        _json_dumps(evidence_ids),
                        _json_dumps(path_detail),
                    ),
                )
                top_path_ids.append(path_id)
                top_score = path_score if top_score is None else max(top_score, path_score)
                mode_counts[_trim(features.get("path_mode")) or "seed_only"] += 1
                if _optional_bool(features.get("round_trip_detected")) is True:
                    feature_counts["round_trip"] += 1
                if _optional_bool(features.get("cycle_detected")) is True:
                    feature_counts["cycle"] += 1
                if _optional_bool(features.get("merge_detected")) is True:
                    feature_counts["merge"] += 1
                if _optional_bool(features.get("split_detected")) is True:
                    feature_counts["split"] += 1
                if len(top_path_ids) == 1:
                    best_match_mode = _trim(features.get("path_mode")) or "seed_only"
            summary = {
                "seed_type": _trim(seed_type),
                "seed_value": _trim(seed_value),
                "path_count": len(top_path_ids),
                "top_score": round(top_score, 2) if top_score is not None else None,
                "best_match_mode": best_match_mode if top_path_ids else "unresolved",
                "best_path_id": top_path_ids[0] if top_path_ids else "",
                "runner_up_path_id": top_path_ids[1] if len(top_path_ids) > 1 else "",
                "source_signature": source_signature,
                "graph_search": True,
            }
            stats = {
                "scanned_txn_count": len(txn_rows),
                "candidate_txn_count": candidate_txn_count,
                "window_sec": effective_window,
                "tolerance_rate": round(effective_tolerance, 4),
                "path_score_spread": (
                    round(max(best_ranked_score - second_ranked_score, 0.0), 4)
                    if best_ranked_score is not None and second_ranked_score is not None
                    else None
                ),
                "ranked_path_count": len(ranked_paths),
                "mode_counts": dict(mode_counts),
                "round_trip_count": mode_counts["round_trip_trace"],
                "cycle_count": mode_counts["cycle_trace"],
                "merge_count": mode_counts["merge_trace"],
                "split_count": mode_counts["split_trace"],
                "round_trip_feature_count": feature_counts["round_trip"],
                "cycle_feature_count": feature_counts["cycle"],
                "merge_feature_count": feature_counts["merge"],
                "split_feature_count": feature_counts["split"],
                "unresolved_amount_path_count": unresolved_amount_path_count,
                "explicit_zero_path_count": explicit_zero_path_count,
            }
            trace_status = "unresolved" if unresolved_amount_path_count or explicit_zero_path_count else "succeeded"
            engine.execute(
                """
                UPDATE analysis_trace_run
                   SET status=?, summary_json=?, updated_at=?
                 WHERE trace_id=? AND case_id=?
                """,
                (
                    trace_status,
                    _json_dumps({"summary": summary, "stats": stats, "top_path_ids": top_path_ids}),
                    _now_text(),
                    trace_key,
                    case_id,
                ),
            )
            self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="trace_fund",
                params={
                    "case_id": case_id,
                    "seed_type": seed_type,
                    "seed_value": seed_value,
                    "depth": effective_depth,
                    "time_window_sec": effective_window,
                    "tolerance_rate": effective_tolerance,
                    "file_ids": normalized_file_ids,
                },
                summary={"trace_id": trace_key, **summary, **stats},
                row_count=len(top_path_ids),
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            result = {
                "trace_id": trace_key,
                "status": trace_status,
                "summary": summary,
                "top_path_ids": top_path_ids,
                "stats": stats,
            }
            return result
        finally:
            if owns_engine:
                engine.close()

    def get_trace(self, trace_id: str, *, case_id: str) -> dict[str, Any]:
        normalized_trace_id = _trim(trace_id)
        normalized_case_id = _trim(case_id)
        if not normalized_trace_id or not normalized_case_id:
            raise KeyError(trace_id)
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_trace_run WHERE trace_id=? AND case_id=? LIMIT 1",
                (normalized_trace_id, normalized_case_id),
            )
            if not rows:
                raise KeyError(trace_id)
            trace_row = rows[0]
            summary_payload = _json_loads(trace_row.get("summary_json"), {})
            path_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_trace_path WHERE trace_id=? AND case_id=? ORDER BY path_score DESC, path_index ASC LIMIT 5",
                (normalized_trace_id, normalized_case_id),
            )
            top_paths: list[dict[str, Any]] = []
            unresolved_readback_count = 0
            for row in path_rows:
                detail_candidate = _json_loads(row.get("detail_json"), {})
                detail = detail_candidate if isinstance(detail_candidate, dict) else {}
                complete = self._trace_path_detail_is_complete(row, detail)
                if not complete:
                    unresolved_readback_count += 1
                top_paths.append(
                    {
                        "path_id": _trim(row.get("path_id")),
                        "path_rank": _optional_positive_int(
                            _first_known_value(detail.get("path_rank"), row.get("path_index"))
                        ),
                        "path_score": _optional_rounded_float(row.get("path_score"), 2),
                        "amount_match_rate": _optional_rounded_float(row.get("amount_match_rate"), 4),
                        "time_span_sec": _optional_rounded_float(row.get("time_span_sec"), 0),
                        "summary": _trim(row.get("summary")) if complete else self._trace_private_unresolved_summary(),
                        "path_mode": _trim(detail.get("path_mode")) if complete else "unresolved",
                        "conservation_score": _optional_rounded_float(detail.get("conservation_score"), 4),
                        "cycle_detected": _optional_bool(detail.get("cycle_detected")),
                        "round_trip_detected": _optional_bool(detail.get("round_trip_detected")),
                        "merge_detected": _optional_bool(detail.get("merge_detected")),
                        "split_detected": _optional_bool(detail.get("split_detected")),
                        "cycle_risk_score": _optional_rounded_float(detail.get("cycle_risk_score"), 4),
                        "cycle_risk_level": _trim(detail.get("cycle_risk_level")) if complete else "unresolved",
                        "merge_support_ratio": _optional_rounded_float(detail.get("merge_support_ratio"), 4),
                        "amount_gap": _optional_rounded_float(detail.get("amount_gap"), 2),
                        "score_gap_to_best": _optional_rounded_float(detail.get("score_gap_to_best"), 4),
                        "score_gap_to_next": _optional_rounded_float(detail.get("score_gap_to_next"), 4),
                        "support_txn_ids": _unique_texts(detail.get("support_txn_ids") or []) if complete else [],
                        "evidence_ids": _json_loads(row.get("evidence_ids_json"), []) if complete else [],
                    }
                )
            summary_value = summary_payload.get("summary", {}) if isinstance(summary_payload, dict) else {}
            stats_value = summary_payload.get("stats", {}) if isinstance(summary_payload, dict) else {}
            if unresolved_readback_count:
                summary_value = {
                    "coverage_status": "unresolved",
                    "verified_path_count": len(path_rows) - unresolved_readback_count,
                }
                stats_value = {
                    "coverage_status": "unresolved",
                    "unresolved_path_count": unresolved_readback_count,
                }
            return {
                "trace_id": normalized_trace_id,
                "case_id": normalized_case_id,
                "status": _trim(trace_row.get("status")) or "queued",
                "summary": summary_value,
                "stats": stats_value,
                "top_paths": top_paths,
            }
        finally:
            engine.close()

    def get_trace_path(self, trace_id: str, path_id: str, *, case_id: str) -> dict[str, Any]:
        normalized_trace_id = _trim(trace_id)
        normalized_path_id = _trim(path_id)
        normalized_case_id = _trim(case_id)
        if not normalized_trace_id or not normalized_path_id or not normalized_case_id:
            raise KeyError(path_id or trace_id)
        engine = self.open_case_engine(normalized_case_id)
        try:
            self.ensure_analysis_schema(engine)
            path_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_trace_path WHERE trace_id=? AND path_id=? AND case_id=? LIMIT 1",
                (normalized_trace_id, normalized_path_id, normalized_case_id),
            )
            if not path_rows:
                raise KeyError(path_id)
            path_row = path_rows[0]
            hop_rows = self._query_dicts(
                engine,
                "SELECT * FROM analysis_trace_path_hop WHERE path_id=? ORDER BY hop_index ASC",
                (normalized_path_id,),
            )
            path_detail = _json_loads(path_row.get("detail_json"), {})
            if not isinstance(path_detail, dict):
                path_detail = {}
            complete = self._trace_path_detail_is_complete(path_row, path_detail, hop_rows=hop_rows)
            support_txn_ids = path_detail.get("support_txn_ids") if isinstance(path_detail.get("support_txn_ids"), list) else _json_loads(path_detail.get("support_txn_ids"), [])
            evidence_ids = _json_loads(path_row.get("evidence_ids_json"), []) if complete else []
            mixed_fund_tracking = (
                self._trace_path_mixed_fund_tracking(normalized_case_id, path_detail=path_detail)
                if complete
                else {}
            )
            return {
                "path_id": normalized_path_id,
                "path_rank": _optional_positive_int(
                    _first_known_value(path_detail.get("path_rank"), path_row.get("path_index"))
                ),
                "compared_path_count": _optional_positive_int(path_detail.get("compared_path_count")),
                "next_best_path_id": _trim(path_detail.get("next_best_path_id")),
                "path_score": _optional_rounded_float(path_row.get("path_score"), 2),
                "amount_match_rate": _optional_rounded_float(path_row.get("amount_match_rate"), 4),
                "time_span_sec": _optional_rounded_float(path_row.get("time_span_sec"), 0),
                "summary": _trim(path_row.get("summary")) if complete else self._trace_private_unresolved_summary(),
                "path_mode": _trim(path_detail.get("path_mode")) if complete else "unresolved",
                "conservation_score": _optional_rounded_float(path_detail.get("conservation_score"), 4),
                "cycle_detected": _optional_bool(path_detail.get("cycle_detected")),
                "round_trip_detected": _optional_bool(path_detail.get("round_trip_detected")),
                "merge_detected": _optional_bool(path_detail.get("merge_detected")),
                "split_detected": _optional_bool(path_detail.get("split_detected")),
                "cycle_risk_score": _optional_rounded_float(path_detail.get("cycle_risk_score"), 4),
                "cycle_risk_level": _trim(path_detail.get("cycle_risk_level")) if complete else "unresolved",
                "merge_support_ratio": _optional_rounded_float(path_detail.get("merge_support_ratio"), 4),
                "amount_gap": _optional_rounded_float(path_detail.get("amount_gap"), 2),
                "score_gap_to_best": _optional_rounded_float(path_detail.get("score_gap_to_best"), 4),
                "score_gap_to_next": _optional_rounded_float(path_detail.get("score_gap_to_next"), 4),
                "support_txn_ids": _unique_texts(support_txn_ids or []) if complete else [],
                "allocation_explanation": (
                    _trim(path_detail.get("allocation_explanation"))
                    if complete
                    else self._trace_private_unresolved_summary()
                ),
                "replay_steps": list(path_detail.get("replay_steps") or []) if complete else [],
                "merge_support_breakdown": list(path_detail.get("merge_support_breakdown") or []) if complete else [],
                "hops": [
                    {
                        "hop_index": _optional_positive_int(row.get("hop_index")),
                        "txn_id": _trim(row.get("txn_id")),
                        "src_entity_id": _trim(row.get("src_entity_id")),
                        "dst_entity_id": _trim(row.get("dst_entity_id")),
                        "txn_time": _trim(row.get("txn_time")),
                        "amount": _optional_rounded_float(row.get("amount"), 2),
                        "direction": _trim(row.get("direction")),
                    }
                    for row in hop_rows
                ],
                "evidence_ids": evidence_ids,
                "evidence_refs": self._evidence_ref_rows_for_ids(engine, normalized_case_id, evidence_ids) if complete else [],
                "mixed_fund_tracking": mixed_fund_tracking,
            }
        finally:
            engine.close()

    def _trace_path_mixed_fund_tracking(self, case_id: str, *, path_detail: Mapping[str, Any] | None) -> dict[str, Any]:
        detail = dict(path_detail or {})
        seed_txn_id = _trim(detail.get("seed_txn_id"))
        txn_ids = _unique_texts(detail.get("txn_ids") or [])
        if not seed_txn_id and txn_ids:
            seed_txn_id = txn_ids[0]
        if not seed_txn_id:
            return {}
        candidate_txn_ids = _unique_texts(
            [
                *list(detail.get("support_txn_ids") or []),
                *txn_ids[1:],
            ]
        )
        tracking = self.get_seed_mixed_fund_tracking(
            case_id,
            seed_txn_id=seed_txn_id,
            candidate_txn_ids=candidate_txn_ids,
        )
        if _trim(tracking.get("semantic_status")) == "unresolved":
            return tracking
        return enrich_mixed_fund_tracking(tracking, scope_source="db")

    def get_case_top_mixed_fund_tracking(self, case_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        # The former path depended on an unversioned report helper and treated
        # an empty read as a factual no-hit. A case-level conclusion now stays
        # closed until the publication pipeline supplies a current receipt.
        return self._mixed_fund_unresolved("", "host_evidence_receipt_required")

    def _trace_path_preview_payload(
        self,
        engine: DuckDBEngine,
        case_id: str,
        path_id: str,
    ) -> Optional[dict[str, Any]]:
        rows = self._query_dicts(
            engine,
            "SELECT * FROM analysis_trace_path WHERE case_id=? AND path_id=? LIMIT 1",
            (case_id, _trim(path_id)),
        )
        if not rows:
            return None
        row = rows[0]
        detail = _json_loads(row.get("detail_json"), {})
        if not isinstance(detail, dict):
            detail = {}
        complete = self._trace_path_detail_is_complete(row, detail)
        evidence_ids = _unique_texts(_json_loads(row.get("evidence_ids_json"), [])) if complete else []
        return {
            "path_id": _trim(row.get("path_id")),
            "trace_id": _trim(row.get("trace_id")),
            "path_rank": _optional_positive_int(
                _first_known_value(detail.get("path_rank"), row.get("path_index"))
            ),
            "hop_count": _optional_nonnegative_int(row.get("hop_count")),
            "path_score": _optional_rounded_float(row.get("path_score"), 4),
            "amount_match_rate": _optional_rounded_float(row.get("amount_match_rate"), 4),
            "time_span_sec": _optional_rounded_float(row.get("time_span_sec"), 0),
            "summary": _trim(row.get("summary")) if complete else self._trace_private_unresolved_summary(),
            "path_mode": _trim(detail.get("path_mode")) if complete else "unresolved",
            "support_txn_ids": _unique_texts(detail.get("support_txn_ids") or []) if complete else [],
            "evidence_ids": evidence_ids,
            "evidence_refs": self._evidence_ref_rows_for_ids(engine, case_id, evidence_ids[:8]),
            "created_at": "",
        }

    def get_llm_reference_preview(self, case_id: str, ref_type: str, ref_id: str) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        normalized_type = _trim(ref_type).lower()
        normalized_ref_id = _trim(ref_id)
        if not normalized_ref_id:
            raise KeyError(ref_id)
        engine = self.open_case_engine(case_id)
        try:
            self.ensure_analysis_schema(engine)
            if normalized_type == "query":
                rows = self._query_dicts(
                    engine,
                    "SELECT * FROM analysis_query_log WHERE case_id=? AND query_id=? LIMIT 1",
                    (case_id, normalized_ref_id),
                )
                if not rows:
                    legacy_rows = self._query_dicts(
                        engine,
                        "SELECT * FROM analysis_query_log WHERE case_id=? ORDER BY created_at DESC LIMIT 5000",
                        (case_id,),
                    )
                    rows = [
                        row
                        for row in legacy_rows
                        if normalize_case_sql_query_ref(
                            case_id=case_id,
                            query_id=row.get("query_id"),
                        )
                        == normalized_ref_id
                    ][:1]
                if not rows:
                    raise KeyError(normalized_ref_id)
                row = rows[0]
                if _trim(row.get("case_id")) != _trim(case_id):
                    raise KeyError(normalized_ref_id)
                history = project_workbench_history_item(
                    case_id=case_id,
                    row={
                        **row,
                        "params_json": _json_loads(row.get("params_json"), {}),
                        "summary_json": _json_loads(row.get("summary_json"), {}),
                    },
                    host_registry_member=True,
                )
                return {
                    "ref_type": "query",
                    "ref_id": history["query_ref"],
                    "title": "查询诊断",
                    "subtitle": history["tool_category"],
                    "summary": "该记录仅用于操作诊断，不能作为案件事实或引用依据。",
                    "created_at": project_diagnostic_timestamp(row.get("created_at")),
                    "data": history,
                }
            if normalized_type == "evidence":
                rows = self._query_dicts(
                    engine,
                    "SELECT * FROM analysis_evidence_ref WHERE case_id=? AND evidence_id=? LIMIT 1",
                    (case_id, normalized_ref_id),
                )
                if not rows:
                    raise KeyError(normalized_ref_id)
                row = rows[0]
                payload = _json_loads(row.get("payload_json"), {})
                return {
                    "ref_type": "evidence",
                    "ref_id": normalized_ref_id,
                    "title": _trim(row.get("title")) or normalized_ref_id,
                    "subtitle": f"{_trim(row.get('evidence_type')) or 'evidence'} · {_trim(row.get('ref_table'))}:{_trim(row.get('ref_pk'))}",
                    "summary": _trim(row.get("snippet")),
                    "created_at": _trim(row.get("created_at")),
                    "data": {
                        "evidence_type": _trim(row.get("evidence_type")),
                        "ref_table": _trim(row.get("ref_table")),
                        "ref_pk": _trim(row.get("ref_pk")),
                        "snippet": _trim(row.get("snippet")),
                        "payload": payload,
                    },
                }
            if normalized_type == "path":
                payload = self._trace_path_preview_payload(engine, case_id, normalized_ref_id)
                if not payload:
                    raise KeyError(normalized_ref_id)
                return {
                    "ref_type": "path",
                    "ref_id": normalized_ref_id,
                    "title": f"路径 {normalized_ref_id}",
                    "subtitle": f"评分 {payload['path_score']} · {payload['hop_count']} 跳",
                    "summary": payload.get("summary") or "资金穿透路径结果。",
                    "created_at": "",
                    "data": payload,
                }
        finally:
            engine.close()
        raise KeyError(normalized_ref_id)

    def _validate_small_refresh_persistence(
        self,
        engine: DuckDBEngine,
        case_id: str,
        *,
        rule_hit_count: int,
        graph_stats: Mapping[str, int],
    ) -> None:
        required_tables = (
            "analysis_rule_hit",
            "analysis_entity_node",
            "analysis_relation_edge",
            "analysis_evidence_ref",
        )
        if not all(self._table_exists(engine, table_name) for table_name in required_tables):
            raise AnalysisGraphContractError()
        rows = engine.query(
            """
            SELECT
              (SELECT COUNT(1) FROM analysis_rule_hit WHERE case_id=?),
              (SELECT COUNT(1) FROM analysis_entity_node WHERE case_id=?),
              (SELECT COUNT(1) FROM analysis_entity_node WHERE case_id=? AND entity_type='account'),
              (SELECT COUNT(1) FROM analysis_relation_edge WHERE case_id=?),
              (SELECT COUNT(1) FROM analysis_relation_edge
                WHERE case_id=? AND txn_count>0 AND amount_sum IS NULL),
              (SELECT COUNT(1) FROM analysis_relation_edge AS edge
                WHERE edge.case_id=? AND (
                  NOT EXISTS (
                    SELECT 1 FROM analysis_entity_node AS src
                    WHERE src.case_id=edge.case_id AND src.entity_id=edge.src_entity_id
                  )
                  OR NOT EXISTS (
                    SELECT 1 FROM analysis_entity_node AS dst
                    WHERE dst.case_id=edge.case_id AND dst.entity_id=edge.dst_entity_id
                  )
                )),
              (SELECT COUNT(1) FROM analysis_relation_edge
                WHERE case_id=? AND relation_type='account_to_account_transfer' AND txn_count>0),
              (SELECT COALESCE(SUM(txn_count), 0) FROM analysis_relation_edge
                WHERE case_id=? AND relation_type='account_to_account_transfer')
            """,
            (case_id,) * 8,
        )
        row = _exact_single_query_row(rows, width=8)
        if row is None:
            raise AnalysisGraphContractError()
        counts = tuple(_exact_nonnegative_db_int(value) for value in row)
        if any(value is None for value in counts):
            raise AnalysisGraphContractError()
        (
            persisted_rule_count,
            persisted_node_count,
            persisted_account_count,
            persisted_edge_count,
            unresolved_edge_count,
            dangling_edge_count,
            transfer_edge_count,
            transfer_txn_count,
        ) = counts
        if (
            persisted_rule_count != rule_hit_count
            or persisted_node_count != graph_stats["node_count"]
            or persisted_account_count != graph_stats["account_node_count"]
            or persisted_edge_count != graph_stats["edge_count"]
            or unresolved_edge_count != graph_stats["unresolved_amount_edge_count"]
            or dangling_edge_count != 0
            or transfer_edge_count <= 0
            or transfer_edge_count > persisted_edge_count
            or transfer_txn_count != graph_stats["txn_count"]
        ):
            raise AnalysisGraphContractError()

        rule_rows = engine.query(
            "SELECT rule_hit_id, evidence_ids_json FROM analysis_rule_hit WHERE case_id=? ORDER BY rule_hit_id",
            (case_id,),
        )
        registry_rows = engine.query(
            "SELECT evidence_id, ref_table, ref_pk FROM analysis_evidence_ref WHERE case_id=? ORDER BY evidence_id",
            (case_id,),
        )
        if type(rule_rows) is not list or type(registry_rows) is not list:
            raise AnalysisGraphContractError()
        registry: dict[str, tuple[str, str]] = {}
        for registry_row in registry_rows:
            if type(registry_row) not in (list, tuple) or len(registry_row) != 3:
                raise AnalysisGraphContractError()
            evidence_id, ref_table, ref_pk = registry_row
            if (
                type(evidence_id) is not str
                or not evidence_id
                or evidence_id != evidence_id.strip()
                or type(ref_table) is not str
                or not ref_table
                or ref_table != ref_table.strip()
                or type(ref_pk) is not str
                or not ref_pk
                or ref_pk != ref_pk.strip()
                or evidence_id in registry
            ):
                raise AnalysisGraphContractError()
            registry[evidence_id] = (ref_table, ref_pk)

        seen_rule_ids: set[str] = set()
        for rule_row in rule_rows:
            if type(rule_row) not in (list, tuple) or len(rule_row) != 2:
                raise AnalysisGraphContractError()
            rule_hit_id, evidence_ids_json = rule_row
            if (
                type(rule_hit_id) is not str
                or not rule_hit_id
                or rule_hit_id != rule_hit_id.strip()
                or rule_hit_id in seen_rule_ids
                or type(evidence_ids_json) is not str
            ):
                raise AnalysisGraphContractError()
            seen_rule_ids.add(rule_hit_id)
            try:
                evidence_ids = json.loads(evidence_ids_json)
            except (TypeError, ValueError):
                raise AnalysisGraphContractError() from None
            if (
                type(evidence_ids) is not list
                or not evidence_ids
                or any(
                    type(evidence_id) is not str
                    or not evidence_id
                    or evidence_id != evidence_id.strip()
                    for evidence_id in evidence_ids
                )
                or len(set(evidence_ids)) != len(evidence_ids)
                or any(evidence_id not in registry for evidence_id in evidence_ids)
                or not any(
                    registry[evidence_id] == ("analysis_rule_hit", rule_hit_id)
                    for evidence_id in evidence_ids
                )
            ):
                raise AnalysisGraphContractError()
        if len(rule_rows) != rule_hit_count or len(seen_rule_ids) != rule_hit_count:
            raise AnalysisGraphContractError()

    def _refresh_large_case_analysis_summary(
        self,
        case_id: str,
        *,
        txn_count: int,
        force_refresh: bool,
        native_rule_params: Mapping[str, Any],
        native_rule_txn_index_ready: bool,
        native_rule_pattern_index_ready: bool,
        daily_agg_ready: bool,
        started: float,
    ) -> dict[str, Any]:
        unknown_stats = {
            "txn_count": None,
            "node_count": None,
            "edge_count": None,
            "account_count": None,
            "counterparty_count": None,
            "rule_index_count": None,
            "rule_pattern_count": None,
        }

        def blocked(status: str, blocker: str) -> dict[str, Any]:
            return {
                "case_id": case_id,
                "status": status,
                "refreshed": [],
                "stats": dict(unknown_stats),
                "blocker": blocker,
            }

        if not (
            native_rule_txn_index_ready is True
            and native_rule_pattern_index_ready is True
            and daily_agg_ready is True
        ):
            return blocked("dependency_unavailable", "analysis_materialization_dependency_unavailable")

        engine = self.open_case_engine(case_id)
        in_transaction = False

        def rollback_and_block(status: str, blocker: str) -> dict[str, Any]:
            nonlocal in_transaction
            if in_transaction:
                try:
                    engine.execute("ROLLBACK")
                except Exception as exc:
                    in_transaction = False
                    raise RuntimeError("analysis_refresh_rollback_failed") from exc
                in_transaction = False
            return blocked(status, blocker)

        def exact_scalar_count(sql: str, params: Sequence[Any] = ()) -> int | None:
            rows = engine.query(sql, tuple(params))
            row = _exact_single_query_row(rows, width=1)
            if row is None:
                return None
            return _exact_nonnegative_db_int(row[0])

        try:
            engine.execute("BEGIN TRANSACTION")
            in_transaction = True

            current_txn_count = self._analysis_refresh_txn_count_from_engine(engine, case_id)
            if current_txn_count is None:
                return rollback_and_block("source_unavailable", "transaction_source_unavailable")
            if current_txn_count != txn_count:
                return rollback_and_block("partial", "transaction_count_mismatch")

            required_tables = (
                "analysis_txn_detail_idx",
                "analysis_account_dim",
                "analysis_txn_daily_agg",
                "analysis_rule_txn_idx",
                "analysis_rule_pattern_idx",
            )
            if not all(self._table_exists(engine, table_name) for table_name in required_tables):
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_materialization_dependency_unavailable",
                )
            if not self._rule_txn_index_meta_ready(engine, case_id):
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_rule_txn_index_unverified",
                )

            expected_pattern_signature = self._native_rule_pattern_param_signature(native_rule_params)
            native_pattern_features = self._load_native_rule_pattern_features(
                engine,
                case_id,
                params=native_rule_params,
            )
            if (
                type(native_pattern_features) is not dict
                or set(native_pattern_features) != {"ready", "param_signature", "by_account"}
                or native_pattern_features.get("ready") is not True
                or native_pattern_features.get("param_signature") != expected_pattern_signature
                or not isinstance(native_pattern_features.get("by_account"), Mapping)
            ):
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_rule_pattern_index_unverified",
                )

            try:
                amount_coverage = self._daily_agg.require_complete_amount_coverage(
                    case_id=case_id,
                    engine=engine,
                    ensure_ready=False,
                )
            except TxnAmountCoverageIncompleteError:
                return rollback_and_block(
                    "partial",
                    "transaction_amount_coverage_incomplete",
                )
            coverage_counts = (
                _exact_nonnegative_db_int(amount_coverage.total_rows),
                _exact_nonnegative_db_int(amount_coverage.amount_valid_rows),
                _exact_nonnegative_db_int(amount_coverage.amount_missing_rows),
                _exact_nonnegative_db_int(amount_coverage.amount_parse_failed_rows),
                _exact_nonnegative_db_int(amount_coverage.direction_covered_rows),
            ) if type(amount_coverage) is TxnAmountCoverageV1 else (None, None, None, None, None)
            if (
                type(amount_coverage) is not TxnAmountCoverageV1
                or amount_coverage.case_id != case_id
                or amount_coverage.contract != "TxnAmountCoverageV1"
                or type(amount_coverage.materialization_identity) is not str
                or re.fullmatch(
                    r"txn_daily_snapshot:v12:[0-9a-f]{64}",
                    amount_coverage.materialization_identity,
                )
                is None
                or any(value is None for value in coverage_counts)
                or coverage_counts
                != (
                    current_txn_count,
                    current_txn_count,
                    0,
                    0,
                    current_txn_count,
                )
            ):
                return rollback_and_block(
                    "partial",
                    "transaction_amount_coverage_incomplete",
                )

            detail_count = exact_scalar_count("SELECT COUNT(1) FROM analysis_txn_detail_idx")
            account_count = exact_scalar_count("SELECT COUNT(1) FROM analysis_account_dim")
            aggregate_rows = engine.query(
                """
                SELECT
                    COUNT(DISTINCT acct_key),
                    COUNT(DISTINCT cp_key),
                    COUNT(1),
                    SUM(txn_count)
                FROM analysis_txn_daily_agg
                WHERE acct_key IS NOT NULL AND TRIM(acct_key)<>''
                """,
            )
            rule_index_count = exact_scalar_count(
                "SELECT COUNT(1) FROM analysis_rule_txn_idx WHERE case_id=?",
                (case_id,),
            )
            pattern_index_count = exact_scalar_count(
                "SELECT COUNT(1) FROM analysis_rule_pattern_idx WHERE case_id=? AND param_signature=?",
                (case_id, expected_pattern_signature),
            )
            aggregate_row = _exact_single_query_row(aggregate_rows, width=4)
            if aggregate_row is None:
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_materialization_count_unavailable",
                )

            normalized_txn_count = _exact_nonnegative_db_int(txn_count)
            aggregate_account_count = _exact_nonnegative_db_int(aggregate_row[0])
            counterparty_count = _exact_nonnegative_db_int(aggregate_row[1])
            daily_edge_count = _exact_nonnegative_db_int(aggregate_row[2])
            daily_txn_count = _exact_nonnegative_db_int(aggregate_row[3])
            observed = (
                normalized_txn_count,
                detail_count,
                account_count,
                aggregate_account_count,
                counterparty_count,
                daily_edge_count,
                daily_txn_count,
                rule_index_count,
                pattern_index_count,
            )
            if any(value is None for value in observed):
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_materialization_count_unavailable",
                )
            node_count = _exact_nonnegative_db_int(account_count + counterparty_count)
            if node_count is None:
                return rollback_and_block(
                    "dependency_unavailable",
                    "analysis_materialization_count_unavailable",
                )
            if (
                normalized_txn_count <= 0
                or detail_count != normalized_txn_count
                or daily_txn_count != normalized_txn_count
                or rule_index_count != normalized_txn_count
                or account_count <= 0
                or account_count != aggregate_account_count
                or daily_edge_count <= 0
                or daily_edge_count > normalized_txn_count
            ):
                return rollback_and_block(
                    "partial",
                    "analysis_materialization_count_mismatch",
                )

            stats = {
                "txn_count": normalized_txn_count,
                "node_count": node_count,
                "edge_count": daily_edge_count,
                "account_count": account_count,
                "counterparty_count": counterparty_count,
                "rule_index_count": rule_index_count,
                "rule_pattern_count": pattern_index_count,
                "daily_agg_ready": True,
                "native_rule_txn_index_ready": True,
                "native_rule_pattern_index_ready": True,
                "mode": "large_case_materialized_summary",
                "limit": int(self.LARGE_REFRESH_TXN_ROW_LIMIT),
            }
            self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="refresh_case_analysis",
                params={
                    "case_id": case_id,
                    "force_refresh": bool(force_refresh),
                    "mode": "large_case_materialized_summary",
                },
                summary=stats,
                row_count=1,
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            engine.execute("COMMIT")
            in_transaction = False
            return {
                "case_id": case_id,
                "status": "succeeded",
                "refreshed": ["rule_indexes", "daily_aggregates", "analysis_summary"],
                "stats": stats,
            }
        except Exception:
            if in_transaction:
                try:
                    engine.execute("ROLLBACK")
                except Exception as exc:
                    in_transaction = False
                    raise RuntimeError("analysis_refresh_rollback_failed") from exc
                in_transaction = False
            raise
        finally:
            engine.close()

    def refresh_case_analysis(self, case_id: str, *, force_refresh: bool = False) -> dict[str, Any]:
        if not self.case_exists(case_id):
            raise KeyError(case_id)
        started = time.perf_counter()
        txn_count = self._analysis_refresh_txn_count(case_id)
        if txn_count is None:
            return {
                "case_id": case_id,
                "status": "source_unavailable",
                "refreshed": [],
                "stats": {
                    "rule_hit_count": None,
                    "node_count": None,
                    "edge_count": None,
                    "txn_count": None,
                },
                "blocker": "transaction_source_unavailable",
            }
        if txn_count == 0:
            return {
                "case_id": case_id,
                "status": "empty_unverified",
                "refreshed": [],
                "stats": {
                    "rule_hit_count": None,
                    "node_count": None,
                    "edge_count": None,
                    "txn_count": None,
                },
                "blocker": "transaction_source_empty_unverified",
            }
        if txn_count >= self.LARGE_REFRESH_TXN_ROW_LIMIT:
            native_rule_params = self._load_rule_params_for_native(case_id)
            native_rule_txn_index_ready = self._prepare_rule_txn_index_native(case_id, force=force_refresh)
            if native_rule_txn_index_ready is not True:
                return self._refresh_large_case_analysis_summary(
                    case_id,
                    txn_count=txn_count,
                    force_refresh=force_refresh,
                    native_rule_params=native_rule_params,
                    native_rule_txn_index_ready=False,
                    native_rule_pattern_index_ready=False,
                    daily_agg_ready=False,
                    started=started,
                )
            native_rule_pattern_index_ready = self._prepare_rule_pattern_index_native(
                case_id,
                params=native_rule_params,
                force=force_refresh,
            )
            if native_rule_pattern_index_ready is not True:
                return self._refresh_large_case_analysis_summary(
                    case_id,
                    txn_count=txn_count,
                    force_refresh=force_refresh,
                    native_rule_params=native_rule_params,
                    native_rule_txn_index_ready=True,
                    native_rule_pattern_index_ready=False,
                    daily_agg_ready=False,
                    started=started,
                )
            daily_agg_ready = self._daily_agg.ensure_materialized(case_id, force=force_refresh, engine=None)
            return self._refresh_large_case_analysis_summary(
                case_id,
                txn_count=txn_count,
                force_refresh=force_refresh,
                native_rule_params=native_rule_params,
                native_rule_txn_index_ready=native_rule_txn_index_ready,
                native_rule_pattern_index_ready=native_rule_pattern_index_ready,
                daily_agg_ready=daily_agg_ready,
                started=started,
            )
        engine = self.open_case_engine(case_id)
        in_transaction = False

        def rollback_current_refresh() -> None:
            nonlocal in_transaction
            if not in_transaction:
                return
            try:
                engine.execute("ROLLBACK")
            except Exception as exc:
                in_transaction = False
                raise RuntimeError("analysis_refresh_rollback_failed") from exc
            in_transaction = False

        try:
            engine.execute("BEGIN TRANSACTION")
            in_transaction = True
            current_txn_count = self._analysis_refresh_txn_count_from_engine(engine, case_id)
            if current_txn_count is None:
                rollback_current_refresh()
                return {
                    "case_id": case_id,
                    "status": "source_unavailable",
                    "refreshed": [],
                    "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                    "blocker": "transaction_source_unavailable",
                }
            if current_txn_count != txn_count:
                rollback_current_refresh()
                return {
                    "case_id": case_id,
                    "status": "partial",
                    "refreshed": [],
                    "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                    "blocker": "transaction_count_mismatch",
                }
            txn_rows = self._load_transaction_rows(
                engine,
                case_id,
                prefer_rule_txn_index=False,
            )
            input_coverage = _rule_input_coverage(txn_rows)
            if len(txn_rows) != current_txn_count:
                rollback_current_refresh()
                return {
                    "case_id": case_id,
                    "status": "partial",
                    "refreshed": [],
                    "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                    "blocker": "transaction_count_mismatch",
                }
            if input_coverage.get("status") != "complete":
                rollback_current_refresh()
                return {
                    "case_id": case_id,
                    "status": "partial",
                    "refreshed": [],
                    "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                    "blocker": "rule_input_coverage_unresolved",
                }

            self.ensure_analysis_schema(engine)
            rule_hits = self._ensure_rule_hits(
                engine,
                case_id,
                txn_rows,
                force_refresh=force_refresh,
                native_pattern_features=None,
            )
            graph_stats = self._ensure_materialized_graph(engine, case_id, txn_rows, force_refresh=force_refresh)
            validated_graph_stats = _validated_graph_materialization_stats(
                graph_stats,
                expected_txn_count=current_txn_count,
            )
            rule_hit_count = _exact_nonnegative_db_int(len(rule_hits)) if type(rule_hits) is list else None
            if rule_hit_count is None:
                raise AnalysisGraphContractError()
            self._validate_small_refresh_persistence(
                engine,
                case_id,
                rule_hit_count=rule_hit_count,
                graph_stats=validated_graph_stats,
            )
            result = {
                "case_id": case_id,
                "status": "succeeded",
                "refreshed": ["rule_hits", "entity_graph"],
                "stats": {
                    "rule_hit_count": rule_hit_count,
                    "node_count": validated_graph_stats["node_count"],
                    "edge_count": validated_graph_stats["edge_count"],
                    "txn_count": validated_graph_stats["txn_count"],
                },
            }
            self._append_query_log(
                engine,
                case_id=case_id,
                tool_name="refresh_case_analysis",
                params={"case_id": case_id, "force_refresh": bool(force_refresh)},
                summary=result["stats"],
                row_count=1,
                duration_ms=int((time.perf_counter() - started) * 1000),
            )
            engine.execute("COMMIT")
            in_transaction = False
            return result
        except AnalysisGraphContractError:
            rollback_current_refresh()
            return {
                "case_id": case_id,
                "status": "partial",
                "refreshed": [],
                "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                "blocker": "analysis_graph_contract_invalid",
            }
        except TransactionFactSourceUnavailableError:
            rollback_current_refresh()
            return {
                "case_id": case_id,
                "status": "source_unavailable",
                "refreshed": [],
                "stats": {"rule_hit_count": None, "node_count": None, "edge_count": None, "txn_count": None},
                "blocker": "transaction_source_unavailable",
            }
        except Exception:
            rollback_current_refresh()
            raise
        finally:
            engine.close()
