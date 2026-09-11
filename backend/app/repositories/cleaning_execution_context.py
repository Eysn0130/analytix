from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Optional


CancelCheck = Callable[[], bool]
ProgressCallback = Callable[[int, str, Optional[int]], None]
LogCallback = Callable[[str, str], None]


class CleaningCancelled(Exception):
    pass


@dataclass
class CleaningExecutionContext:
    cancel_check: Optional[CancelCheck] = None
    progress_cb: Optional[ProgressCallback] = None
    log_cb: Optional[LogCallback] = None
    allow_legacy_python_cleaning: bool = False

    def check_cancel(self) -> None:
        if self.cancel_check is not None and self.cancel_check():
            raise CleaningCancelled()

    def emit_progress(
        self,
        step_idx: int,
        step_total: int,
        msg: str,
        processed: int = 0,
        total: int = 0,
    ) -> int:
        if total and total > 0:
            pct = int(((step_idx - 1) + (processed / total)) / max(step_total, 1) * 100)
        else:
            pct = int(step_idx / max(step_total, 1) * 100)
        if step_idx >= step_total:
            pct = min(pct, 99)
        if self.progress_cb:
            self.progress_cb(max(0, min(100, pct)), msg, step_idx)
        return pct

    def emit_final_progress(self, msg: str) -> None:
        if self.progress_cb:
            self.progress_cb(100, msg, None)

    def emit_log(self, step_id: str, msg: str) -> None:
        if self.log_cb:
            self.log_cb(step_id, msg)

    def legacy_python_cleaning_allowed(self) -> bool:
        return self.allow_legacy_python_cleaning


__all__ = [
    "CancelCheck",
    "CleaningCancelled",
    "CleaningExecutionContext",
    "LogCallback",
    "ProgressCallback",
]
