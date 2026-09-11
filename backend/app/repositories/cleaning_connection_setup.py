from __future__ import annotations

from typing import Any

from app.core.db_engine import configure_duckdb


def configure_cleaning_connection(storage: Any, case_id: str, con: Any) -> None:
    try:
        configure_duckdb(con, case_dir=storage.case_dir(case_id))
    except Exception:
        pass


__all__ = ["configure_cleaning_connection"]
