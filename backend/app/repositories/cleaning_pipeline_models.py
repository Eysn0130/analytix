from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from app.repositories import cleaning_native_execution


class NativeCleaningAction(str, Enum):
    CLEAN_ALL = "clean_all"
    AMOUNT_BALANCE = "amount_balance"
    QUALITY_FLAGS = "quality_flags"
    DC_FLAG = "dc_flag"
    ACCOUNT_KEYS = "account_keys"
    ACCOUNT_INFO = "account_info"


@dataclass(frozen=True)
class CleaningPipelineSegment:
    command_attr: str
    execution_spec: cleaning_native_execution.NativeExecutionSpec
    step_numbers: tuple[int, ...]
    native_action: NativeCleaningAction


__all__ = [
    "CleaningPipelineSegment",
    "NativeCleaningAction",
]
