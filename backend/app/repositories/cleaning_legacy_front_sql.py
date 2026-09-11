from __future__ import annotations

import time
from typing import Any, Dict, Sequence

from app.repositories.cleaning_legacy_front_specs import FRONT_LEGACY_STEP_SPECS
from app.repositories.cleaning_legacy_step_runner import AdvanceProgress, run_legacy_cleaning_step


def run_legacy_python_front_step(
    executor: Any,
    *,
    cur: Any,
    con: Any,
    step_no: int,
    total_rows: int,
    native_command: Sequence[str],
    advance_progress: AdvanceProgress,
) -> Dict[str, Any]:
    if native_command:
        raise RuntimeError(
            f"legacy python front cleaning step{step_no} blocked because native command is available"
        )

    return run_legacy_cleaning_step(
        executor=executor,
        cur=cur,
        con=con,
        step_no=step_no,
        total_rows=total_rows,
        step_specs=FRONT_LEGACY_STEP_SPECS,
        owner_label="front",
        advance_progress=advance_progress,
        perf_counter=time.perf_counter,
    )


__all__ = ["run_legacy_python_front_step"]
