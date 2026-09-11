from __future__ import annotations

from typing import Any, Callable, Dict, Mapping

from app.repositories.cleaning_legacy_step_spec import LegacyCleaningStepSpec
from app.repositories.cleaning_step_context import CleaningStepContext


AdvanceProgress = Callable[[int, str], None]
PerfCounter = Callable[[], float]


def run_legacy_cleaning_step(
    *,
    executor: Any,
    cur: Any,
    con: Any,
    step_no: int,
    total_rows: int,
    step_specs: Mapping[int, LegacyCleaningStepSpec],
    owner_label: str,
    advance_progress: AdvanceProgress,
    perf_counter: PerfCounter,
) -> Dict[str, Any]:
    step_context = CleaningStepContext.from_executor(executor)
    spec = step_specs.get(step_no)
    if spec is None:
        raise RuntimeError(f"legacy python {owner_label} cleaning does not own step{step_no}")

    t0 = perf_counter()
    raw_result = spec.run(step_context, cur, total_rows)
    con.commit()
    elapsed = perf_counter() - t0
    advance_progress(spec.step_no, spec.format_progress(raw_result, elapsed))
    return spec.map_result(raw_result)


__all__ = [
    "AdvanceProgress",
    "PerfCounter",
    "run_legacy_cleaning_step",
]
