from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any


def _append_reason(counter: dict[str, int], reason: str) -> None:
    normalized = str(reason or "").strip() or "unknown"
    counter[normalized] = int(counter.get(normalized) or 0) + 1


@dataclass
class WorkspaceJanitorSummary:
    scanned_count: int = 0
    artifact_scanned_count: int = 0
    stale_count: int = 0
    deleted_count: int = 0
    skipped_count: int = 0
    expired_workspace_ids: list[str] = field(default_factory=list)
    deleted_workspace_ids: list[str] = field(default_factory=list)
    deleted_artifact_ids: list[str] = field(default_factory=list)
    skip_reason_counts: dict[str, int] = field(default_factory=dict)

    def add_skip(self, reason: str) -> None:
        self.skipped_count += 1
        _append_reason(self.skip_reason_counts, reason)

    def to_payload(self) -> dict[str, Any]:
        payload = asdict(self)
        payload["skip_reasons"] = [
            {"reason": reason, "count": int(count)}
            for reason, count in sorted(self.skip_reason_counts.items())
        ]
        payload.pop("skip_reason_counts", None)
        return payload
