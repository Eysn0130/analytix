from __future__ import annotations

import time
from dataclasses import dataclass
from typing import MutableMapping

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_projection import FcProjectionSchema, raw_col
from app.core.fc_import_timing import add_phase
from app.core.import_count_semantics import known_public_import_count


@dataclass(frozen=True)
class ImportStagingMetricsRequest:
    schema: FcProjectionSchema
    staging_table: str


@dataclass(frozen=True)
class ImportStagingMetrics:
    total_rows: int
    key_missing: dict[str, float]


def collect_import_staging_metrics(
    *,
    engine: DuckDBEngine,
    request: ImportStagingMetricsRequest,
    timings: MutableMapping[str, float],
) -> ImportStagingMetrics:
    if request.schema.table == "fc_transaction":
        return _collect_transaction_metrics(engine=engine, request=request, timings=timings)
    return ImportStagingMetrics(
        total_rows=_count_staging_rows(engine=engine, staging_table=request.staging_table, timings=timings),
        key_missing={},
    )


def _collect_transaction_metrics(
    *,
    engine: DuckDBEngine,
    request: ImportStagingMetricsRequest,
    timings: MutableMapping[str, float],
) -> ImportStagingMetrics:
    started = time.perf_counter()
    raw_txn = raw_col("txn_time")
    raw_amt = raw_col("amount")
    raw_card = raw_col("card_no")
    raw_acct = raw_col("acct_no")
    rows = engine.query(
        f"SELECT "
        f"COUNT(1) AS total_rows, "
        f"COUNT(1) FILTER (WHERE COALESCE(TRIM({raw_txn}), '')='') AS miss_time, "
        f"COUNT(1) FILTER (WHERE COALESCE(TRIM({raw_amt}), '')='') AS miss_amt, "
        f"COUNT(1) FILTER (WHERE COALESCE(TRIM({raw_card}), '')='' "
        f"  AND COALESCE(TRIM({raw_acct}), '')='') AS miss_acct "
        f"FROM {request.staging_table}"
    )
    row = _require_single_metric_row(rows, field="transaction_staging_metrics", width=4)
    total_rows = _require_count(row[0], field="transaction_staging_total_rows")
    miss_time = _require_count(row[1], field="transaction_staging_missing_time")
    miss_amt = _require_count(row[2], field="transaction_staging_missing_amount")
    miss_acct = _require_count(row[3], field="transaction_staging_missing_account")
    if any(value > total_rows for value in (miss_time, miss_amt, miss_acct)):
        raise ValueError("transaction_staging_missing_count_exceeds_total")

    key_missing: dict[str, float] = {}
    if total_rows > 0:
        key_missing = {
            "交易时间": miss_time / total_rows,
            "交易金额": miss_amt / total_rows,
            "交易账号/卡号": miss_acct / total_rows,
        }
    add_phase(timings, "staging_metrics_s", time.perf_counter() - started)
    return ImportStagingMetrics(total_rows=total_rows, key_missing=key_missing)


def _count_staging_rows(
    *,
    engine: DuckDBEngine,
    staging_table: str,
    timings: MutableMapping[str, float],
) -> int:
    started = time.perf_counter()
    rows = engine.query(f"SELECT COUNT(1) FROM {staging_table}")
    row = _require_single_metric_row(rows, field="staging_count", width=1)
    total_rows = _require_count(row[0], field="staging_total_rows")
    add_phase(timings, "count_rows_s", time.perf_counter() - started)
    return total_rows


def _require_single_metric_row(
    rows: object,
    *,
    field: str,
    width: int,
) -> tuple[object, ...]:
    if not isinstance(rows, list) or len(rows) != 1:
        raise ValueError(f"{field}_row_unavailable")
    row = rows[0]
    if not isinstance(row, (list, tuple)) or len(row) != width:
        raise ValueError(f"{field}_row_invalid")
    return tuple(row)


def _require_count(value: object, *, field: str) -> int:
    normalized = known_public_import_count(value)
    if normalized is None:
        raise ValueError(f"{field}_count_invalid")
    return normalized
