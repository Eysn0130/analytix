from __future__ import annotations

import time
from dataclasses import dataclass, field
from typing import Any, Callable, Optional


ProfileCallback = Callable[[dict[str, Any]], None]

SETUP_PHASES = frozenset(
    {
        "ensure_meta_table",
        "compute_source_snapshot",
        "check_current",
        "load_columns",
        "build_plan",
    }
)
MATERIALIZATION_PHASES = frozenset(
    {
        "drop_staging_tables",
        "build_daily_agg",
        "build_detail_idx",
        "build_account_dim",
        "swap_tables",
    }
)
METADATA_PHASES = frozenset({"write_meta", "commit"})
NATIVE_PHASES = frozenset({"native_materialize"})


@dataclass
class TxnDailyMaterializationProfile:
    started_at: float = field(default_factory=time.perf_counter)
    phases: dict[str, float] = field(default_factory=dict)
    index_phases: dict[str, float] = field(default_factory=dict)
    native_phases: dict[str, float] = field(default_factory=dict)
    native_phase_groups: dict[str, float] = field(default_factory=dict)

    def record_since(self, name: str, started_at: float) -> None:
        self.record_elapsed(name, time.perf_counter() - started_at)

    def record_elapsed(self, name: str, elapsed_s: float) -> None:
        if elapsed_s < 0:
            return
        phase = str(name or "").strip()
        if not phase:
            return
        self.phases[phase] = self.phases.get(phase, 0.0) + float(elapsed_s)

    def record_index_elapsed(self, name: str, elapsed_s: float) -> None:
        if elapsed_s < 0:
            return
        phase = str(name or "").strip()
        if not phase:
            return
        self.index_phases[phase] = self.index_phases.get(phase, 0.0) + float(elapsed_s)

    def record_native_profile(self, payload: dict[str, Any]) -> None:
        self.native_phases.update(_float_map(payload.get("phases_s") or {}))
        self.native_phase_groups.update(_float_map(payload.get("phase_groups_s") or {}))

    def as_dict(self, *, ok: bool, native_used: bool = False, wall_s: Optional[float] = None) -> dict[str, Any]:
        rounded = {key: round(float(value or 0), 6) for key, value in sorted(self.phases.items())}
        rounded_index = {key: round(float(value or 0), 6) for key, value in sorted(self.index_phases.items())}
        rounded_native = {key: round(float(value or 0), 6) for key, value in sorted(self.native_phases.items())}
        rounded_native_groups = {key: round(float(value or 0), 6) for key, value in sorted(self.native_phase_groups.items())}
        elapsed_wall = time.perf_counter() - self.started_at if wall_s is None else float(wall_s or 0)
        output = {
            "ok": bool(ok),
            "native_used": bool(native_used),
            "phases_s": rounded,
            "index_phases_s": rounded_index,
            "phase_groups_s": summarize_phase_groups(rounded, rounded_index),
            "total_phase_s": round(sum(rounded.values()), 6),
            "wall_s": round(max(0.0, elapsed_wall), 6),
        }
        if rounded_native:
            output["native_phases_s"] = rounded_native
        if rounded_native_groups:
            output["native_phase_groups_s"] = rounded_native_groups
        return output

    def emit(self, callback: Optional[ProfileCallback], *, ok: bool, native_used: bool = False) -> None:
        if callback:
            callback(self.as_dict(ok=ok, native_used=native_used))


def summarize_phase_groups(phases: dict[str, Any], index_phases: dict[str, Any]) -> dict[str, float]:
    phase_values = {str(key): float(value or 0) for key, value in dict(phases or {}).items()}
    index_total = phase_values.get("create_indexes")
    if index_total is None:
        index_total = sum(float(value or 0) for value in dict(index_phases or {}).values())
    materialization_sql = sum(value for key, value in phase_values.items() if key in MATERIALIZATION_PHASES)
    setup = sum(value for key, value in phase_values.items() if key in SETUP_PHASES)
    metadata = sum(value for key, value in phase_values.items() if key in METADATA_PHASES)
    native = sum(value for key, value in phase_values.items() if key in NATIVE_PHASES)
    known = setup + materialization_sql + index_total + metadata + native
    other = max(0.0, sum(phase_values.values()) - known)
    groups: dict[str, float] = {}
    for key, value in {
        "setup": setup,
        "materialization_sql": materialization_sql,
        "index": index_total,
        "metadata": metadata,
        "native": native,
        "other": other,
    }.items():
        rounded_value = round(value, 6)
        if rounded_value > 0:
            groups[key] = rounded_value
    return groups


def _float_map(values: dict[str, Any]) -> dict[str, float]:
    mapped: dict[str, float] = {}
    for key, value in dict(values or {}).items():
        try:
            mapped[str(key)] = float(value or 0)
        except (TypeError, ValueError):
            continue
    return mapped
