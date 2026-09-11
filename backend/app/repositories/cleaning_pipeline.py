from __future__ import annotations

from typing import Any, Dict, Optional, Sequence

from app.repositories.cleaning_native_invoker import CleaningNativeInvoker
from app.repositories.cleaning_pipeline_models import CleaningPipelineSegment
from app.repositories.cleaning_pipeline_steps import (
    CLEAN_ALL_SEGMENT,
    STEPWISE_SEGMENTS,
)


def execute_cleaning_pipeline(
    *,
    executor: Any,
    native_plan: Any,
    native_runner: Any,
) -> None:
    native_invoker = CleaningNativeInvoker(executor=executor)
    native_clean_all = _execute_native_segment(
        native_invoker=native_invoker,
        native_runner=native_runner,
        segment=CLEAN_ALL_SEGMENT,
        command=native_plan.clean_all,
    )

    for segment in STEPWISE_SEGMENTS:
        native_command = (
            getattr(native_plan, segment.command_attr)
            if native_clean_all is None
            else ()
        )
        if native_clean_all is None:
            _require_native_command_for_active_steps(
                segment=segment,
                command=native_command,
                active_steps=native_plan.active_step_set,
            )
        _execute_native_segment(
            native_invoker=native_invoker,
            native_runner=native_runner,
            segment=segment,
            command=native_command,
        )


def _execute_native_segment(
    *,
    native_invoker: CleaningNativeInvoker,
    native_runner: Any,
    segment: CleaningPipelineSegment,
    command: Sequence[str],
) -> Optional[Dict[str, int]]:
    return native_runner.execute(
        segment.execution_spec,
        command,
        lambda argv: native_invoker.run(segment.native_action, argv),
    )


def _require_native_command_for_active_steps(
    *,
    segment: CleaningPipelineSegment,
    command: Sequence[str],
    active_steps: set[int],
) -> None:
    if command:
        return
    missing_steps = tuple(step for step in segment.step_numbers if int(step) in active_steps)
    if not missing_steps:
        return
    step_label = ",".join(str(step) for step in missing_steps)
    raise RuntimeError(
        "native cleaning command unavailable for step(s) "
        f"{step_label}; legacy Python cleaning fallback is retired"
    )


__all__ = ["execute_cleaning_pipeline"]
