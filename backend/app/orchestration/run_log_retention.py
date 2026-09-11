from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any


RUN_LOG_ANCHOR_STAGES = frozenset(
    {
        "instruction",
        "compact",
        "policy",
        "approval",
        "continuation",
        "final",
    }
)

RUN_LOG_TERMINAL_EVENT_TYPES = frozenset(
    {
        "llm.turn.completed",
        "llm.turn.failed",
    }
)


def is_run_log_anchor(*, event_stage: str = "", event_type: str = "", payload: dict[str, Any] | None = None) -> bool:
    normalized_stage = str(event_stage or "").strip()
    normalized_type = str(event_type or "").strip()
    if normalized_type in RUN_LOG_TERMINAL_EVENT_TYPES:
        return True
    if normalized_stage in RUN_LOG_ANCHOR_STAGES:
        return True
    payload_dict = dict(payload or {})
    return bool(
        payload_dict.get("terminal_reason_present")
        or payload_dict.get("stop_reason")
        or payload_dict.get("stop_code")
        or payload_dict.get("blocked_reason")
    )


def _append_reason(counter: dict[str, int], reason: str) -> None:
    normalized = str(reason or "").strip() or "unknown"
    counter[normalized] = int(counter.get(normalized) or 0) + 1


@dataclass
class RunLogRetentionSummary:
    scanned_count: int = 0
    stale_count: int = 0
    deleted_count: int = 0
    skipped_count: int = 0
    preserved_anchor_count: int = 0
    deleted_event_ids: list[str] = field(default_factory=list)
    stale_reason_counts: dict[str, int] = field(default_factory=dict)
    skip_reason_counts: dict[str, int] = field(default_factory=dict)

    def add_stale(self, reason: str) -> None:
        self.stale_count += 1
        _append_reason(self.stale_reason_counts, reason)

    def add_skip(self, reason: str) -> None:
        self.skipped_count += 1
        _append_reason(self.skip_reason_counts, reason)

    def to_payload(self) -> dict[str, Any]:
        payload = asdict(self)
        payload["stale_reasons"] = [
            {"reason": reason, "count": int(count)}
            for reason, count in sorted(self.stale_reason_counts.items())
        ]
        payload["skip_reasons"] = [
            {"reason": reason, "count": int(count)}
            for reason, count in sorted(self.skip_reason_counts.items())
        ]
        payload.pop("stale_reason_counts", None)
        payload.pop("skip_reason_counts", None)
        return payload
