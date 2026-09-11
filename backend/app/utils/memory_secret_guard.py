from __future__ import annotations

import re
from typing import Any, Mapping


def _trim(value: Any) -> str:
    return str(value or "").strip()


def _mask(value: str) -> str:
    normalized = _trim(value)
    if not normalized:
        return ""
    if len(normalized) <= 6:
        return "····"
    return f"{normalized[:3]}····{normalized[-2:]}"


_SECRET_PATTERNS: tuple[tuple[str, re.Pattern[str]], ...] = (
    ("openai_api_key", re.compile(r"\bsk-[A-Za-z0-9]{16,}\b")),
    ("aws_access_key", re.compile(r"\bAKIA[0-9A-Z]{16}\b")),
    ("bearer_token", re.compile(r"\bBearer\s+[A-Za-z0-9\-._~+/]+=*\b", re.IGNORECASE)),
    (
        "credential_assignment",
        re.compile(
            r"(?i)(?:\b(?:api[_ -]?key|access[_ -]?token|refresh[_ -]?token|secret|password|passwd|pwd)\b|密钥|口令|密码|令牌)"
            r"\s*[:=：]\s*['\"]?[A-Za-z0-9_\-./+=]{8,}"
        ),
    ),
)


def _scan_secret_fields(fields: Mapping[str, Any] | None) -> dict[str, Any]:
    normalized_fields = {
        str(field_name or "").strip(): _trim(field_value)
        for field_name, field_value in dict(fields or {}).items()
        if str(field_name or "").strip()
    }
    findings: list[dict[str, str]] = []
    for field_name, content in normalized_fields.items():
        if not content:
            continue
        for secret_type, pattern in _SECRET_PATTERNS:
            match = pattern.search(content)
            if not match:
                continue
            findings.append(
                {
                    "field": field_name,
                    "secret_type": secret_type,
                    "masked_preview": _mask(match.group(0)),
                }
            )
    unique_types = []
    seen_types: set[str] = set()
    for finding in findings:
        secret_type = finding["secret_type"]
        if secret_type in seen_types:
            continue
        seen_types.add(secret_type)
        unique_types.append(secret_type)
    return {
        "blocked": bool(findings),
        "finding_count": len(findings),
        "secret_types": unique_types,
        "findings": findings,
    }


def scan_memory_note_for_secrets(note: dict[str, Any] | None) -> dict[str, Any]:
    payload = dict(note or {})
    return _scan_secret_fields(
        {
            "title": payload.get("title"),
            "summary": payload.get("summary"),
            "detail": payload.get("detail"),
            "tags": " ".join(_trim(tag) for tag in list(payload.get("tags") or []) if _trim(tag)),
        }
    )

def scan_session_memory_for_secrets(
    memory: dict[str, Any] | None,
    *,
    title: str = "",
    last_prompt: str = "",
) -> dict[str, Any]:
    payload = dict(memory or {})
    return _scan_secret_fields(
        {
            "title": title,
            "last_prompt": last_prompt,
            "focus_summary": payload.get("focus_summary"),
            "conclusion_summary": payload.get("conclusion_summary"),
            "next_steps_summary": payload.get("next_steps_summary"),
            "tags": " ".join(_trim(tag) for tag in list(payload.get("tags") or []) if _trim(tag)),
        }
    )
