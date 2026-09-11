from __future__ import annotations

import time
from typing import Any


class AnalysisComputeDiagnostics:
    def __init__(self) -> None:
        self._started_at = time.perf_counter()
        self._stages: list[dict[str, Any]] = []
        self._metrics: dict[str, Any] = {}
        self._total_ms = 0.0

    def mark(self) -> float:
        return time.perf_counter()

    def record_elapsed(self, stage: str, started_at: float) -> None:
        self._stages.append(
            {
                "stage": str(stage),
                "duration_ms": _round_ms((time.perf_counter() - started_at) * 1000.0),
            }
        )

    def record_metric(self, name: str, value: Any) -> None:
        self._metrics[str(name)] = value

    def finish(self) -> None:
        self._total_ms = _round_ms((time.perf_counter() - self._started_at) * 1000.0)

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {
            "total_ms": self._total_ms,
            "stages": list(self._stages),
        }
        if self._metrics:
            out["metrics"] = dict(self._metrics)
        return out


def merge_analysis_compute_diagnostics(
    payload: dict[str, Any] | None,
    *,
    section: str,
    diagnostics: AnalysisComputeDiagnostics,
) -> dict[str, Any] | None:
    if not isinstance(payload, dict):
        return payload
    current = payload.get("diagnostics")
    root = dict(current) if isinstance(current, dict) else {}
    root[str(section)] = diagnostics.to_dict()
    payload["diagnostics"] = root
    return payload


def _round_ms(value: float) -> float:
    return round(float(value), 3)
