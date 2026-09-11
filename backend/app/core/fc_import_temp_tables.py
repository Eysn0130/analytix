from __future__ import annotations

import re
import time
from dataclasses import dataclass
from typing import MutableMapping, Sequence

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_timing import add_phase


def safe_import_temp_name(prefix: str, file_id: str) -> str:
    token = re.sub(r"[^A-Za-z0-9_]+", "_", file_id or "")
    token = token.strip("_") or "tmp"
    return f"{prefix}_{token[:16]}"


@dataclass(frozen=True)
class ImportTempTables:
    staging_table: str
    dedup_table: str


def build_import_temp_tables(*, schema_table: str, file_id: str) -> ImportTempTables:
    file_token = safe_import_temp_name("tmp", file_id)
    return ImportTempTables(
        staging_table=f"stg_{schema_table}_{file_token}",
        dedup_table=f"dedup_{schema_table}_{file_token}",
    )


def drop_table_if_exists(engine: DuckDBEngine, table: str) -> None:
    try:
        engine.execute(f"DROP TABLE IF EXISTS {table}")
    except Exception:
        pass


def drop_import_temp_tables(
    *,
    engine: DuckDBEngine,
    tables: Sequence[str],
    timings: MutableMapping[str, float],
) -> None:
    started = time.perf_counter()
    for table in tables:
        drop_table_if_exists(engine, table)
    add_phase(timings, "drop_temp_s", time.perf_counter() - started)
