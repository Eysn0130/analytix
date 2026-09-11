from __future__ import annotations

import hashlib
import json
import math
import threading
import time
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Optional, Sequence

from app.core.analysis_compute import try_materialize_txn_daily
from app.core.db_engine import DuckDBEngine
from app.core.storage import CaseStorage
from app.repositories.analysis_revision import get_stats_flow_source_revision
from app.repositories.txn_daily_materialization_executor import (
    execute_materialization_sql_steps,
    execute_materialized_index_sql_steps,
)
from app.repositories.txn_daily_materialization_plan import (
    build_materialization_plan,
    create_materialization_sql_steps,
)
from app.repositories.txn_daily_materialization_plan_constants import (
    ACCOUNT_DIM_TABLE as _ACCOUNT_DIM_TABLE,
    AGG_TABLE as _AGG_TABLE,
    AGG_VERSION as _AGG_VERSION,
    DETAIL_TABLE as _DETAIL_TABLE,
    MATERIALIZATION_IDENTITY_SCHEMA_VERSION as _MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
    KEYWORD_TABLE as _KEYWORD_TABLE,
    META_TABLE as _META_TABLE,
)
from app.repositories.txn_daily_materialization_identity import (
    TxnDailyMaterializationIdentity,
    build_txn_daily_materialization_identity,
)
from app.repositories.txn_daily_materialization_profile import (
    ProfileCallback,
    TxnDailyMaterializationProfile,
)
from app.repositories.txn_materialized_index_sql import create_materialized_index_sql_steps
from app.utils.counterparty_placeholders import normalize_placeholder_kinds, placeholder_token_sql


_RESULT_SIGNATURE_DOMAIN = "analytix.txn-daily-result/v12"
_JSON_SAFE_INTEGER_MAX = (1 << 53) - 1
_RESULT_SIGNATURE_TABLES = (
    (_AGG_TABLE, "r.acct_key, r.txn_day, r.cp_key, r.dc_val"),
    (_DETAIL_TABLE, "TRY_CAST(r.id AS BIGINT), r.id"),
    (
        _KEYWORD_TABLE,
        "TRY_CAST(r.txn_row_id AS BIGINT), r.txn_row_id, r.kind, r.token, r.token_order",
    ),
    (_ACCOUNT_DIM_TABLE, "r.account_key"),
)


def _is_json_safe_nonnegative_int(value: object) -> bool:
    return type(value) is int and 0 <= value <= _JSON_SAFE_INTEGER_MAX


def _shutdown_stats_worker_before_duckdb_write() -> None:
    try:
        from app.core.analysis_compute_worker import shutdown_stats_query_worker

        shutdown_stats_query_worker()
    except Exception:
        pass


class TxnAmountCoverageIncompleteError(RuntimeError):
    """Fail-closed boundary for transaction amount and direction facts.

    The message is deliberately constant: case identifiers and source values
    belong in controlled evidence, not ordinary error/log projection.
    """

    code = "transaction_amount_coverage_incomplete"

    def __init__(self) -> None:
        super().__init__(self.code)


def _require_json_safe_nonnegative_int(value: object) -> int:
    if not _is_json_safe_nonnegative_int(value):
        raise TxnAmountCoverageIncompleteError()
    assert type(value) is int
    return value


def _require_exact_identity_text(value: object) -> str:
    if type(value) is not str or not value or value != value.strip():
        raise TxnAmountCoverageIncompleteError()
    return value


@dataclass(frozen=True)
class TxnAmountCoverageV1:
    case_id: str
    materialization_identity: str
    total_rows: int
    amount_valid_rows: int
    amount_missing_rows: int
    amount_parse_failed_rows: int
    direction_covered_rows: int

    @property
    def contract(self) -> str:
        return "TxnAmountCoverageV1"

    @property
    def complete(self) -> bool:
        return (
            self.total_rows > 0
            and self.amount_valid_rows == self.total_rows
            and self.amount_missing_rows == 0
            and self.amount_parse_failed_rows == 0
            and self.direction_covered_rows == self.total_rows
        )

    @property
    def digest(self) -> str:
        payload = {
            "amountMissingRows": self.amount_missing_rows,
            "amountParseFailedRows": self.amount_parse_failed_rows,
            "amountValidRows": self.amount_valid_rows,
            "caseId": self.case_id,
            "contract": self.contract,
            "directionCoveredRows": self.direction_covered_rows,
            "materializationIdentity": self.materialization_identity,
            "totalRows": self.total_rows,
        }
        canonical = json.dumps(payload, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
        digest = hashlib.sha256(b"AnalytixTxnAmountCoverageV1\x00" + canonical.encode("utf-8")).hexdigest()
        return f"txn_amount_coverage_v1_{digest}"


class TxnDailyAggregateStore:
    _guard = threading.RLock()
    _case_locks: dict[str, threading.RLock] = {}

    def __init__(self, storage: CaseStorage) -> None:
        self._storage = storage

    def ensure_materialized(
        self,
        case_id: str,
        *,
        force: bool = False,
        engine: Optional[DuckDBEngine] = None,
        profile_cb: Optional[ProfileCallback] = None,
    ) -> bool:
        case_key = str(case_id or "").strip()
        if not case_key:
            return False
        # A caller-owned connection cannot prove that the production native
        # materializer or its verifier executed.  In particular, legacy
        # Python-created tables on that connection are not fact authority.
        if engine is not None:
            profile = TxnDailyMaterializationProfile()
            profile.emit(profile_cb, ok=False)
            return False
        case_lock = self._get_case_lock(case_key)
        with case_lock:
            profile = TxnDailyMaterializationProfile()

            con = self._storage.open_case_engine(case_key, read_only=True)
            try:
                if not self._table_exists(con, "fc_transaction_norm"):
                    profile.emit(profile_cb, ok=False)
                    return False
                source_revision = get_stats_flow_source_revision(con)
                started = time.perf_counter()
                source_snapshot = self._compute_source_snapshot(con, case_key)
                profile.record_since("compute_source_snapshot", started)
                try:
                    db_path = con.path
                    con.close()
                except Exception:
                    profile.emit(profile_cb, ok=False)
                    return False
                _shutdown_stats_worker_before_duckdb_write()
                started = time.perf_counter()
                native_result = try_materialize_txn_daily(
                    case_id=case_key,
                    db_path=db_path,
                    source_revision=source_revision,
                    source_snapshot=source_snapshot,
                )
                profile.record_since("native_materialize", started)
                if native_result is None:
                    profile.emit(profile_cb, ok=False)
                    return False
                profile.record_native_profile(native_result)
                con = self._storage.open_case_engine(case_key, read_only=True)
                verified_revision = get_stats_flow_source_revision(con)
                verified_snapshot = self._compute_source_snapshot(con, case_key)
                if (
                    verified_revision != source_revision
                    or verified_snapshot != source_snapshot
                    or not self._is_current(
                        con,
                        case_key,
                        verified_revision,
                        verified_snapshot,
                    )
                ):
                    profile.emit(profile_cb, ok=False)
                    return False
                profile.emit(profile_cb, ok=True, native_used=True)
                return True
            except Exception:
                profile.emit(profile_cb, ok=False)
                return False
            finally:
                try:
                    con.close()
                except Exception:
                    pass

    def query_flow_focus_rows(
        self,
        *,
        case_id: str,
        query_seed_ids: Sequence[str],
        selected_focus_ids: Sequence[str],
        selected_placeholder_kinds: Sequence[str],
        include_missing_counterparty: bool,
        direction: str,
        min_amount: float,
        date_start: Optional[datetime],
        date_end_excl: Optional[datetime],
        ensure_ready: bool = True,
        engine: Optional[DuckDBEngine] = None,
    ) -> Optional[list[tuple]]:
        if isinstance(min_amount, bool) or not isinstance(min_amount, (int, float)):
            raise TxnAmountCoverageIncompleteError()
        normalized_min_amount = float(min_amount)
        if not math.isfinite(normalized_min_amount) or normalized_min_amount < 0:
            raise TxnAmountCoverageIncompleteError()
        if normalized_min_amount > 0:
            return None
        seeds = [str(item or "").strip() for item in (query_seed_ids or []) if str(item or "").strip()]
        focus_ids = [str(item or "").strip() for item in (selected_focus_ids or []) if str(item or "").strip()]
        placeholder_kinds = normalize_placeholder_kinds(selected_placeholder_kinds)
        if not seeds or (not focus_ids and not placeholder_kinds):
            return None
        if ensure_ready and not self.ensure_materialized(case_id, engine=engine):
            return None
        close_engine = engine is None
        con = engine or self._storage.open_case_engine(case_id, read_only=True)
        try:
            if close_engine:
                con.execute("BEGIN TRANSACTION")
            self.require_complete_amount_coverage(case_id=case_id, engine=con, ensure_ready=False)
            if not self._table_exists(con, _AGG_TABLE):
                return None

            params: list[Any] = []
            filters = []
            seed_placeholders = ",".join(["?"] * len(seeds))
            filters.append(f"acct_key IN ({seed_placeholders})")
            params.extend(seeds)
            filters.append("dc_val IN ('进','出')")
            if date_start is not None:
                filters.append("txn_day >= CAST(? AS DATE)")
                params.append(date_start.strftime("%Y-%m-%d"))
            if date_end_excl is not None:
                filters.append("txn_day < CAST(? AS DATE)")
                params.append(date_end_excl.strftime("%Y-%m-%d"))

            focus_match_parts: list[str] = []
            if focus_ids:
                focus_placeholders = ",".join(["?"] * len(focus_ids))
                focus_match_parts.append(f"(cp_placeholder_kind IS NULL AND cp_key IN ({focus_placeholders}))")
                params.extend(focus_ids)
            if placeholder_kinds:
                kind_placeholders = ",".join(["?"] * len(placeholder_kinds))
                focus_match_parts.append(f"(cp_placeholder_kind IN ({kind_placeholders}))")
                params.extend(placeholder_kinds)
            if not focus_match_parts:
                return None
            filters.append("(" + " OR ".join(focus_match_parts) + ")")

            dir_mode = direction if direction in {"in", "out"} else "all"
            if dir_mode == "out":
                filters.append("dc_val='出'")
            elif dir_mode == "in":
                filters.append("dc_val='进'")

            unknown_prefix = "__unknown_cp__name::"
            unknown_cp_id_empty = f"{unknown_prefix}__empty__"
            if include_missing_counterparty:
                cp_key_norm_expr = (
                    "CASE "
                    "WHEN cp_placeholder_kind IS NOT NULL AND cp_name IS NOT NULL THEN "
                    f"{placeholder_token_sql('cp_placeholder_kind')} || '::name::' || cp_name "
                    "WHEN cp_placeholder_kind IS NOT NULL THEN "
                    f"{placeholder_token_sql('cp_placeholder_kind')} "
                    "WHEN cp_key IS NOT NULL THEN cp_key "
                    "WHEN cp_name IS NOT NULL THEN "
                    f"'{unknown_prefix}' || cp_name "
                    "ELSE "
                    f"'{unknown_cp_id_empty}' END"
                )
            else:
                cp_key_norm_expr = "cp_key"

            where_sql = " AND ".join(filters) if filters else "1=1"
            sql = (
                "WITH base AS ("
                "  SELECT acct_key, cp_key, cp_raw AS cp_key_raw, cp_placeholder_kind, dc_val, "
                "         txn_count, amt_sum, amount_valid_count, amount_missing_count, "
                "         amount_parse_failed_count, first_ts, last_ts, open_name, cp_name, cp_name_pick "
                f"  FROM {_AGG_TABLE} "
                f"  WHERE {where_sql}"
                ") "
                "SELECT "
                f"  acct_key, {cp_key_norm_expr} AS cp_key, cp_key_raw, dc_val, "
                "  SUM(txn_count) AS txn_count, "
                "  CASE WHEN SUM(txn_count)=SUM(amount_valid_count) "
                "             AND SUM(amount_missing_count)=0 "
                "             AND SUM(amount_parse_failed_count)=0 "
                "       THEN SUM(amt_sum) ELSE NULL END AS amt_sum, "
                "  MIN(first_ts) AS first_ts, "
                "  MAX(last_ts) AS last_ts, "
                "  MAX(open_name) AS open_name, "
                "  COALESCE(MAX(cp_name_pick), MAX(cp_name)) AS cp_name "
                "FROM base "
                f"GROUP BY acct_key, {cp_key_norm_expr}, cp_key, cp_key_raw, dc_val "
                "ORDER BY amt_sum DESC"
            )
            return list(con.query(sql, tuple(params)))
        except TxnAmountCoverageIncompleteError:
            raise
        except Exception:
            return None
        finally:
            if close_engine:
                try:
                    con.execute("ROLLBACK")
                except Exception:
                    pass
                try:
                    con.close()
                except Exception:
                    pass

    def require_complete_amount_coverage(
        self,
        *,
        case_id: str,
        engine: Optional[DuckDBEngine] = None,
        ensure_ready: bool = True,
    ) -> TxnAmountCoverageV1:
        """Require complete case-wide amount and direction coverage.

        P0 intentionally uses a conservative case-wide gate. Scope-specific
        amount publication can be restored only with a host-owned query scope,
        dataset snapshot and EvidenceReceipt binding.
        """

        if type(case_id) is not str or not case_id or case_id != case_id.strip():
            raise TxnAmountCoverageIncompleteError()
        case_key = case_id
        if ensure_ready and not self.ensure_materialized(case_key, engine=engine):
            raise TxnAmountCoverageIncompleteError()

        close_engine = engine is None
        con = engine or self._storage.open_case_engine(case_key, read_only=True)
        try:
            if (
                not self._table_exists(con, _DETAIL_TABLE)
                or not self._table_exists(con, _AGG_TABLE)
                or not self._table_exists(con, _META_TABLE)
            ):
                raise TxnAmountCoverageIncompleteError()
            source_revision = self._read_existing_source_revision(con)
            source_snapshot = self._compute_source_snapshot(con, case_key)
            if not self._is_current(con, case_key, source_revision, source_snapshot):
                raise TxnAmountCoverageIncompleteError()
            required_columns = {"amount", "amount_source_present", "amount_parse_failed", "dc_val"}
            if not required_columns.issubset(self._table_columns(con, _DETAIL_TABLE)):
                raise TxnAmountCoverageIncompleteError()
            required_aggregate_columns = {
                "txn_count",
                "amount_source_present_count",
                "amount_valid_count",
                "amount_missing_count",
                "amount_parse_failed_count",
                "amt_sum",
                "dc_val",
            }
            if not required_aggregate_columns.issubset(self._table_columns(con, _AGG_TABLE)):
                raise TxnAmountCoverageIncompleteError()
            required_meta_columns = {
                "agg_name",
                "agg_version",
                "case_id",
                "source_signature",
                "result_signature",
                "row_count",
            }
            if not required_meta_columns.issubset(self._table_columns(con, _META_TABLE)):
                raise TxnAmountCoverageIncompleteError()
            rows = con.query(
                f"""
                SELECT
                  COUNT(1) AS total_rows,
                  SUM(CASE WHEN amount IS NOT NULL
                                AND isfinite(amount)
                                AND amount_source_present=1
                                AND amount_parse_failed=0 THEN 1 ELSE 0 END) AS amount_valid_rows,
                  SUM(CASE WHEN amount IS NULL
                                AND amount_source_present=0
                                AND amount_parse_failed=0 THEN 1 ELSE 0 END) AS amount_missing_rows,
                  SUM(CASE WHEN amount IS NULL
                                AND amount_source_present=1
                                AND amount_parse_failed=1 THEN 1 ELSE 0 END) AS amount_parse_failed_rows,
                  SUM(CASE WHEN dc_val IN ('进','出') THEN 1 ELSE 0 END) AS direction_covered_rows
                FROM {_DETAIL_TABLE}
                """
            )
            aggregate_rows = con.query(
                f"""
                SELECT
                  COUNT(1) AS aggregate_rows,
                  COALESCE(SUM(txn_count), 0) AS total_rows,
                  COALESCE(SUM(amount_source_present_count), 0) AS amount_source_present_rows,
                  COALESCE(SUM(amount_valid_count), 0) AS amount_valid_rows,
                  COALESCE(SUM(amount_missing_count), 0) AS amount_missing_rows,
                  COALESCE(SUM(amount_parse_failed_count), 0) AS amount_parse_failed_rows,
                  COALESCE(SUM(CASE WHEN dc_val IN ('进','出') THEN txn_count ELSE 0 END), 0)
                    AS direction_covered_rows,
                  COALESCE(SUM(CASE
                    WHEN txn_count <= 0
                      OR amount_source_present_count < 0
                      OR amount_valid_count < 0
                      OR amount_missing_count < 0
                      OR amount_parse_failed_count < 0
                      OR txn_count <> amount_valid_count + amount_missing_count + amount_parse_failed_count
                      OR amount_source_present_count <> amount_valid_count + amount_parse_failed_count
                      OR (amount_valid_count = txn_count AND (amt_sum IS NULL OR NOT isfinite(amt_sum)))
                      OR (amount_valid_count <> txn_count AND amt_sum IS NOT NULL)
                    THEN 1 ELSE 0 END), 0) AS invalid_rows
                FROM {_AGG_TABLE}
                """
            )
            identity_rows = con.query(
                f"SELECT agg_name, agg_version, case_id, row_count "
                f"FROM {_META_TABLE} WHERE starts_with(agg_name, 'txn_daily_')"
            )
            if (
                len(rows) != 1
                or len(rows[0]) != 5
                or len(aggregate_rows) != 1
                or len(aggregate_rows[0]) != 8
                or len(identity_rows) != 1
                or len(identity_rows[0]) != 4
            ):
                raise TxnAmountCoverageIncompleteError()
            counts = tuple(_require_json_safe_nonnegative_int(value) for value in rows[0])
            aggregate_counts = tuple(
                _require_json_safe_nonnegative_int(value) for value in aggregate_rows[0]
            )
            total_rows, valid_rows, missing_rows, failed_rows, direction_rows = counts
            if valid_rows + missing_rows + failed_rows != total_rows:
                raise TxnAmountCoverageIncompleteError()
            (
                aggregate_row_count,
                aggregate_total_rows,
                aggregate_present_rows,
                aggregate_valid_rows,
                aggregate_missing_rows,
                aggregate_failed_rows,
                aggregate_direction_rows,
                aggregate_invalid_rows,
            ) = aggregate_counts
            materialization_identity = _require_exact_identity_text(identity_rows[0][0])
            materialization_version = _require_json_safe_nonnegative_int(identity_rows[0][1])
            materialization_case_id = _require_exact_identity_text(identity_rows[0][2])
            materialization_row_count = _require_json_safe_nonnegative_int(identity_rows[0][3])
            identity_digest = materialization_identity.removeprefix("txn_daily_snapshot:v12:")
            if (
                materialization_version != _AGG_VERSION
                or materialization_case_id != case_key
                or not materialization_identity.startswith("txn_daily_snapshot:v12:")
                or len(identity_digest) != 64
                or any(character not in "0123456789abcdef" for character in identity_digest)
                or materialization_row_count != aggregate_row_count
                or aggregate_invalid_rows != 0
                or aggregate_total_rows != total_rows
                or aggregate_present_rows != valid_rows + failed_rows
                or aggregate_valid_rows != valid_rows
                or aggregate_missing_rows != missing_rows
                or aggregate_failed_rows != failed_rows
                or aggregate_direction_rows != direction_rows
            ):
                raise TxnAmountCoverageIncompleteError()
            coverage = TxnAmountCoverageV1(
                case_id=case_key,
                materialization_identity=materialization_identity,
                total_rows=total_rows,
                amount_valid_rows=valid_rows,
                amount_missing_rows=missing_rows,
                amount_parse_failed_rows=failed_rows,
                direction_covered_rows=direction_rows,
            )
            if not coverage.materialization_identity or not coverage.complete:
                raise TxnAmountCoverageIncompleteError()
            return coverage
        except TxnAmountCoverageIncompleteError:
            raise
        except Exception as exc:
            raise TxnAmountCoverageIncompleteError() from exc
        finally:
            if close_engine:
                try:
                    con.close()
                except Exception:
                    pass

    def query_account_dim_rows(
        self,
        *,
        case_id: str,
        engine: Optional[DuckDBEngine] = None,
    ) -> Optional[list[tuple]]:
        if not self.ensure_materialized(case_id, engine=engine):
            return None

        close_engine = engine is None
        con = engine or self._storage.open_case_engine(case_id, read_only=False)
        try:
            if not self._table_exists(con, _ACCOUNT_DIM_TABLE):
                return None
            return list(
                con.query(
                    f"SELECT account_key, acct_display, card_display, open_name, id_no, bank_name, branch_name, acct_type "
                    f"FROM {_ACCOUNT_DIM_TABLE} ORDER BY account_key"
                )
            )
        except Exception:
            return None
        finally:
            if close_engine:
                try:
                    con.close()
                except Exception:
                    pass

    def _rebuild_materialized(
        self,
        con: DuckDBEngine,
        case_id: str,
        source_revision: int,
        source_snapshot: tuple[int, str, int],
        *,
        profile: Optional[TxnDailyMaterializationProfile] = None,
    ) -> None:
        def record_phase(name: str, started: float) -> None:
            if profile is not None:
                profile.record_since(name, started)

        identity = build_txn_daily_materialization_identity(
            con,
            case_id=case_id,
            source_revision=source_revision,
            source_snapshot=source_snapshot,
        )
        started = time.perf_counter()
        cols = self._table_columns(con, "fc_transaction_norm")
        record_phase("load_columns", started)
        started = time.perf_counter()
        account_columns = self._table_columns(con, "fc_account_norm") if self._table_exists(con, "fc_account_norm") else None
        sub_account_columns = (
            self._table_columns(con, "fc_sub_account_norm") if self._table_exists(con, "fc_sub_account_norm") else None
        )
        plan = build_materialization_plan(
            case_id=case_id,
            txn_columns=cols,
            account_columns=account_columns,
            sub_account_columns=sub_account_columns,
        )
        record_phase("build_plan", started)

        execute_materialization_sql_steps(con, create_materialization_sql_steps(plan), profile=profile)
        started = time.perf_counter()
        execute_materialized_index_sql_steps(con, create_materialized_index_sql_steps(), profile=profile)
        record_phase("create_indexes", started)
        started = time.perf_counter()
        row_count_rows = con.query(f"SELECT COUNT(1) FROM {_AGG_TABLE}")
        if (
            len(row_count_rows) != 1
            or len(row_count_rows[0]) != 1
            or not _is_json_safe_nonnegative_int(row_count_rows[0][0])
        ):
            raise RuntimeError("materialization result row count is unavailable")
        row_count = row_count_rows[0][0]
        current_identity = build_txn_daily_materialization_identity(
            con,
            case_id=case_id,
            source_revision=source_revision,
            source_snapshot=source_snapshot,
        )
        if current_identity != identity:
            raise RuntimeError("materialization source identity changed during rebuild")
        snapshot_row_count, snapshot_max_txn_ts, snapshot_max_id = source_snapshot
        self._replace_materialization_metadata(
            con,
            case_id=case_id,
            identity=identity,
            source_revision=source_revision,
            source_row_count=snapshot_row_count,
            source_max_txn_ts=snapshot_max_txn_ts,
            source_max_id=snapshot_max_id,
            row_count=row_count,
        )
        record_phase("write_meta", started)

    def _replace_materialization_metadata(
        self,
        con: DuckDBEngine,
        *,
        case_id: str,
        identity: TxnDailyMaterializationIdentity,
        source_revision: int,
        source_row_count: int,
        source_max_txn_ts: str,
        source_max_id: int,
        row_count: int,
    ) -> None:
        numeric_metadata = (
            source_revision,
            source_row_count,
            source_max_id,
            row_count,
        )
        if not all(_is_json_safe_nonnegative_int(value) for value in numeric_metadata):
            raise RuntimeError("materialization identity metadata is inconsistent")
        if source_revision <= 0:
            raise RuntimeError("materialization identity metadata is inconsistent")
        if type(case_id) is not str or not case_id or case_id != case_id.strip():
            raise RuntimeError("materialization identity metadata is inconsistent")
        if type(source_max_txn_ts) is not str:
            raise RuntimeError("materialization identity metadata is inconsistent")
        actual_row_count_rows = con.query(f"SELECT COUNT(1) FROM {_AGG_TABLE}")
        if (
            len(actual_row_count_rows) != 1
            or len(actual_row_count_rows[0]) != 1
            or not _is_json_safe_nonnegative_int(actual_row_count_rows[0][0])
            or actual_row_count_rows[0][0] != row_count
        ):
            raise RuntimeError("materialization result row count is inconsistent")
        result_signature = self._materialization_result_signature(con, case_id)
        con.execute(
            f"DELETE FROM {_META_TABLE} WHERE starts_with(agg_name, 'txn_daily_') AND agg_name<>?",
            (identity.value,),
        )
        con.execute(
            f"""
            INSERT INTO {_META_TABLE}(
                agg_name,
                agg_version,
                case_id,
                identity_schema_version,
                source_revision,
                source_row_count,
                source_max_txn_ts,
                source_max_id,
                source_signature,
                result_signature,
                built_at,
                row_count
            )
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
            ON CONFLICT(agg_name) DO NOTHING
            """,
            (
                identity.value,
                _AGG_VERSION,
                case_id,
                _MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
                source_revision,
                source_row_count,
                source_max_txn_ts or None,
                source_max_id,
                identity.source_signature,
                result_signature,
                datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                row_count,
            ),
        )
        rows = con.query(
            f"""
            SELECT agg_version, case_id, identity_schema_version, source_revision,
                   source_row_count, COALESCE(CAST(source_max_txn_ts AS VARCHAR), ''),
                   source_max_id, source_signature, result_signature, row_count
              FROM {_META_TABLE}
             WHERE starts_with(agg_name, 'txn_daily_')
            """
        )
        expected = (
            _AGG_VERSION,
            case_id,
            _MATERIALIZATION_IDENTITY_SCHEMA_VERSION,
            source_revision,
            source_row_count,
            source_max_txn_ts,
            source_max_id,
            identity.source_signature,
            result_signature,
            row_count,
        )
        if (
            len(rows) != 1
            or len(rows[0]) != len(expected)
            or not all(
                _is_json_safe_nonnegative_int(rows[0][index])
                for index in (0, 2, 3, 4, 6, 9)
            )
            or tuple(rows[0]) != expected
        ):
            raise RuntimeError("materialization identity metadata is inconsistent")

    def _is_current(
        self,
        con: DuckDBEngine,
        case_id: str,
        source_revision: int,
        source_snapshot: tuple[int, str, int],
    ) -> bool:
        # Legacy tables, their row counts, and analysis_materialization_meta
        # are locally writable compatibility state.  They cannot establish a
        # current producer manifest or host snapshot authority.  This remains
        # fail-closed until the admitted native verifier supplies that proof.
        del con, case_id, source_revision, source_snapshot
        return False

    @staticmethod
    def _read_existing_source_revision(con: DuckDBEngine) -> int:
        rows = con.query(
            "SELECT revision FROM analysis_revision_state WHERE revision_key='stats_flow_source'"
        )
        if len(rows) != 1 or len(rows[0]) != 1:
            raise TxnAmountCoverageIncompleteError()
        revision = _require_json_safe_nonnegative_int(rows[0][0])
        if revision <= 0:
            raise TxnAmountCoverageIncompleteError()
        return revision

    def _ensure_meta_table(self, con: DuckDBEngine) -> None:
        con.execute(
            f"""
            CREATE TABLE IF NOT EXISTS {_META_TABLE}(
                agg_name TEXT PRIMARY KEY,
                agg_version INTEGER NOT NULL,
                source_revision BIGINT NOT NULL,
                source_row_count BIGINT,
                source_max_txn_ts TIMESTAMP,
                source_max_id BIGINT,
                case_id TEXT,
                identity_schema_version INTEGER,
                source_signature TEXT,
                result_signature TEXT,
                built_at TIMESTAMP,
                row_count BIGINT DEFAULT 0
            )
            """
        )
        cols = self._table_columns(con, _META_TABLE)
        if "source_row_count" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN source_row_count BIGINT")
        if "source_max_txn_ts" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN source_max_txn_ts TIMESTAMP")
        if "source_max_id" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN source_max_id BIGINT")
        if "case_id" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN case_id TEXT")
        if "identity_schema_version" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN identity_schema_version INTEGER")
        if "source_signature" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN source_signature TEXT")
        if "result_signature" not in cols:
            con.execute(f"ALTER TABLE {_META_TABLE} ADD COLUMN result_signature TEXT")

    def _materialization_result_signature(self, con: DuckDBEngine, case_id: str) -> str:
        case_key = str(case_id or "").strip()
        if not case_key:
            raise RuntimeError("materialization result case identity is unavailable")
        digest = hashlib.sha256()

        def update_framed(value: str) -> None:
            encoded = value.encode("utf-8")
            digest.update(len(encoded).to_bytes(8, "big"))
            digest.update(encoded)

        update_framed(_RESULT_SIGNATURE_DOMAIN)
        update_framed(case_key)
        for table, order_by in _RESULT_SIGNATURE_TABLES:
            columns = con.query(
                """
                SELECT column_name
                  FROM information_schema.columns
                 WHERE table_schema='main' AND table_name=?
                 ORDER BY ordinal_position
                """,
                (table,),
            )
            ordered_columns = [str(row[0]) for row in columns if row and row[0]]
            if not ordered_columns:
                raise RuntimeError("materialization result schema is unavailable")
            update_framed(table)
            for column in ordered_columns:
                update_framed(column)
            rows = con.query(
                f"SELECT to_json(r) FROM {table} r ORDER BY {order_by}, to_json(r)"
            )
            for row in rows:
                if len(row) != 1 or row[0] is None:
                    raise RuntimeError("materialization result integrity is unavailable")
                update_framed(str(row[0]))
        return digest.hexdigest()

    def _compute_source_snapshot(self, con: DuckDBEngine, case_id: str) -> tuple[int, str, int]:
        case_key = str(case_id or "").strip()
        if not case_key:
            raise RuntimeError("materialization source snapshot is unavailable")
        rows = con.query(
            """
            SELECT COUNT(1), MAX(txn_ts), MAX(id)
              FROM fc_transaction_norm
             WHERE case_id=?
            """,
            (case_key,),
        )
        if not isinstance(rows, list) or len(rows) != 1:
            raise RuntimeError("materialization source snapshot is unavailable")
        row = rows[0]
        if not isinstance(row, (list, tuple)) or len(row) != 3:
            raise RuntimeError("materialization source snapshot is unavailable")
        row_count_raw, max_txn_ts_raw, max_id_raw = row
        if not _is_json_safe_nonnegative_int(row_count_raw):
            raise RuntimeError("materialization source snapshot is unavailable")
        row_count = row_count_raw
        if row_count == 0:
            if max_txn_ts_raw is not None or max_id_raw is not None:
                raise RuntimeError("materialization source snapshot is unavailable")
            return (0, "", 0)
        if not _is_json_safe_nonnegative_int(max_id_raw):
            raise RuntimeError("materialization source snapshot is unavailable")
        if isinstance(max_txn_ts_raw, bool) or isinstance(max_txn_ts_raw, (dict, list, tuple, set)):
            raise RuntimeError("materialization source snapshot is unavailable")
        max_txn_ts = "" if max_txn_ts_raw is None else str(max_txn_ts_raw)
        max_id = max_id_raw
        return (row_count, max_txn_ts, max_id)

    @classmethod
    def _get_case_lock(cls, case_id: str) -> threading.RLock:
        with cls._guard:
            lock = cls._case_locks.get(case_id)
            if lock is None:
                lock = threading.RLock()
                cls._case_locks[case_id] = lock
            return lock

    @staticmethod
    def _table_exists(con: DuckDBEngine, table: str) -> bool:
        rows = con.query(
            "SELECT COUNT(1) FROM information_schema.tables WHERE table_schema='main' AND table_name=?",
            (table,),
        )
        return (
            len(rows) == 1
            and len(rows[0]) == 1
            and _is_json_safe_nonnegative_int(rows[0][0])
            and rows[0][0] == 1
        )

    @staticmethod
    def _table_columns(con: DuckDBEngine, table: str) -> set[str]:
        rows = con.query(
            "SELECT column_name FROM information_schema.columns WHERE table_schema='main' AND table_name=?",
            (table,),
        )
        return {str(row[0]) for row in rows}
