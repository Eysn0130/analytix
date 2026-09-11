from __future__ import annotations

from dataclasses import dataclass, field
from typing import Iterable, Mapping


@dataclass(frozen=True)
class ImportTimingProfile:
    phases: dict[str, float] = field(default_factory=dict)
    rows_seen: int = 0
    rows_imported: int = 0
    chunks: int = 0
    files: int = 1
    wall_s: float = 0.0

    def as_dict(self) -> dict:
        phases = {key: round(float(value or 0), 6) for key, value in sorted(self.phases.items())}
        total_s = round(sum(phases.values()), 6)
        out = {
            "phases_s": phases,
            "total_phase_s": total_s,
            "rows_seen": int(self.rows_seen or 0),
            "rows_imported": int(self.rows_imported or 0),
            "chunks": int(self.chunks or 0),
            "files": int(self.files or 0),
            "wall_s": round(float(self.wall_s or 0), 6),
        }
        if total_s > 0:
            out["rows_per_phase_second"] = round(int(self.rows_imported or 0) / total_s, 1)
        return out


def add_phase(phases: dict[str, float], name: str, elapsed_s: float) -> None:
    if elapsed_s < 0:
        return
    phases[name] = phases.get(name, 0.0) + float(elapsed_s)


def merge_timing_profiles(profiles: Iterable[Mapping]) -> dict:
    phases: dict[str, float] = {}
    rows_seen = 0
    rows_imported = 0
    chunks = 0
    files = 0
    wall_s = 0.0
    for profile in profiles:
        if not profile:
            continue
        for key, value in dict(profile.get("phases_s") or {}).items():
            phases[str(key)] = phases.get(str(key), 0.0) + float(value or 0)
        rows_seen += int(profile.get("rows_seen") or 0)
        rows_imported += int(profile.get("rows_imported") or 0)
        chunks += int(profile.get("chunks") or 0)
        files += int(profile.get("files") or 0)
        wall_s += float(profile.get("wall_s") or 0)
    return ImportTimingProfile(
        phases=phases,
        rows_seen=rows_seen,
        rows_imported=rows_imported,
        chunks=chunks,
        files=files,
        wall_s=wall_s,
    ).as_dict()
