from __future__ import annotations

import time
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Iterator


@dataclass
class ImportPreviewProfiler:
    """Lightweight preview timing/counter collector for smoke diagnostics."""

    started_at: float = field(default_factory=time.perf_counter)
    phases: dict[str, float] = field(default_factory=dict)
    counters: dict[str, int] = field(default_factory=dict)
    tabular_scans: list[dict[str, object]] = field(default_factory=list)

    def count(self, name: str, delta: int = 1) -> None:
        self.counters[name] = int(self.counters.get(name, 0)) + int(delta)

    def add_phase(self, name: str, elapsed_s: float) -> None:
        if elapsed_s < 0:
            return
        self.phases[name] = float(self.phases.get(name, 0.0)) + float(elapsed_s)

    def record_tabular_scan(
        self,
        *,
        label: str,
        elapsed_s: float,
        rows_total: int,
        columns_total: int,
        profile_columns_total: int,
        profile_ready: bool,
    ) -> None:
        self.tabular_scans.append(
            {
                "label": str(label or "").strip(),
                "elapsed_s": max(0.0, float(elapsed_s or 0.0)),
                "rows_total": max(0, int(rows_total or 0)),
                "columns_total": max(0, int(columns_total or 0)),
                "profile_columns_total": max(0, int(profile_columns_total or 0)),
                "profile_ready": bool(profile_ready),
            }
        )

    @contextmanager
    def time_phase(self, name: str) -> Iterator[None]:
        started = time.perf_counter()
        try:
            yield
        finally:
            self.add_phase(name, time.perf_counter() - started)

    def as_dict(self) -> dict[str, object]:
        phases_s = {
            name: round(elapsed_s, 6)
            for name, elapsed_s in sorted(self.phases.items())
        }
        top_phases = [
            {"phase": name, "elapsed_s": round(elapsed_s, 6)}
            for name, elapsed_s in sorted(self.phases.items(), key=lambda item: item[1], reverse=True)[:10]
        ]
        tabular_scans = [
            {
                **scan,
                "elapsed_s": round(float(scan.get("elapsed_s") or 0.0), 6),
            }
            for scan in self.tabular_scans
        ]
        top_tabular_scans = sorted(tabular_scans, key=lambda item: float(item.get("elapsed_s") or 0.0), reverse=True)[:10]
        return {
            "wall_s": round(time.perf_counter() - self.started_at, 6),
            "total_phase_s": round(sum(self.phases.values()), 6),
            "phases_s": phases_s,
            "top_phases": top_phases,
            "counters": dict(sorted(self.counters.items())),
            "tabular_scan_summary": {
                "files": len(tabular_scans),
                "rows_total": sum(int(scan.get("rows_total") or 0) for scan in tabular_scans),
                "elapsed_s": round(sum(float(scan.get("elapsed_s") or 0.0) for scan in tabular_scans), 6),
            },
            "top_tabular_scans": top_tabular_scans,
            "tabular_scans": tabular_scans,
        }
