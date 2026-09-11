from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence


@dataclass(frozen=True)
class MaterializationSqlStatement:
    sql: str
    params: tuple = ()


@dataclass(frozen=True)
class MaterializationSqlStep:
    phase_name: str
    statements: tuple[MaterializationSqlStatement, ...]


def statements_from_sql(sql_items: Sequence[str]) -> tuple[MaterializationSqlStatement, ...]:
    return tuple(MaterializationSqlStatement(sql) for sql in sql_items)
