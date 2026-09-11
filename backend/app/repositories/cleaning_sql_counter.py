from __future__ import annotations

from typing import Any, Tuple


class CleaningCountUnavailableError(RuntimeError):
    pass


def require_non_negative_count(value: Any, *, code: str = "cleaning_count_unavailable") -> int:
    if type(value) is not int or value < 0 or value > 9_223_372_036_854_775_807:
        raise CleaningCountUnavailableError(code)
    return value


def require_count_row(row: Any, *, code: str = "cleaning_count_unavailable") -> int:
    return require_count_values(row, width=1, code=code)[0]


def require_count_values(
    row: Any,
    *,
    width: int,
    code: str = "cleaning_count_unavailable",
) -> Tuple[int, ...]:
    if not isinstance(row, (list, tuple)) or len(row) != width:
        raise CleaningCountUnavailableError(code)
    return tuple(require_non_negative_count(value, code=code) for value in row)


def changes(cur: Any) -> int:
    try:
        cur.execute("SELECT changes()")
        row = cur.fetchone()
        return require_count_row(row, code="cleaning_changes_count_unavailable")
    except Exception:
        raise CleaningCountUnavailableError("cleaning_changes_count_unavailable") from None


def update_count(cur: Any, sql: str, params: Tuple[Any, ...]) -> int:
    stmt = sql.strip().rstrip(";")
    try:
        cur.execute(f"SELECT COUNT(1) FROM ({stmt} RETURNING 1) AS _u", params)
    except Exception:
        pass
    else:
        return require_count_row(
            cur.fetchone(),
            code="cleaning_update_count_unavailable",
        )
    try:
        cur.execute(f"{stmt} RETURNING 1", params)
    except Exception:
        raise CleaningCountUnavailableError("cleaning_update_count_unavailable") from None
    try:
        rows = cur.fetchall()
    except Exception:
        raise CleaningCountUnavailableError("cleaning_update_count_unavailable") from None
    if not isinstance(rows, list):
        raise CleaningCountUnavailableError("cleaning_update_count_unavailable")
    return require_non_negative_count(
        len(rows),
        code="cleaning_update_count_unavailable",
    )


__all__ = [
    "CleaningCountUnavailableError",
    "changes",
    "require_count_row",
    "require_count_values",
    "require_non_negative_count",
    "update_count",
]
