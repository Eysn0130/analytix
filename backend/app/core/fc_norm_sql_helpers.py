from __future__ import annotations

from collections.abc import Iterable, Sequence


def select_list_sql(items: Iterable[str]) -> str:
    return ", ".join(items)


def dedupe_preserve_order(values: Sequence[str]) -> tuple[str, ...]:
    seen: set[str] = set()
    result: list[str] = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        result.append(value)
    return tuple(result)
