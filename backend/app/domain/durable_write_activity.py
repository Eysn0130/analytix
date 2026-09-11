from __future__ import annotations

from typing import Any, Sequence


def _text(value: Any) -> str:
    return str(value or "").strip()


def _unique_texts(values: Sequence[Any]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for value in values:
        normalized = _text(value)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        result.append(normalized)
    return result


def build_durable_write_result_ref(
    *,
    kind: str,
    payloads: Sequence[dict[str, Any]],
    status: str,
    error: str = "",
) -> dict[str, Any]:
    payload_list = [dict(payload or {}) for payload in list(payloads or [])]
    result_ref: dict[str, Any] = {
        "durable_write_kind": _text(kind),
        "durable_write_status": _text(status),
        "report_ids": [
            _text(payload.get("report_id"))
            for payload in payload_list
            if _text(payload.get("report_id"))
        ]
        + [
            _text(item)
            for payload in payload_list
            for item in list(payload.get("report_ids") or payload.get("source_report_ids") or [])
            if _text(item)
        ],
        "artifact_ids": _unique_texts(
            [
                candidate
                for payload in payload_list
                for candidate in (payload.get("artifact_id"), payload.get("note_id"), payload.get("revision_id"))
            ]
        ),
        "query_ids": [
            _text(item)
            for payload in payload_list
            for item in list(payload.get("query_ids") or payload.get("source_query_ids") or [])
            if _text(item)
        ],
        "evidence_ids": [
            _text(item)
            for payload in payload_list
            for item in list(payload.get("evidence_ids") or payload.get("source_evidence_ids") or [])
            if _text(item)
        ],
        "path_ids": [
            _text(item)
            for payload in payload_list
            for item in list(payload.get("path_ids") or payload.get("source_path_ids") or [])
            if _text(item)
        ],
        "workspace_files": [
            _text(file_name)
            for payload in payload_list
            for file_name in list(payload.get("files") or payload.get("workspace_files") or [])
            if _text(file_name)
        ],
        "projection_kinds": [
            _text(payload.get("projection_kind"))
            for payload in payload_list
            if _text(payload.get("projection_kind"))
        ],
        "render_modes": [
            _text(payload.get("render_mode"))
            for payload in payload_list
            if _text(payload.get("render_mode"))
        ],
    }
    if error:
        result_ref["error"] = str(error)
    return {key: value for key, value in result_ref.items() if value not in ("", None, [], {})}


def build_durable_write_event(
    *,
    kind: str,
    payloads: Sequence[dict[str, Any]],
    status: str,
    error: str = "",
) -> tuple[dict[str, Any], dict[str, Any]]:
    normalized_status = _text(status) or "completed"
    result_ref = build_durable_write_result_ref(
        kind=kind,
        payloads=payloads,
        status=normalized_status,
        error=error,
    )
    return result_ref, {
        "event_type": f"analysis.durable_write.{normalized_status}",
        "actor_id": "system",
        "actor_role": "system",
        "payload": {
            "kind": _text(kind),
            "status": normalized_status,
            "result_ref": result_ref,
            **({"error": str(error)} if error else {}),
        },
    }
