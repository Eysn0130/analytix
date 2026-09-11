from __future__ import annotations

from app.repositories import cleaning_native_execution
from app.repositories.cleaning_pipeline_models import (
    CleaningPipelineSegment,
    NativeCleaningAction,
)


CLEAN_ALL_SEGMENT = CleaningPipelineSegment(
    command_attr="clean_all",
    execution_spec=cleaning_native_execution.CLEAN_ALL,
    step_numbers=tuple(range(1, 11)),
    native_action=NativeCleaningAction.CLEAN_ALL,
)

STEPWISE_SEGMENTS: tuple[CleaningPipelineSegment, ...] = (
    CleaningPipelineSegment(
        command_attr="amount_balance",
        execution_spec=cleaning_native_execution.AMOUNT_BALANCE,
        step_numbers=(1,),
        native_action=NativeCleaningAction.AMOUNT_BALANCE,
    ),
    CleaningPipelineSegment(
        command_attr="quality_flags",
        execution_spec=cleaning_native_execution.QUALITY_FLAGS,
        step_numbers=(2, 3, 4),
        native_action=NativeCleaningAction.QUALITY_FLAGS,
    ),
    CleaningPipelineSegment(
        command_attr="dc_flag",
        execution_spec=cleaning_native_execution.DC_FLAG,
        step_numbers=(5,),
        native_action=NativeCleaningAction.DC_FLAG,
    ),
    CleaningPipelineSegment(
        command_attr="account_keys",
        execution_spec=cleaning_native_execution.ACCOUNT_KEYS,
        step_numbers=(6, 7, 8, 9),
        native_action=NativeCleaningAction.ACCOUNT_KEYS,
    ),
    CleaningPipelineSegment(
        command_attr="account_info",
        execution_spec=cleaning_native_execution.ACCOUNT_INFO,
        step_numbers=(10,),
        native_action=NativeCleaningAction.ACCOUNT_INFO,
    ),
)


__all__ = [
    "CLEAN_ALL_SEGMENT",
    "STEPWISE_SEGMENTS",
]
