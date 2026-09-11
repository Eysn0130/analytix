from __future__ import annotations

from typing import Any


def table_exists(cur: Any, name: str) -> bool:
    cur.execute(
        "SELECT 1 FROM information_schema.tables WHERE table_schema='main' AND table_name=? LIMIT 1",
        (name,),
    )
    return cur.fetchone() is not None


def column_exists(cur: Any, table: str, name: str) -> bool:
    try:
        cur.execute(
            "SELECT 1 FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=? AND column_name=? LIMIT 1",
            (table, name),
        )
        return cur.fetchone() is not None
    except Exception:
        return False


def column_names(cur: Any, table: str) -> set[str]:
    try:
        cur.execute(
            "SELECT column_name FROM information_schema.columns "
            "WHERE table_schema='main' AND table_name=?",
            (table,),
        )
        return {str(row[0]) for row in cur.fetchall() if row and row[0]}
    except Exception:
        return set()


__all__ = ["column_exists", "column_names", "table_exists"]
