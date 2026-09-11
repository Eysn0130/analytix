from __future__ import annotations

import copy
import hashlib
import json
import logging
import math
import os
import threading
import time
from datetime import datetime
from pathlib import Path
from typing import Any, Optional, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.safe_observability import log_closed_diagnostic
from app.core.analysis_compute import (
    query_chart_dashboard,
    query_stats_date_range,
    query_stats_rows,
    query_stats_rows_to_file,
    query_stats_txn_rows,
    query_stats_txn_rows_to_file,
)
from app.core.storage import CaseStorage
from app.domain.ordinary_diagnostic_projection import (
    project_query_log_diagnostic,
    project_query_tool_name,
)
from app.repositories.analysis_revision import bump_stats_flow_source_revision, read_stats_flow_source_revision
from app.repositories.txn_daily_aggregate import TxnDailyAggregateStore
from app.repositories.txn_daily_materialization_plan_constants import ACCOUNT_DIM_TABLE, AGG_TABLE, DETAIL_TABLE

_STATS_DASHBOARD_FLOW_ITEM_LIMIT = 20
_STATS_DASHBOARD_COUNTERPARTY_ITEM_LIMIT = 50
_LOGGER = logging.getLogger("analytix.data_analysis.stats_repository")


class _RepositoryTiming:
    def __init__(self, prefix: str) -> None:
        self._prefix = str(prefix or "repository").strip() or "repository"
        self._started_at = time.perf_counter()
        self._stages: list[dict[str, Any]] = []

    def mark(self) -> float:
        return time.perf_counter()

    def record_elapsed(self, stage: str, started_at: float) -> None:
        self._stages.append(
            {
                "stage": f"{self._prefix}.{stage}",
                "duration_ms": _round_ms((time.perf_counter() - started_at) * 1000.0),
            }
        )

    def to_dict(self) -> dict[str, Any]:
        return {
            "total_ms": _round_ms((time.perf_counter() - self._started_at) * 1000.0),
            "stages": list(self._stages),
        }


def _attach_repository_diagnostics(payload: dict, timing: _RepositoryTiming) -> dict:
    diagnostics = payload.get("diagnostics")
    root = dict(diagnostics) if isinstance(diagnostics, dict) else {}
    root["repository"] = timing.to_dict()
    payload["diagnostics"] = root
    return payload


def _round_ms(value: float) -> float:
    return round(float(value), 3)


def _normalize_txn_key_values(values: Sequence[str]) -> list[str]:
    out: list[str] = []
    for item in values or []:
        value = str(item or "").strip()
        if value in out:
            continue
        out.append(value)
    return out


def _trim_expr(expr: str) -> str:
    return f"NULLIF(TRIM({expr}), '')"


def _clean_text_expr(expr: str) -> str:
    cleaned = _trim_expr(expr)
    cleaned = f"NULLIF({cleaned}, '-')"
    cleaned = f"NULLIF({cleaned}, '—')"
    cleaned = f"NULLIF({cleaned}, '－')"
    return cleaned


def _coalesce_expr(exprs: Sequence[Optional[str]], default: str = "NULL") -> str:
    parts = [e for e in exprs if e]
    if not parts:
        return default
    if len(parts) == 1:
        return parts[0]
    return "COALESCE(" + ",".join(parts) + ")"


def _has_col(columns: set[str], col: str) -> bool:
    return col in columns


def _text_col_expr(
    prefix: str,
    columns: set[str],
    col: str,
    *,
    clean_invalid: bool = False,
) -> Optional[str]:
    if not _has_col(columns, col):
        return None
    if clean_invalid:
        return _clean_text_expr(f"{prefix}.{col}")
    return _trim_expr(f"{prefix}.{col}")


def _account_key_expr(
    prefix: str,
    columns: set[str],
    candidates: Optional[Sequence[str]] = None,
) -> str:
    cols = candidates or (
        "clean_acct_no",
        "acct_no_norm",
        "acct_no",
        "clean_card_no",
        "card_no_norm",
        "card_no",
    )
    return _coalesce_expr([_text_col_expr(prefix, columns, c, clean_invalid=True) for c in cols])


def _env_bool(name: str, default: bool) -> bool:
    raw = os.environ.get(name)
    if raw is None:
        return bool(default)
    text = str(raw).strip().lower()
    if text in {"1", "true", "yes", "on"}:
        return True
    if text in {"0", "false", "no", "off", "null", "none", "nan", ""}:
        return False
    return bool(default)


def _env_int(name: str, default: int, *, min_value: int = 0, max_value: int = 9999) -> int:
    raw = os.environ.get(name)
    if raw is None:
        return int(default)
    try:
        value = int(str(raw).strip())
    except Exception:
        return int(default)
    return max(int(min_value), min(int(max_value), value))


def _safe_text(value: Any) -> str:
    return str(value or "").strip()


def _json_dumps(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def _stable_json_dumps(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def _int_cell(rows: Sequence[Sequence[Any]], index: int = 0) -> int:
    try:
        return max(0, int(rows[0][index] or 0)) if rows else 0
    except Exception:
        return 0


def _float_cell(rows: Sequence[Sequence[Any]], index: int = 0) -> float:
    try:
        return float(rows[0][index] or 0.0) if rows else 0.0
    except Exception:
        return 0.0


def _strict_nonnegative_int_cell(rows: Sequence[Sequence[Any]], index: int) -> Optional[int]:
    if len(rows) != 1 or index < 0 or index >= len(rows[0]):
        return None
    raw = rows[0][index]
    if raw is None or isinstance(raw, bool):
        return None
    try:
        value = int(raw)
        numeric = float(raw)
    except Exception:
        return None
    if value < 0 or not math.isfinite(numeric) or numeric != float(value):
        return None
    return value


def _strict_finite_float_cell(rows: Sequence[Sequence[Any]], index: int) -> Optional[float]:
    if len(rows) != 1 or index < 0 or index >= len(rows[0]):
        return None
    raw = rows[0][index]
    if raw is None or isinstance(raw, bool):
        return None
    try:
        value = float(raw)
    except Exception:
        return None
    return value if math.isfinite(value) else None


def _stats_query_id() -> str:
    stamp = datetime.utcnow().strftime("%Y%m%d%H%M%S%f")
    digest = hashlib.sha1(f"{stamp}:{os.getpid()}:{os.urandom(8).hex()}".encode("utf-8")).hexdigest()[:10]
    return f"qstats_{stamp}_{digest}"



def _normalize_metric_mode(value: str) -> str:
    return value if value in {"amount", "count", "counterparty"} else "amount"


def _normalize_direction_mode(value: str) -> str:
    return value if value in {"all", "in", "out", "net"} else "all"


def _normalize_granularity(value: str) -> str:
    return value if value in {"day", "week", "month", "hour"} else "day"


def _normalize_sort_dir(value: str) -> str:
    return "asc" if _safe_text(value).lower() == "asc" else "desc"


def _copy_json_dict(value: Any) -> dict:
    if not isinstance(value, dict):
        return {}
    return copy.deepcopy(value)


def _trim_dashboard_list(container: dict, key: str, limit: int, *, count_key: str, truncated_key: str) -> None:
    items = container.get(key)
    if not isinstance(items, list):
        return
    total = len(items)
    container[count_key] = total
    if total > limit:
        container[key] = items[:limit]
        container[truncated_key] = True
    else:
        container[truncated_key] = False


def _trim_chart_dashboard_for_response(dashboard: dict) -> dict:
    result = _copy_json_dict(dashboard)

    flow = result.get("flow")
    if isinstance(flow, dict):
        _trim_dashboard_list(
            flow,
            "inbound_items",
            _STATS_DASHBOARD_FLOW_ITEM_LIMIT,
            count_key="inbound_item_count",
            truncated_key="inbound_items_truncated",
        )
        _trim_dashboard_list(
            flow,
            "outbound_items",
            _STATS_DASHBOARD_FLOW_ITEM_LIMIT,
            count_key="outbound_item_count",
            truncated_key="outbound_items_truncated",
        )

    counterparties = result.get("counterparties")
    views = counterparties.get("views") if isinstance(counterparties, dict) else None
    if isinstance(views, dict):
        for view in views.values():
            if isinstance(view, dict):
                _trim_dashboard_list(
                    view,
                    "all_items",
                    _STATS_DASHBOARD_COUNTERPARTY_ITEM_LIMIT,
                    count_key="all_item_count",
                    truncated_key="all_items_truncated",
                )

    return result


class StatsRepository:
    """Repository adapter for stats queries aligned with the persisted analysis SQL semantics."""

    def __init__(self) -> None:
        self._storage = CaseStorage()
        self._daily_agg = TxnDailyAggregateStore(self._storage)
        self._chart_dashboard_compute_lock = threading.RLock()

    @property
    def storage(self) -> CaseStorage:
        return self._storage

    @property
    def _prefs_path(self):
        return self._storage.app_dir / "analysis_ui_prefs.json"

    def case_exists(self, case_id: str) -> bool:
        return self._storage.get_case(case_id) is not None

    def open_case_engine(self, case_id: str, *, read_only: bool = False) -> DuckDBEngine:
        return self._storage.open_case_engine(case_id, read_only=read_only)

    def _stats_source_revision(self, con: DuckDBEngine) -> int:
        try:
            return max(1, int(read_stats_flow_source_revision(con) or 1))
        except Exception:
            return 1

    def append_skill_runtime_query_log(
        self,
        *,
        case_id: str,
        tool_name: str,
        params: dict[str, Any],
        summary: dict[str, Any],
        row_count: int,
        duration_ms: int,
    ) -> str:
        con = self.open_case_engine(case_id, read_only=False)
        try:
            self._ensure_analysis_query_log_table(con)
            query_id = _stats_query_id()
            public_params, public_summary = project_query_log_diagnostic(
                case_id=case_id,
                query_id=query_id,
                params=params,
                summary=summary,
            )
            query_id = str(public_summary["query_ref"])
            con.execute(
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
                    datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S"),
                ),
            )
            self.storage.record_case_audit(
                case_id,
                "stats.skill_query",
                extra={"query_id": query_id, "tool_name": _safe_text(tool_name), "row_count": max(0, int(row_count))},
            )
            return query_id
        finally:
            try:
                con.close()
            except Exception:
                pass

    def _ensure_analysis_query_log_table(self, con: DuckDBEngine) -> None:
        con.execute(
            """
            CREATE TABLE IF NOT EXISTS analysis_query_log(
                query_id TEXT PRIMARY KEY,
                case_id TEXT,
                tool_name TEXT,
                params_json TEXT,
                summary_json TEXT,
                row_count BIGINT,
                duration_ms BIGINT,
                created_at TEXT
            )
            """
        )
        try:
            con.execute(
                "CREATE INDEX IF NOT EXISTS idx_analysis_query_log_case_tool_created ON analysis_query_log(case_id, tool_name, created_at)"
            )
        except Exception:
            pass

    def query_date_range(self, *, case_id: str) -> dict:
        case_id = str(case_id or "").strip()
        if not case_id or not self._daily_agg.ensure_materialized(case_id):
            return {"min": "", "max": ""}
        payload = query_stats_date_range(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
        )
        date_range = payload["date_range"]
        return {
            "min": str(date_range.get("min") or ""),
            "max": str(date_range.get("max") or ""),
        }

    def get_funds_status(self, *, case_id: str) -> str:
        if not str(case_id or "").strip():
            return "no-case"
        try:
            return "clean-ready" if self.storage.is_funds_clean_ready(case_id) else "not-clean"
        except Exception:
            return "unknown"

    def query_v2_case_overview(self, *, case_id: str) -> dict:
        case_id = _safe_text(case_id)
        overview = self._empty_case_overview(case_id=case_id, source_revision=1)
        if not case_id or not self._daily_agg.ensure_materialized(case_id):
            return overview

        con = self.open_case_engine(case_id, read_only=True)
        try:
            overview["source_revision"] = self._stats_source_revision(con)
            try:
                overview.update(self._query_case_overview_transactions(con))
            except Exception:
                overview.update(
                    {
                        "transaction_status": "unavailable",
                        "transaction_blocker": "transaction_query_failed",
                        "transaction_count": None,
                        "amount_status": "unavailable",
                        "amount_blocker": "transaction_query_failed",
                        "inflow_amount": None,
                        "outflow_amount": None,
                    }
                )
            if self._table_exists(con, ACCOUNT_DIM_TABLE):
                account_columns = self._table_columns(con, ACCOUNT_DIM_TABLE)
                try:
                    account_overview = self._query_case_overview_accounts(
                        con,
                        table=ACCOUNT_DIM_TABLE,
                        columns=account_columns,
                        key_col="account_key",
                        acct_type_col="acct_type",
                        open_name_col="open_name",
                        id_no_col="id_no",
                    )
                except Exception:
                    account_overview = self._unavailable_case_overview_accounts("account_query_failed")
                overview.update(account_overview)
            if overview["account_status"] != "verified" and self._table_exists(con, DETAIL_TABLE):
                detail_columns = self._table_columns(con, DETAIL_TABLE)
                try:
                    account_overview = self._query_case_overview_accounts(
                        con,
                        table=DETAIL_TABLE,
                        columns=detail_columns,
                        key_col="acct_key",
                        open_name_col="account_open_name",
                        id_no_col="opener_id_no",
                    )
                except Exception:
                    account_overview = self._unavailable_case_overview_accounts("account_query_failed")
                overview.update(account_overview)
            if overview["account_status"] != "verified" and not self._table_exists(con, ACCOUNT_DIM_TABLE) and not self._table_exists(con, DETAIL_TABLE):
                overview.update(self._unavailable_case_overview_accounts("account_source_unavailable"))
            return self._case_overview_host_boundary(overview)
        finally:
            try:
                con.close()
            except Exception:
                pass

    def _empty_case_overview(self, *, case_id: str, source_revision: int = 1) -> dict:
        return {
            "case_id": _safe_text(case_id),
            "fact_answer_allowed": False,
            "account_status": "unavailable",
            "account_blocker": "materialization_unavailable",
            "account_count": None,
            "personal_account_count": None,
            "corporate_account_count": None,
            "unknown_account_count": None,
            "transaction_status": "unavailable",
            "transaction_blocker": "materialization_unavailable",
            "transaction_count": None,
            "amount_status": "unavailable",
            "amount_blocker": "materialization_unavailable",
            "inflow_amount": None,
            "outflow_amount": None,
            "amount_total_rows": None,
            "amount_present_rows": None,
            "amount_missing_rows": None,
            "amount_parse_failed_rows": None,
            "direction_covered_rows": None,
            "source_table": "",
            "account_source_table": "",
            "source_revision": max(1, int(source_revision or 1)),
            "generated_at": datetime.utcnow().isoformat(timespec="milliseconds") + "Z",
        }

    @staticmethod
    def _case_overview_host_boundary(overview: dict[str, Any]) -> dict:
        boundary = dict(overview)
        boundary.update(
            {
                "fact_answer_allowed": False,
                "account_status": "unavailable",
                "account_blocker": "host_evidence_receipt_required",
                "account_count": None,
                "personal_account_count": None,
                "corporate_account_count": None,
                "unknown_account_count": None,
                "transaction_status": "unavailable",
                "transaction_blocker": "host_evidence_receipt_required",
                "transaction_count": None,
                "amount_status": "unavailable",
                "amount_blocker": "host_evidence_receipt_required",
                "inflow_amount": None,
                "outflow_amount": None,
                "amount_total_rows": None,
                "amount_present_rows": None,
                "amount_missing_rows": None,
                "amount_parse_failed_rows": None,
                "direction_covered_rows": None,
            }
        )
        return boundary

    def _query_case_overview_transactions(self, con: DuckDBEngine) -> dict:
        if not self._table_exists(con, DETAIL_TABLE):
            return {
                "transaction_status": "unavailable",
                "transaction_blocker": "transaction_source_unavailable",
                "transaction_count": None,
                "amount_status": "unavailable",
                "amount_blocker": "transaction_source_unavailable",
                "inflow_amount": None,
                "outflow_amount": None,
                "source_table": "",
            }

        columns = self._table_columns(con, DETAIL_TABLE)
        has_amount = "amount" in columns
        has_direction = "dc_val" in columns
        amount_expr = "TRY_CAST(amount AS DOUBLE)" if has_amount else "NULL"
        amount_source_present = (
            "TRIM(COALESCE(CAST(amount AS VARCHAR), '')) <> ''" if has_amount else "FALSE"
        )
        amount_finite = f"({amount_expr} IS NOT NULL AND isfinite({amount_expr}))" if has_amount else "FALSE"
        if has_amount and "amount_parse_failed" in columns:
            parse_flag_expr = "TRY_CAST(amount_parse_failed AS BIGINT)"
            amount_valid = f"({amount_finite} AND {parse_flag_expr}=0)"
            amount_parse_failed_expr = (
                f"CASE WHEN {parse_flag_expr} IS NULL OR {parse_flag_expr}<>0 "
                f"OR ({amount_source_present} AND NOT {amount_finite}) THEN 1 ELSE 0 END"
            )
        elif has_amount:
            amount_valid = amount_finite
            amount_parse_failed_expr = (
                f"CASE WHEN {amount_source_present} AND NOT {amount_finite} THEN 1 ELSE 0 END"
            )
        else:
            amount_valid = "FALSE"
            amount_parse_failed_expr = "0"
        amount_present_expr = f"CASE WHEN {amount_valid} THEN 1 ELSE 0 END"
        dc_expr = "TRIM(COALESCE(CAST(dc_val AS VARCHAR), ''))" if has_direction else "''"
        direction_covered_expr = f"CASE WHEN {dc_expr} IN ('进','出') THEN 1 ELSE 0 END"
        rows = con.query(
            f"""
            SELECT
              COUNT(1) AS transaction_count,
              COALESCE(SUM({amount_present_expr}), 0) AS amount_present_rows,
              COALESCE(SUM({amount_parse_failed_expr}), 0) AS amount_parse_failed_rows,
              COALESCE(SUM({direction_covered_expr}), 0) AS direction_covered_rows,
              COALESCE(SUM(CASE WHEN {dc_expr}='进' AND {amount_valid} THEN 1 ELSE 0 END), 0) AS inflow_valid_rows,
              COALESCE(SUM(CASE WHEN {dc_expr}='出' AND {amount_valid} THEN 1 ELSE 0 END), 0) AS outflow_valid_rows,
              SUM(CASE WHEN {dc_expr}='进' AND {amount_valid} THEN ABS({amount_expr}) ELSE NULL END) AS inflow_amount,
              SUM(CASE WHEN {dc_expr}='出' AND {amount_valid} THEN ABS({amount_expr}) ELSE NULL END) AS outflow_amount
            FROM {DETAIL_TABLE}
            """
        )
        if len(rows) != 1 or len(rows[0]) != 8:
            raise RuntimeError("case overview transaction aggregate shape is invalid")
        total_rows = _strict_nonnegative_int_cell(rows, 0)
        amount_present_rows = _strict_nonnegative_int_cell(rows, 1)
        amount_parse_failed_rows = _strict_nonnegative_int_cell(rows, 2)
        direction_covered_rows = _strict_nonnegative_int_cell(rows, 3)
        inflow_valid_rows = _strict_nonnegative_int_cell(rows, 4)
        outflow_valid_rows = _strict_nonnegative_int_cell(rows, 5)
        if (
            total_rows is None
            or amount_present_rows is None
            or amount_parse_failed_rows is None
            or direction_covered_rows is None
            or inflow_valid_rows is None
            or outflow_valid_rows is None
        ):
            raise RuntimeError("case overview transaction coverage is invalid")
        amount_missing_rows = total_rows - amount_present_rows
        if (
            amount_missing_rows < 0
            or amount_parse_failed_rows > amount_missing_rows
            or direction_covered_rows > total_rows
            or inflow_valid_rows + outflow_valid_rows > amount_present_rows
        ):
            raise RuntimeError("case overview transaction coverage is inconsistent")
        if total_rows == 0:
            return {
                "transaction_status": "unavailable",
                "transaction_blocker": "empty_scope_requires_host_receipt",
                "transaction_count": None,
                "amount_status": "unavailable",
                "amount_blocker": "empty_scope_requires_host_receipt",
                "inflow_amount": None,
                "outflow_amount": None,
                "amount_total_rows": 0,
                "amount_present_rows": 0,
                "amount_missing_rows": 0,
                "amount_parse_failed_rows": 0,
                "direction_covered_rows": 0,
                "source_table": DETAIL_TABLE,
            }
        amount_complete = (
            has_amount
            and amount_missing_rows == 0
            and amount_parse_failed_rows == 0
            and direction_covered_rows == total_rows
        )
        inflow_amount: Optional[float] = None
        outflow_amount: Optional[float] = None
        amount_status = "unavailable"
        amount_blocker = "amount_column_unavailable"
        if amount_complete:
            raw_inflow_cell = rows[0][6]
            raw_outflow_cell = rows[0][7]
            raw_inflow = _strict_finite_float_cell(rows, 6)
            raw_outflow = _strict_finite_float_cell(rows, 7)
            inflow_valid = (
                raw_inflow_cell is None
                if inflow_valid_rows == 0
                else raw_inflow is not None and raw_inflow >= 0
            )
            outflow_valid = (
                raw_outflow_cell is None
                if outflow_valid_rows == 0
                else raw_outflow is not None and raw_outflow >= 0
            )
            if (
                inflow_valid_rows + outflow_valid_rows == total_rows
                and inflow_valid
                and outflow_valid
            ):
                inflow_amount = round(raw_inflow if raw_inflow is not None else 0.0, 2)
                outflow_amount = round(raw_outflow if raw_outflow is not None else 0.0, 2)
                amount_status = "verified"
                amount_blocker = ""
            else:
                amount_status = "partial"
                amount_blocker = "amount_aggregate_invalid"
        elif has_amount:
            amount_status = "partial"
            amount_blocker = "amount_or_direction_coverage_partial"
        return {
            "transaction_status": "verified",
            "transaction_blocker": "",
            "transaction_count": total_rows,
            "amount_status": amount_status,
            "amount_blocker": amount_blocker,
            "inflow_amount": inflow_amount,
            "outflow_amount": outflow_amount,
            "amount_total_rows": total_rows,
            "amount_present_rows": amount_present_rows,
            "amount_missing_rows": amount_missing_rows,
            "amount_parse_failed_rows": amount_parse_failed_rows,
            "direction_covered_rows": direction_covered_rows,
            "source_table": DETAIL_TABLE,
        }

    @staticmethod
    def _unavailable_case_overview_accounts(blocker: str) -> dict:
        return {
            "account_status": "unavailable",
            "account_blocker": _safe_text(blocker) or "account_source_unavailable",
            "account_count": None,
            "personal_account_count": None,
            "corporate_account_count": None,
            "unknown_account_count": None,
            "account_source_table": "",
        }

    def _query_case_overview_accounts(
        self,
        con: DuckDBEngine,
        *,
        table: str,
        columns: set[str],
        key_col: str,
        acct_type_col: str = "",
        open_name_col: str = "",
        id_no_col: str = "",
    ) -> dict:
        if key_col not in columns:
            return self._unavailable_case_overview_accounts("account_identity_unavailable")

        def text_expr(column: str) -> str:
            if column and column in columns:
                return f"TRIM(COALESCE(CAST({column} AS VARCHAR), ''))"
            return "''"

        key_expr = text_expr(key_col)
        acct_type_expr = text_expr(acct_type_col)
        open_name_expr = text_expr(open_name_col)
        id_no_expr = text_expr(id_no_col)
        rows = con.query(
            f"""
            WITH accounts AS (
              SELECT DISTINCT
                {key_expr} AS account_key,
                LOWER({acct_type_expr}) AS acct_type,
                {open_name_expr} AS open_name,
                {id_no_expr} AS id_no
              FROM {table}
              WHERE {key_expr} <> ''
            ), classified AS (
              SELECT
                CASE
                  WHEN acct_type LIKE '%对公%'
                    OR acct_type LIKE '%单位%'
                    OR acct_type LIKE '%企业%'
                    OR acct_type LIKE '%公司%'
                    OR acct_type LIKE '%机构%'
                    OR acct_type LIKE '%organization%'
                    OR acct_type LIKE '%company%'
                    OR acct_type LIKE '%corp%'
                    OR open_name LIKE '%公司%'
                    OR open_name LIKE '%企业%'
                    OR open_name LIKE '%银行%'
                    OR open_name LIKE '%集团%'
                    OR open_name LIKE '%中心%'
                    OR open_name LIKE '%委员会%'
                    OR open_name LIKE '%合作社%'
                    OR open_name LIKE '%个体工商户%'
                    OR open_name LIKE '%有限公司%'
                    OR open_name LIKE '%有限责任%'
                    THEN 'corporate'
                  WHEN acct_type LIKE '%个人%'
                    OR acct_type LIKE '%私人%'
                    OR acct_type LIKE '%储蓄%'
                    OR acct_type LIKE '%personal%'
                    OR acct_type LIKE '%individual%'
                    OR regexp_matches(id_no, '^([0-9]{{15}}|[0-9]{{17}}[0-9Xx])$')
                    THEN 'personal'
                  ELSE 'unknown'
                END AS category
              FROM accounts
            )
            SELECT
              COUNT(1) AS account_count,
              COALESCE(SUM(CASE WHEN category='personal' THEN 1 ELSE 0 END), 0) AS personal_account_count,
              COALESCE(SUM(CASE WHEN category='corporate' THEN 1 ELSE 0 END), 0) AS corporate_account_count,
              COALESCE(SUM(CASE WHEN category='unknown' THEN 1 ELSE 0 END), 0) AS unknown_account_count
            FROM classified
            """
        )
        counts = [_strict_nonnegative_int_cell(rows, index) for index in range(4)]
        if any(value is None for value in counts):
            return self._unavailable_case_overview_accounts("account_query_result_invalid")
        account_count, personal_count, corporate_count, unknown_count = [int(value) for value in counts]
        if personal_count + corporate_count + unknown_count != account_count:
            return self._unavailable_case_overview_accounts("account_partition_invalid")
        return {
            "account_status": "verified",
            "account_blocker": "",
            "account_count": account_count,
            "personal_account_count": personal_count,
            "corporate_account_count": corporate_count,
            "unknown_account_count": unknown_count,
            "account_source_table": table,
        }

    def get_v2_table_widths(self, *, tab: str, mode: str) -> dict:
        data = self._load_prefs()
        widths = data.get("tableWidths", {}).get(f"{tab}|{mode}", {})
        if isinstance(widths, dict):
            return widths
        return {}

    def set_v2_table_widths(self, *, tab: str, mode: str, version: str, widths: object) -> dict:
        norm_widths = widths if isinstance(widths, list) else []
        data = self._load_prefs()
        data.setdefault("tableWidths", {})
        data["tableWidths"][f"{tab}|{mode}"] = {
            "v": str(version or ""),
            "widths": norm_widths,
        }
        self._save_prefs(data)
        return {"ok": True}

    def get_v2_log_config(self) -> dict:
        master = _env_bool("ANALYTIX_STATS_LOG_VERIFY_ENABLED", False)

        def _topic(name: str) -> bool:
            raw = os.environ.get(name)
            if raw is None:
                return master
            return _env_bool(name, False)

        left_selection = _topic("ANALYTIX_STATS_LOG_LEFT_SELECTION")
        table_selection = _topic("ANALYTIX_STATS_LOG_TABLE_SELECTION")
        transfer = _topic("ANALYTIX_STATS_LOG_TRANSFER")
        enabled = bool(master or left_selection or table_selection or transfer)
        sample_limit = _env_int("ANALYTIX_STATS_LOG_SAMPLE_LIMIT", 8, min_value=3, max_value=50)
        return {
            "enabled": enabled,
            "leftSelection": bool(left_selection),
            "tableSelection": bool(table_selection),
            "transfer": bool(transfer),
            "sampleLimit": int(sample_limit),
        }

    def log_v2_debug(self, *, payload: dict) -> dict:
        # Keep the repository boundary fail-closed even if an internal caller
        # bypasses the quarantined HTTP endpoint.  The payload is intentionally
        # opaque and is never inspected, rendered, serialized, or logged.
        del payload
        log_closed_diagnostic(
            _LOGGER,
            logging.WARNING,
            topic="stats_request",
            code="failed",
            flags={"retryable": False},
        )
        return {"ok": False, "code": "DEBUG_LOG_QUARANTINED"}

    def query_v2_tree(self, *, case_id: str, tab: str) -> dict:
        del tab
        if not self._daily_agg.ensure_materialized(case_id):
            return {
                "semantic_status": "source_unavailable",
                "fact_answer_allowed": False,
                "blocker": "materialization_unavailable",
                "groups": [],
            }
        # The Python data plane cannot resolve a same-turn, same-case Go
        # EvidenceReceipt.  Do not query, cache, or expose account identities
        # until the host authority handoff is present.
        return {
            "semantic_status": "blocked",
            "fact_answer_allowed": False,
            "blocker": "host_evidence_receipt_required",
            "groups": [],
        }

    def query_v2_rows(
        self,
        *,
        case_id: str,
        mode: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        search_text: str = "",
        row_sort_col: str = "",
        row_sort_dir: str = "desc",
        row_offset: int = 0,
        row_limit: int = 0,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        timing = _RepositoryTiming("repository.rows")
        started = timing.mark()
        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        timing.record_elapsed("normalize", started)
        if not selected:
            return {"rows": [], "status": "no-selection"}

        started = timing.mark()
        materialized = self._daily_agg.ensure_materialized(case_id)
        timing.record_elapsed("ensure_materialized", started)
        if not materialized:
            return _attach_repository_diagnostics({"rows": [], "status": "no-table"}, timing)

        started = timing.mark()
        rows_result = query_stats_rows(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            mode=mode,
            selected_keys=selected,
            date_start=date_start,
            date_end=date_end,
            search_text=search_text,
            row_sort_col=row_sort_col,
            row_sort_dir=row_sort_dir,
            row_offset=row_offset,
            row_limit=row_limit,
            row_format=row_format,
            fields=fields,
        )
        timing.record_elapsed("rust_query", started)

        started = timing.mark()
        raw_rows = rows_result.get("rows") or []
        result = {
            "rows": [dict(row) if isinstance(row, dict) else row for row in raw_rows if isinstance(row, (dict, list))],
            "total": int(rows_result.get("total") or 0),
            "row_summary": dict(rows_result.get("row_summary") or {}),
        }
        row_fields = rows_result.get("row_fields")
        if isinstance(row_fields, list):
            result["row_fields"] = [str(item) for item in row_fields]
        diagnostics = rows_result.get("diagnostics")
        if isinstance(diagnostics, dict):
            result["diagnostics"] = dict(diagnostics)
        timing.record_elapsed("result_copy", started)
        return _attach_repository_diagnostics(result, timing)

    def query_v2_rows_to_file(
        self,
        *,
        case_id: str,
        mode: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        search_text: str = "",
        row_sort_col: str = "",
        row_sort_dir: str = "desc",
        row_offset: int = 0,
        row_limit: int = 0,
        output_json: Path,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        if not selected:
            return {"payload": {"rows": [], "status": "no-selection"}}

        if not self._daily_agg.ensure_materialized(case_id):
            return {"payload": {"rows": [], "status": "no-table"}}

        return query_stats_rows_to_file(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            mode=mode,
            selected_keys=selected,
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

    def query_v2_txn_rows(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict] = None,
        key_values: Sequence[str] = (),
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        timing = _RepositoryTiming("repository.txn")
        started = timing.mark()
        key_type = str(key_type or "account").strip() or "account"
        key_value = str(key_value or "").strip()
        key_values_norm = _normalize_txn_key_values(key_values)
        if key_value and key_value not in key_values_norm:
            key_values_norm.insert(0, key_value)
        has_key_value = bool(key_value) or any(value for value in key_values_norm)
        if not has_key_value and key_type == "name":
            out = {"rows": []}
            if int(limit or 0) > 0:
                out["done"] = True
                out["nextCursor"] = None
            return out

        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        date_start = str(date_start or "").strip()
        date_end = str(date_end or "").strip()
        direction = str(direction or "all").strip()
        sort_col = str(sort_col or "txn_time").strip() or "txn_time"
        sort_col = "amount" if sort_col == "amount" else "txn_time"
        sort_dir = str(sort_dir or "asc").strip().lower()
        sort_dir = "desc" if sort_dir in ("-1", "desc", "down") else "asc"
        limit = int(limit or 0)
        if cursor is not None and limit <= 0:
            limit = 200
        timing.record_elapsed("normalize", started)

        started = timing.mark()
        materialized = self._daily_agg.ensure_materialized(case_id)
        timing.record_elapsed("ensure_materialized", started)
        if not materialized:
            out = {"rows": []}
            if limit > 0:
                out["done"] = True
                out["nextCursor"] = None
            return _attach_repository_diagnostics(out, timing)

        started = timing.mark()
        result = query_stats_txn_rows(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            key_type=key_type,
            key_value=key_value,
            key_values=key_values_norm,
            selected_keys=selected,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            row_format=row_format,
            fields=fields,
        )
        timing.record_elapsed("rust_query", started)

        started = timing.mark()
        result = dict(result)
        result.pop("ok", None)
        result.pop("case_id", None)
        timing.record_elapsed("result_cleanup", started)
        return _attach_repository_diagnostics(result, timing)

    def query_v2_txn_rows_to_file(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict] = None,
        key_values: Sequence[str] = (),
        output_json: Path,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        key_type = str(key_type or "account").strip() or "account"
        key_value = str(key_value or "").strip()
        key_values_norm = _normalize_txn_key_values(key_values)
        if key_value and key_value not in key_values_norm:
            key_values_norm.insert(0, key_value)
        limit = int(limit or 0)
        has_key_value = bool(key_value) or any(value for value in key_values_norm)
        if not has_key_value and key_type == "name":
            payload: dict[str, Any] = {"rows": []}
            if limit > 0:
                payload["done"] = True
                payload["next_cursor"] = None
            return {"payload": payload}

        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        date_start = str(date_start or "").strip()
        date_end = str(date_end or "").strip()
        direction = str(direction or "all").strip()
        sort_col = str(sort_col or "txn_time").strip() or "txn_time"
        sort_col = "amount" if sort_col == "amount" else "txn_time"
        sort_dir = str(sort_dir or "asc").strip().lower()
        sort_dir = "desc" if sort_dir in ("-1", "desc", "down") else "asc"
        if cursor is not None and limit <= 0:
            limit = 200

        if not self._daily_agg.ensure_materialized(case_id):
            payload = {"rows": []}
            if limit > 0:
                payload["done"] = True
                payload["next_cursor"] = None
            return {"payload": payload}

        return query_stats_txn_rows_to_file(
            case_id=case_id,
            db_path=self._storage.case_db(case_id),
            key_type=key_type,
            key_value=key_value,
            key_values=key_values_norm,
            selected_keys=selected,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            row_format=row_format,
            fields=fields,
            output_json=output_json,
        )

    def query_v2_account_txn_rows(
        self,
        *,
        case_id: str,
        account_key: str,
        date_start: str,
        date_end: str,
        start_time: str,
        end_time: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict] = None,
    ) -> dict:
        account_key = str(account_key or "").strip()
        if not account_key:
            return self._account_txn_rows_public_boundary("needs_input", "account_key_required")

        if not self._daily_agg.ensure_materialized(case_id):
            return self._account_txn_rows_public_boundary("source_unavailable", "materialization_unavailable")
        return self._account_txn_rows_public_boundary("blocked", "host_evidence_receipt_required")

    @staticmethod
    def _account_txn_rows_public_boundary(semantic_status: str, blocker: str) -> dict:
        return {
            "contract": "StatsAccountTxnRowsPublicBoundaryV1",
            "semantic_status": semantic_status,
            "fact_answer_allowed": False,
            "raw_details_exposed": False,
            "blocker": _safe_text(blocker),
            "rows": [],
            "done": False,
            "next_cursor": None,
        }

    def query_v2_chart_dashboard(
        self,
        *,
        case_id: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        metric_mode: str,
        direction_mode: str,
        granularity: str,
        success_filter: str,
        cash_filter: str,
        chart_filters: Sequence[dict],
        panel_views: Sequence[dict],
    ) -> dict:
        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        selected_count = len(selected)
        selection_mode = "single-card" if selected_count == 1 else "multi-card"
        active_filter_count = len([token for token in (chart_filters or []) if _safe_text((token or {}).get("dimension"))])
        object_summary = {
            "selected_card_count": selected_count,
            "time_range": {"date_start": _safe_text(date_start), "date_end": _safe_text(date_end)},
            "active_filter_count": active_filter_count,
            "panel_views": list(panel_views or []),
        }
        if not selected:
            return self._chart_dashboard_boundary_result(
                selection_mode=selection_mode,
                object_summary=object_summary,
                evidence_status="needs_selection",
                blocker="selection_required",
            )
        if not self._daily_agg.ensure_materialized(case_id):
            return self._chart_dashboard_boundary_result(
                selection_mode=selection_mode,
                object_summary=object_summary,
                evidence_status="source_unavailable",
                blocker="materialization_unavailable",
            )
        coverage_result: dict[str, Any]
        coverage_con = self.open_case_engine(case_id, read_only=True)
        try:
            coverage_result = self._query_case_overview_transactions(coverage_con)
        except Exception:
            return self._chart_dashboard_boundary_result(
                selection_mode=selection_mode,
                object_summary=object_summary,
                evidence_status="blocked",
                blocker="coverage_query_failed",
            )
        finally:
            try:
                coverage_con.close()
            except Exception:
                pass
        coverage = self._chart_dashboard_coverage(coverage_result)
        if coverage_result.get("transaction_status") != "verified":
            return self._chart_dashboard_boundary_result(
                selection_mode=selection_mode,
                object_summary=object_summary,
                evidence_status="source_unavailable",
                blocker=_safe_text(coverage_result.get("transaction_blocker")) or "transaction_scope_unavailable",
                coverage=coverage,
            )
        if coverage_result.get("amount_status") != "verified":
            amount_status = _safe_text(coverage_result.get("amount_status"))
            return self._chart_dashboard_boundary_result(
                selection_mode=selection_mode,
                object_summary=object_summary,
                evidence_status="partial" if amount_status == "partial" else "source_unavailable",
                blocker=_safe_text(coverage_result.get("amount_blocker")) or "amount_scope_unavailable",
                coverage=coverage,
            )
        # The Python data plane can validate source quality but cannot mint or
        # resolve the Go runtime's same-turn EvidenceReceipt. Keep every
        # factual dashboard section closed until that host authority is wired.
        return self._chart_dashboard_boundary_result(
            selection_mode=selection_mode,
            object_summary=object_summary,
            evidence_status="blocked",
            blocker="host_evidence_receipt_required",
            coverage=coverage,
        )

        # The code below remains the staged verified-data implementation. It
        # is unreachable until the host receipt handoff replaces the boundary
        # return above; do not activate it from caller-supplied identifiers.
        large_txn_threshold = float(
            _env_int("ANALYTIX_STATS_LARGE_TXN_THRESHOLD", 50000, min_value=1000, max_value=100000000)
        )
        normalized_metric_mode = _normalize_metric_mode(metric_mode)
        normalized_direction_mode = _normalize_direction_mode(direction_mode)
        normalized_granularity = _normalize_granularity(granularity)
        normalized_success_filter = _safe_text(success_filter) or "all"
        normalized_cash_filter = _safe_text(cash_filter) or "all"
        with self._chart_dashboard_compute_lock:
            payload = query_chart_dashboard(
                case_id=case_id,
                db_path=self._storage.case_db(case_id),
                selected_keys=selected,
                date_start=date_start,
                date_end=date_end,
                metric_mode=normalized_metric_mode,
                direction_mode=normalized_direction_mode,
                granularity=normalized_granularity,
                success_filter=normalized_success_filter,
                cash_filter=normalized_cash_filter,
                selection_mode=selection_mode,
                large_txn_threshold=large_txn_threshold,
                chart_filters=chart_filters,
            )
            dashboard = payload.get("dashboard") if isinstance(payload, dict) else {}
            dashboard = _trim_chart_dashboard_for_response(dashboard if isinstance(dashboard, dict) else {})
            summary = dashboard.get("summary") if isinstance(dashboard.get("summary"), dict) else {}
            txn_total_count = _strict_nonnegative_int_cell([[summary.get("txn_total_count")]], 0)
            active_counterparty_count = _strict_nonnegative_int_cell([[summary.get("active_counterparty_count")]], 0)
            total_in_amount = _strict_finite_float_cell([[summary.get("total_in_amount")]], 0)
            total_out_amount = _strict_finite_float_cell([[summary.get("total_out_amount")]], 0)
            net_in_amount = _strict_finite_float_cell([[summary.get("net_in_amount")]], 0)
            if (
                txn_total_count is None
                or active_counterparty_count is None
                or total_in_amount is None
                or total_out_amount is None
                or net_in_amount is None
                or total_in_amount < 0
                or total_out_amount < 0
                or abs(round(total_in_amount - total_out_amount - net_in_amount, 2)) > 0.01
                or txn_total_count > int(coverage.get("total_rows") or 0)
            ):
                return self._chart_dashboard_boundary_result(
                    selection_mode=selection_mode,
                    object_summary=object_summary,
                    evidence_status="blocked",
                    blocker="dashboard_summary_invalid",
                    coverage=coverage,
                )
            if txn_total_count == 0:
                return self._chart_dashboard_boundary_result(
                    selection_mode=selection_mode,
                    object_summary=object_summary,
                    evidence_status="verified_no_hit",
                    blocker="empty_scope_requires_host_receipt",
                    coverage=coverage,
                )
            result = {
                "evidence_status": "verified",
                "answer_card_complete": True,
                "fact_answer_allowed": True,
                "blocker": "",
                "coverage": coverage,
                "selection_mode": selection_mode,
                "object_summary": object_summary,
                "summary": summary,
                "trend": dashboard.get("trend") if isinstance(dashboard.get("trend"), dict) else {},
                "counterparties": dashboard.get("counterparties") if isinstance(dashboard.get("counterparties"), dict) else {},
                "structure": dashboard.get("structure") if isinstance(dashboard.get("structure"), dict) else {},
                "heatmap": dashboard.get("heatmap") if isinstance(dashboard.get("heatmap"), dict) else {},
                "distribution": dashboard.get("distribution") if isinstance(dashboard.get("distribution"), dict) else {},
                "flow": dashboard.get("flow") if isinstance(dashboard.get("flow"), dict) else {},
                "anomaly": dashboard.get("anomaly") if isinstance(dashboard.get("anomaly"), dict) else {},
            }
            return _copy_json_dict(result)

    @staticmethod
    def _chart_dashboard_coverage(result: dict[str, Any]) -> dict[str, Optional[int]]:
        def value(name: str) -> Optional[int]:
            raw = result.get(name)
            return raw if isinstance(raw, int) and not isinstance(raw, bool) and raw >= 0 else None

        return {
            "total_rows": value("amount_total_rows"),
            "amount_present_rows": value("amount_present_rows"),
            "amount_missing_rows": value("amount_missing_rows"),
            "amount_parse_failed_rows": value("amount_parse_failed_rows"),
            "direction_covered_rows": value("direction_covered_rows"),
        }

    @staticmethod
    def _chart_dashboard_boundary_result(
        *,
        selection_mode: str,
        object_summary: dict[str, Any],
        evidence_status: str,
        blocker: str,
        coverage: Optional[dict[str, Optional[int]]] = None,
    ) -> dict:
        return {
            "evidence_status": evidence_status,
            "answer_card_complete": True,
            "fact_answer_allowed": False,
            "blocker": _safe_text(blocker),
            "coverage": coverage
            or {
                "total_rows": None,
                "amount_present_rows": None,
                "amount_missing_rows": None,
                "amount_parse_failed_rows": None,
                "direction_covered_rows": None,
            },
            "selection_mode": selection_mode,
            "object_summary": dict(object_summary),
            "summary": {},
            "trend": {},
            "counterparties": {},
            "structure": {},
            "heatmap": {},
            "distribution": {},
            "flow": {},
            "anomaly": {},
        }

    def query_v2_chart_detail_rows(
        self,
        *,
        case_id: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        metric_mode: str,
        direction_mode: str,
        granularity: str,
        success_filter: str,
        cash_filter: str,
        chart_filters: Sequence[dict],
        panel_views: Sequence[dict],
        sort_col: str,
        sort_dir: str,
        page: int,
        limit: int,
        visible_columns: Sequence[str],
    ) -> dict:
        selected = [str(item or "").strip() for item in (selected_keys or []) if str(item or "").strip()]
        selected_count = len(selected)
        selection_mode = "single-card" if selected_count == 1 else "multi-card"
        if not selected:
            return {
                "evidence_status": "needs_selection",
                "answer_card_complete": True,
                "fact_answer_allowed": False,
                "blocker": "selection_required",
                "selection_mode": selection_mode,
                "rows": [],
                "total": None,
            }
        if not self._daily_agg.ensure_materialized(case_id):
            return {
                "evidence_status": "source_unavailable",
                "answer_card_complete": True,
                "fact_answer_allowed": False,
                "blocker": "materialization_unavailable",
                "selection_mode": selection_mode,
                "rows": [],
                "total": None,
            }
        return {
            "evidence_status": "blocked",
            "answer_card_complete": True,
            "fact_answer_allowed": False,
            "blocker": "host_evidence_receipt_required",
            "selection_mode": selection_mode,
            "rows": [],
            "total": None,
        }

    def get_account_delete_info(self, *, case_id: str, account_keys: Sequence[str]) -> dict:
        keys = [str(x or "").strip() for x in (account_keys or []) if str(x or "").strip()]
        if not keys:
            return {"txn_count": 0}

        con = self.open_case_engine(case_id)
        try:
            if not self._table_exists(con, "fc_transaction_norm"):
                return {"txn_count": 0}

            cols = self._table_columns(con, "fc_transaction_norm")
            acct_key_expr = _account_key_expr(
                "t",
                cols,
                candidates=("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
            )
            if acct_key_expr == "NULL":
                return {"txn_count": 0}

            placeholders = ",".join(["?"] * len(keys))
            where_sql = f"t.case_id=? AND {acct_key_expr} IN ({placeholders})"
            count_sql = f"SELECT COUNT(1) FROM fc_transaction_norm t WHERE {where_sql}"
            rows = con.query(count_sql, tuple([case_id] + keys))
            return {"txn_count": int(rows[0][0]) if rows else 0}
        finally:
            try:
                con.close()
            except Exception:
                pass

    def update_account_info(
        self,
        *,
        case_id: str,
        account_keys: Sequence[str],
        account_open_name: str,
        opener_id_no: str,
    ) -> dict:
        keys = [str(x or "").strip() for x in (account_keys or []) if str(x or "").strip()]
        if not keys:
            return {"txn_count": 0, "account_count": 0}

        con = self.open_case_engine(case_id, read_only=False)
        try:
            if not self._table_exists(con, "fc_transaction_norm"):
                return {"txn_count": 0, "account_count": 0}

            cols = self._table_columns(con, "fc_transaction_norm")
            acct_key_expr = _account_key_expr(
                "t",
                cols,
                candidates=("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
            )
            if acct_key_expr == "NULL":
                return {"txn_count": 0, "account_count": 0}

            name_raw = str(account_open_name or "").strip()
            id_raw = str(opener_id_no or "").strip()
            name_val = None if not name_raw or name_raw in ("未登记户名",) else name_raw
            id_val = None if not id_raw or id_raw in ("无证件号",) else id_raw

            set_parts: list[str] = []
            params: list[Any] = []
            if "account_open_name" in cols:
                set_parts.append("account_open_name=?")
                params.append(name_val)
            if "opener_id_no" in cols:
                set_parts.append("opener_id_no=?")
                params.append(id_val)
            if "clean_account_filled" in cols:
                set_parts.append("clean_account_filled=0")

            placeholders = ",".join(["?"] * len(keys))
            where_sql = f"t.case_id=? AND {acct_key_expr} IN ({placeholders})"
            where_params = [case_id] + keys

            count_sql = f"SELECT COUNT(1) FROM fc_transaction_norm t WHERE {where_sql}"
            rows = con.query(count_sql, tuple(where_params))
            txn_count = int(rows[0][0]) if rows else 0

            self._ensure_manual_account_mapping_table(con)
            for key in keys:
                con.execute(
                    "INSERT OR REPLACE INTO analysis_manual_account_mapping "
                    "(case_id, account_key, account_open_name, opener_id_no, bank_name, updated_at) "
                    "VALUES (?,?,?,?,?,NOW())",
                    (case_id, key, name_raw, id_raw, "",),
                )

            if set_parts:
                update_sql = f"UPDATE fc_transaction_norm t SET {', '.join(set_parts)} WHERE {where_sql}"
                con.execute(update_sql, tuple(params + where_params))

            account_count = 0
            if self._table_exists(con, "fc_account_norm"):
                acct_cols = self._table_columns(con, "fc_account_norm")
                acct_key_expr = _account_key_expr("a", acct_cols)
                if acct_key_expr != "NULL":
                    acct_set: list[str] = []
                    acct_params: list[Any] = []
                    if "account_open_name" in acct_cols:
                        acct_set.append("account_open_name=?")
                        acct_params.append(name_val)
                    if "opener_id_no" in acct_cols:
                        acct_set.append("opener_id_no=?")
                        acct_params.append(id_val)
                    if acct_set:
                        acct_where = f"a.case_id=? AND {acct_key_expr} IN ({placeholders})"
                        acct_count_sql = f"SELECT COUNT(1) FROM fc_account_norm a WHERE {acct_where}"
                        acct_rows = con.query(acct_count_sql, tuple([case_id] + keys))
                        account_count = int(acct_rows[0][0]) if acct_rows else 0
                        acct_update = f"UPDATE fc_account_norm a SET {', '.join(acct_set)} WHERE {acct_where}"
                        con.execute(acct_update, tuple(acct_params + [case_id] + keys))

            bump_stats_flow_source_revision(con, reason="stats:update_account_info")
            return {"txn_count": txn_count, "account_count": account_count}
        finally:
            try:
                con.close()
            except Exception:
                pass

    def delete_accounts(self, *, case_id: str, account_keys: Sequence[str]) -> dict:
        keys = [str(x or "").strip() for x in (account_keys or []) if str(x or "").strip()]
        if not keys:
            return {"txn_count": 0}

        con = self.open_case_engine(case_id, read_only=False)
        try:
            if not self._table_exists(con, "fc_transaction_norm"):
                return {"txn_count": 0}

            cols = self._table_columns(con, "fc_transaction_norm")
            acct_key_expr = _account_key_expr(
                "t",
                cols,
                candidates=("clean_acct_no", "acct_no_norm", "acct_no", "clean_card_no", "card_no_norm", "card_no"),
            )
            if acct_key_expr == "NULL":
                return {"txn_count": 0}

            placeholders = ",".join(["?"] * len(keys))
            where_sql = f"t.case_id=? AND {acct_key_expr} IN ({placeholders})"
            where_params = [case_id] + keys

            count_sql = f"SELECT COUNT(1) FROM fc_transaction_norm t WHERE {where_sql}"
            rows = con.query(count_sql, tuple(where_params))
            txn_count = int(rows[0][0]) if rows else 0

            delete_sql = f"DELETE FROM fc_transaction_norm t WHERE {where_sql}"
            con.execute(delete_sql, tuple(where_params))

            if self._table_exists(con, "analysis_manual_account_mapping"):
                mapping_where = f"case_id=? AND account_key IN ({placeholders})"
                con.execute(
                    f"DELETE FROM analysis_manual_account_mapping WHERE {mapping_where}",
                    tuple([case_id] + keys),
                )

            if self._table_exists(con, "fc_account_norm"):
                acct_cols = self._table_columns(con, "fc_account_norm")
                acct_key_expr = _account_key_expr("a", acct_cols)
                if acct_key_expr != "NULL":
                    acct_where = f"a.case_id=? AND {acct_key_expr} IN ({placeholders})"
                    acct_delete = f"DELETE FROM fc_account_norm a WHERE {acct_where}"
                    con.execute(acct_delete, tuple([case_id] + keys))

            bump_stats_flow_source_revision(con, reason="stats:delete_accounts")
            return {"txn_count": txn_count}
        finally:
            try:
                con.close()
            except Exception:
                pass

    def set_doc_pending(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        pending: bool,
    ) -> dict:
        con = self.open_case_engine(case_id, read_only=False)
        try:
            self._ensure_doc_table(con)
            if pending:
                con.execute(
                    "INSERT OR REPLACE INTO analysis_doc_status "
                    "(case_id, key_type, key_value, status, updated_at) VALUES (?,?,?,?,NOW())",
                    (case_id, key_type, key_value, "pending"),
                )
            else:
                con.execute(
                    "DELETE FROM analysis_doc_status WHERE case_id=? AND key_type=? AND key_value=?",
                    (case_id, key_type, key_value),
                )
            return {"ok": True}
        finally:
            try:
                con.close()
            except Exception:
                pass

    def _ensure_doc_table(self, con: DuckDBEngine) -> None:
        con.execute(
            "CREATE TABLE IF NOT EXISTS analysis_doc_status ("
            "  case_id VARCHAR,"
            "  key_type VARCHAR,"
            "  key_value VARCHAR,"
            "  status VARCHAR,"
            "  updated_at TIMESTAMP,"
            "  PRIMARY KEY(case_id, key_type, key_value)"
            ")"
        )

    def _load_prefs(self) -> dict:
        try:
            if self._prefs_path.exists():
                return json.loads(self._prefs_path.read_text(encoding="utf-8"))
        except Exception:
            return {}
        return {}

    def _save_prefs(self, data: dict) -> None:
        try:
            self._prefs_path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")
        except Exception:
            pass

    def _ensure_manual_account_mapping_table(self, con: DuckDBEngine) -> None:
        con.execute(
            "CREATE TABLE IF NOT EXISTS analysis_manual_account_mapping ("
            "  case_id VARCHAR,"
            "  account_key VARCHAR,"
            "  account_open_name VARCHAR,"
            "  opener_id_no VARCHAR,"
            "  bank_name VARCHAR,"
            "  updated_at TIMESTAMP,"
            "  PRIMARY KEY(case_id, account_key)"
            ")"
        )

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
        return {str(r[0]) for r in rows if r and r[0]}
