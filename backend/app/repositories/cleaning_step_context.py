from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from app.repositories.cleaning_execution_context import CleaningExecutionContext
from app.repositories.cleaning_scope_state import CleaningScopeState


@dataclass(frozen=True)
class CleaningStepContext:
    case_id: str
    scope_state: CleaningScopeState
    execution_context: CleaningExecutionContext

    @classmethod
    def from_executor(cls, executor: Any) -> "CleaningStepContext":
        return cls(
            case_id=executor.case_id,
            scope_state=executor.scope_state,
            execution_context=executor.execution_context,
        )

    def check_cancel(self) -> None:
        self.execution_context.check_cancel()

    def emit_progress(
        self,
        step_idx: int,
        step_total: int,
        msg: str,
        processed: int = 0,
        total: int = 0,
    ) -> int:
        return self.execution_context.emit_progress(
            step_idx,
            step_total,
            msg,
            processed,
            total,
        )


__all__ = ["CleaningStepContext"]
