from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, List


@dataclass
class CleaningScopeState:
    txn_where_base: str = ""
    txn_where_base_params: List[Any] = field(default_factory=list)
    txn_where: str = ""
    txn_params: List[Any] = field(default_factory=list)
    txn_where_t: str = ""
    acc_where: str = ""
    acc_where_a: str = ""
    acc_params: List[Any] = field(default_factory=list)
    scope_file_ids: List[str] = field(default_factory=list)
    txn_scope_file_ids: List[str] = field(default_factory=list)
    acc_scope_file_ids: List[str] = field(default_factory=list)


__all__ = ["CleaningScopeState"]
