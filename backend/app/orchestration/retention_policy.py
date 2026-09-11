from __future__ import annotations

from dataclasses import asdict, dataclass


@dataclass(frozen=True)
class RuntimeRetentionPolicy:
    scratchpad_delete_grace_hours: int = 72
    skill_cache_ttl_hours: int = 24
    scope_cache_ttl_hours: int = 48
    working_memory_delete_grace_days: int = 7
    run_log_ttl_days: int = 7

    def to_payload(self) -> dict[str, int]:
        return asdict(self)


DEFAULT_RUNTIME_RETENTION_POLICY = RuntimeRetentionPolicy()
