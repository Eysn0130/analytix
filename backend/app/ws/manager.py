from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
import logging
import re
from typing import Dict, Optional

from fastapi import WebSocket

from app.core.safe_observability import log_closed_diagnostic


_CASE_ID_PATTERN = re.compile(r"^[A-Za-z0-9_-]{4,80}$")
_RAW_DIAGNOSTIC_KEYS = frozenset(
    {
        "error",
        "exception",
        "exc_info",
        "stack",
        "stack_info",
        "traceback",
    }
)
_PUBLIC_EVENT_FIELDS = (
    "version",
    "event",
    "type",
    "channel",
    "job_id",
    "case_id",
    "sequence",
    "payload",
    "timestamp",
)
_CLOSED_ERROR_PAYLOAD = {
    "code": "INTERNAL_ERROR",
    "message": "operation failed",
    "retryable": True,
    "details": {},
}
_PUBLIC_OPERATIONAL_PAYLOAD_FIELDS = frozenset(
    {
        "contract",
        "event_binding_hash",
        "event_id",
        "session_id",
        "status",
        "stage",
        "level",
        "message_code",
        "code",
        "progress",
        "step",
        "retryable",
        "counters",
    }
)
_PUBLIC_TOKEN_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$")
_PUBLIC_CODE_RE = re.compile(r"^[A-Za-z][A-Za-z0-9_.:-]{0,79}$")
_PUBLIC_TIMESTAMP_RE = re.compile(r"^[0-9]{4}-[0-9T:+.Z-]{6,36}$")
_PUBLIC_EVENT_TYPES = frozenset({"info", "progress", "success", "warning", "error"})
_PUBLIC_CHANNELS = frozenset({"analysis", "cleaning", "export", "import", "system"})


def _normalize_case_id(value: object) -> str:
    normalized = str(value or "").strip()
    if _CASE_ID_PATTERN.fullmatch(normalized) is None:
        return ""
    return normalized


def _contains_raw_diagnostic(value: object) -> bool:
    if isinstance(value, dict):
        for key, item in value.items():
            if str(key).strip().lower() in _RAW_DIAGNOSTIC_KEYS:
                return True
            if _contains_raw_diagnostic(item):
                return True
    elif isinstance(value, (list, tuple)):
        return any(_contains_raw_diagnostic(item) for item in value)
    return False


def _bounded_non_negative_int(value: object, *, maximum: int) -> Optional[int]:
    if isinstance(value, bool):
        return None
    try:
        normalized = int(value)
    except (TypeError, ValueError, OverflowError):
        return None
    if normalized < 0 or normalized > maximum:
        return None
    return normalized


def _project_public_payload(value: object) -> dict:
    payload = value if isinstance(value, dict) else {}
    projected: dict[str, object] = {
        "contract": "OperationalWebSocketEventV1",
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
    }
    for key in _PUBLIC_OPERATIONAL_PAYLOAD_FIELDS:
        item = payload.get(key)
        if key in {"progress", "step"}:
            normalized = _bounded_non_negative_int(item, maximum=1_000_000_000)
            if normalized is not None:
                projected[key] = normalized
            continue
        if key == "retryable":
            if isinstance(item, bool):
                projected[key] = item
            continue
        if key == "counters":
            if not isinstance(item, dict):
                continue
            counters: dict[str, int] = {}
            for counter_key, counter_value in item.items():
                normalized_key = str(counter_key or "").strip()
                normalized_value = _bounded_non_negative_int(
                    counter_value,
                    maximum=1_000_000_000,
                )
                if (
                    _PUBLIC_CODE_RE.fullmatch(normalized_key)
                    and normalized_value is not None
                    and len(counters) < 32
                ):
                    counters[normalized_key] = normalized_value
            if counters:
                projected[key] = counters
            continue
        normalized_text = str(item or "").strip() if type(item) is str else ""
        pattern = _PUBLIC_TOKEN_RE if key in {"event_binding_hash", "event_id", "session_id"} else _PUBLIC_CODE_RE
        if normalized_text and pattern.fullmatch(normalized_text):
            projected[key] = normalized_text
    return projected


def _project_public_event(event: object) -> Optional[dict]:
    if not isinstance(event, dict):
        return None
    case_id = _normalize_case_id(event.get("case_id"))
    if not case_id:
        return None

    projected = {name: event[name] for name in _PUBLIC_EVENT_FIELDS if name in event}
    projected["case_id"] = case_id
    event_name = str(projected.get("event") or "").strip().lower()
    projected["event"] = event_name if _PUBLIC_CODE_RE.fullmatch(event_name) else "system.event_blocked"
    event_type = str(projected.get("type") or "").strip().lower()
    projected["type"] = event_type if event_type in _PUBLIC_EVENT_TYPES else "info"
    channel = str(projected.get("channel") or "").strip().lower()
    if channel:
        projected["channel"] = channel if channel in _PUBLIC_CHANNELS else "system"
    job_id = str(projected.get("job_id") or "").strip()
    if job_id:
        if _PUBLIC_TOKEN_RE.fullmatch(job_id):
            projected["job_id"] = job_id
        else:
            projected.pop("job_id", None)
    version = str(projected.get("version") or "").strip()
    if version:
        if _PUBLIC_TOKEN_RE.fullmatch(version):
            projected["version"] = version
        else:
            projected.pop("version", None)
    for number_field in ("sequence",):
        if number_field not in projected:
            continue
        normalized_number = _bounded_non_negative_int(
            projected[number_field],
            maximum=9_007_199_254_740_991,
        )
        if normalized_number is None:
            projected.pop(number_field, None)
        else:
            projected[number_field] = normalized_number
    timestamp = str(projected.get("timestamp") or "").strip()
    if timestamp:
        if _PUBLIC_TIMESTAMP_RE.fullmatch(timestamp):
            projected["timestamp"] = timestamp
        else:
            projected.pop("timestamp", None)
    if projected["type"] == "error" or _contains_raw_diagnostic(event):
        projected["event"] = "system.diagnostic_blocked"
        projected["type"] = "error"
        projected["payload"] = {**_CLOSED_ERROR_PAYLOAD, "details": {}}
    else:
        projected["payload"] = _project_public_payload(event.get("payload"))
    return projected


@dataclass
class _WebSocketClientState:
    websocket: WebSocket
    case_filter: str
    tracked_session_ids: set[str] = field(default_factory=set)


class WebSocketManager:
    """Manage case-bound ws connections and publish closed public events."""

    def __init__(self) -> None:
        self._clients: Dict[int, _WebSocketClientState] = {}
        self._lock = asyncio.Lock()
        self._loop: Optional[asyncio.AbstractEventLoop] = None
        self._logger = logging.getLogger("analytix.ws")

    def bind_loop(self, loop: asyncio.AbstractEventLoop) -> None:
        self._loop = loop

    async def connect(self, websocket: WebSocket, case_id: Optional[str]) -> None:
        normalized_case_id = _normalize_case_id(case_id)
        if not normalized_case_id:
            raise ValueError("case_id_required")
        async with self._lock:
            self._clients[id(websocket)] = _WebSocketClientState(
                websocket=websocket,
                case_filter=normalized_case_id,
            )

    async def disconnect(self, websocket: WebSocket) -> None:
        async with self._lock:
            self._clients.pop(id(websocket), None)

    async def track_session(self, websocket: WebSocket, session_id: str) -> None:
        normalized_session_id = str(session_id or "").strip()
        if not normalized_session_id:
            return
        async with self._lock:
            state = self._clients.get(id(websocket))
            if state is None:
                return
            state.tracked_session_ids.add(normalized_session_id)

    @staticmethod
    def _matches(
        client_state: _WebSocketClientState,
        *,
        event_case_id: Optional[str],
        event_session_id: Optional[str],
    ) -> bool:
        if client_state.case_filter != _normalize_case_id(event_case_id):
            return False
        if not client_state.tracked_session_ids:
            return True
        normalized_session_id = str(event_session_id or "").strip()
        if not normalized_session_id:
            return False
        return normalized_session_id in client_state.tracked_session_ids

    async def publish(self, event: dict) -> None:
        projected_event = _project_public_event(event)
        if projected_event is None:
            return
        async with self._lock:
            clients = list(self._clients.values())

        stale: list[WebSocket] = []
        event_case_id = projected_event.get("case_id")
        payload = projected_event.get("payload")
        event_session_id = None
        if isinstance(payload, dict):
            event_session_id = payload.get("session_id")
        if not event_session_id:
            event_session_id = projected_event.get("job_id")

        for client_state in clients:
            if not self._matches(
                client_state,
                event_case_id=event_case_id,
                event_session_id=event_session_id,
            ):
                continue
            try:
                await client_state.websocket.send_json(projected_event)
            except Exception:
                stale.append(client_state.websocket)

        if stale:
            async with self._lock:
                for websocket in stale:
                    self._clients.pop(id(websocket), None)

    def publish_threadsafe(self, event: dict) -> None:
        loop = self._loop
        if loop is None or loop.is_closed():
            return

        try:
            future = asyncio.run_coroutine_threadsafe(self.publish(event), loop)
            future.add_done_callback(self._log_publish_error)
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="websocket",
                code="publish_failed",
            )

    def _log_publish_error(self, future: "asyncio.Future[None]") -> None:
        try:
            _ = future.result()
        except Exception:
            log_closed_diagnostic(
                self._logger,
                logging.ERROR,
                topic="websocket",
                code="publish_failed",
            )
