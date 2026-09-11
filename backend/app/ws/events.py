from datetime import datetime, timezone
from typing import Any, Optional


def build_event(
    event: str,
    payload: dict[str, Any],
    *,
    event_type: str,
    channel: str,
    sequence: int,
    version: str = "v1",
    job_id: Optional[str] = None,
    case_id: Optional[str] = None,
) -> dict[str, Any]:
    return {
        "version": version,
        "event": event,
        "type": event_type,
        "channel": channel,
        "job_id": job_id,
        "case_id": case_id,
        "sequence": sequence,
        "payload": payload,
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }
