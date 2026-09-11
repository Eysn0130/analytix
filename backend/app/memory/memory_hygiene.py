from __future__ import annotations

import hashlib
from datetime import datetime
from typing import Any, Sequence


_REVALIDATION_THRESHOLD_DAYS = {
    "stable": 30,
    "time_sensitive": 7,
    "volatile": 2,
}

_DERIVED_ARCHIVE_THRESHOLD_DAYS = {
    "stable": 90,
    "time_sensitive": 14,
    "volatile": 5,
}


def _trim(value: Any) -> str:
    return str(value or "").strip()


def _coerce_datetime(value: Any) -> datetime | None:
    text = _trim(value)
    if not text:
        return None
    normalized = text.replace("Z", "+00:00")
    for candidate in (normalized, normalized.replace("T", " ")):
        try:
            parsed = datetime.fromisoformat(candidate)
            return parsed.replace(tzinfo=None) if parsed.tzinfo is not None else parsed
        except Exception:
            continue
    for pattern in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%d %H:%M", "%Y-%m-%d"):
        try:
            return datetime.strptime(text, pattern)
        except Exception:
            continue
    return None


def _age_days(value: Any, *, now: datetime) -> int:
    parsed = _coerce_datetime(value)
    if parsed is None:
        return 0
    return max(0, (now - parsed).days)


def _proposal_id(*parts: str) -> str:
    digest = hashlib.sha1("|".join(parts).encode("utf-8")).hexdigest()[:12]
    return f"memhyg_{digest}"


def _note_signature(note: dict[str, Any]) -> tuple[str, str, str, tuple[str, ...]]:
    summary_anchor = _trim(note.get("summary")).lower() or _trim(note.get("detail"))[:120].lower()
    return (
        _trim(note.get("scope")).lower(),
        _trim(note.get("memory_type")).lower(),
        summary_anchor,
        tuple(sorted({_trim(item).lower() for item in list(note.get("tags") or []) if _trim(item)})),
    )


def _build_note_cleanup_proposal(
    *,
    action: str,
    note: dict[str, Any],
    rationale: str,
    severity: str,
    evidence: dict[str, Any] | None = None,
    suggested_inputs: dict[str, Any] | None = None,
) -> dict[str, Any]:
    note_id = _trim(note.get("note_id"))
    title = _trim(note.get("title")) or "未命名记忆"
    return {
        "proposal_id": _proposal_id(action, note_id, title),
        "group": "cleanup",
        "action": action,
        "target_kind": "memory_note",
        "target_id": note_id,
        "title": title,
        "scope": _trim(note.get("scope")),
        "severity": severity,
        "rationale": rationale,
        "suggested_inputs": dict(suggested_inputs or {}),
        "evidence": dict(evidence or {}),
    }


def review_memory_hygiene(
    *,
    case_id: str,
    actor_id: str,
    notes: Sequence[dict[str, Any]],
    now: datetime | None = None,
) -> dict[str, Any]:
    current_time = now or datetime.utcnow()
    normalized_notes = [dict(item or {}) for item in notes if isinstance(item, dict)]
    active_notes = [
        note
        for note in normalized_notes
        if _trim(note.get("status")).lower() in {"", "active"}
    ]
    promotions: list[dict[str, Any]] = []
    cleanup: list[dict[str, Any]] = []
    ambiguous: list[dict[str, Any]] = []
    no_action_needed: list[dict[str, Any]] = []
    proposed_note_ids: set[str] = set()

    signature_map: dict[tuple[str, str, str, tuple[str, ...]], list[dict[str, Any]]] = {}
    for note in active_notes:
        signature = _note_signature(note)
        if not any(signature):
            continue
        signature_map.setdefault(signature, []).append(note)

    for related_notes in signature_map.values():
        if len(related_notes) < 2:
            continue
        ordered = sorted(
            related_notes,
            key=lambda item: (
                _coerce_datetime(item.get("updated_at")) or datetime.min,
                _coerce_datetime(item.get("created_at")) or datetime.min,
            ),
            reverse=True,
        )
        keeper = ordered[0]
        for duplicate in ordered[1:]:
            note_id = _trim(duplicate.get("note_id"))
            if note_id:
                proposed_note_ids.add(note_id)
            cleanup.append(
                _build_note_cleanup_proposal(
                    action="archive_note",
                    note=duplicate,
                    rationale=f"存在较新的相同记忆“{_trim(keeper.get('title')) or '未命名记忆'}”，建议归档旧版本，避免 recall 重复命中。",
                    severity="warning",
                    evidence={
                        "duplicate_of_note_id": _trim(keeper.get("note_id")),
                        "duplicate_signature": list(_note_signature(duplicate)),
                    },
                    suggested_inputs={
                        "case_id": case_id,
                        "note_id": note_id,
                        "status": "archived",
                    },
                )
            )

    for note in active_notes:
        note_id = _trim(note.get("note_id"))
        age_days = _age_days(note.get("updated_at"), now=current_time)
        freshness = _trim(note.get("freshness")).lower() or "time_sensitive"
        trust_level = _trim(note.get("trust_level")).lower() or "working"
        required_revalidation = bool(note.get("required_revalidation"))
        source = _trim(note.get("source")).lower() or "manual"
        scope = _trim(note.get("scope")).lower() or "workspace"

        if note_id in proposed_note_ids:
            continue

        if scope == "workspace" and trust_level == "preference":
            ambiguous.append(
                {
                    "proposal_id": _proposal_id("review_scope", note_id),
                    "group": "ambiguous",
                    "action": "review_scope",
                    "target_kind": "memory_note",
                    "target_id": note_id,
                    "title": _trim(note.get("title")) or "未命名记忆",
                    "scope": scope,
                    "severity": "warning",
                    "rationale": "共享 workspace memory 中出现了偏好型内容，更像个人工作习惯，建议确认是否应迁回 operator memory。",
                    "suggested_inputs": {
                        "case_id": case_id,
                        "note_id": note_id,
                    },
                    "evidence": {
                        "trust_level": trust_level,
                        "memory_type": _trim(note.get("memory_type")),
                    },
                }
            )
            proposed_note_ids.add(note_id)
            continue

        revalidation_threshold = int(_REVALIDATION_THRESHOLD_DAYS.get(freshness, 7))
        if required_revalidation and age_days >= revalidation_threshold:
            cleanup.append(
                _build_note_cleanup_proposal(
                    action="revalidate_note",
                    note=note,
                    rationale=f"该记忆被标记为需复核，且已 {age_days} 天未更新，超过 {revalidation_threshold} 天阈值，建议重新验证后再继续参与 recall。",
                    severity="warning",
                    evidence={
                        "age_days": age_days,
                        "freshness": freshness,
                        "required_revalidation": True,
                    },
                    suggested_inputs={
                        "case_id": case_id,
                        "note_id": note_id,
                        "required_revalidation": True,
                    },
                )
            )
            proposed_note_ids.add(note_id)
            continue

        derived_archive_threshold = int(_DERIVED_ARCHIVE_THRESHOLD_DAYS.get(freshness, 14))
        if source == "derived" and age_days >= derived_archive_threshold and freshness in {"time_sensitive", "volatile"}:
            cleanup.append(
                _build_note_cleanup_proposal(
                    action="archive_note",
                    note=note,
                    rationale=f"该派生记忆已 {age_days} 天未更新，且属于 {freshness} 信息，建议归档，避免旧推断长期参与 recall。",
                    severity="info",
                    evidence={
                        "age_days": age_days,
                        "freshness": freshness,
                        "source": source,
                    },
                    suggested_inputs={
                        "case_id": case_id,
                        "note_id": note_id,
                        "status": "archived",
                    },
                )
            )
            proposed_note_ids.add(note_id)
            continue

        no_action_needed.append(
            {
                "proposal_id": _proposal_id("keep", note_id),
                "group": "no_action_needed",
                "action": "keep",
                "target_kind": "memory_note",
                "target_id": note_id,
                "title": _trim(note.get("title")) or "未命名记忆",
                "scope": scope,
                "severity": "info",
                "rationale": "当前记忆未发现重复、过期或 scope 风险，可继续保留。",
                "suggested_inputs": {},
                "evidence": {
                    "age_days": age_days,
                    "freshness": freshness,
                    "trust_level": trust_level,
                },
            }
        )

    return {
        "case_id": case_id,
        "actor_id": actor_id,
        "summary": {
            "generated_at": current_time.strftime("%Y-%m-%d %H:%M:%S"),
            "active_note_count": len(active_notes),
            "promotion_count": len(promotions),
            "cleanup_count": len(cleanup),
            "ambiguous_count": len(ambiguous),
            "no_action_count": len(no_action_needed),
        },
        "promotions": promotions,
        "cleanup": cleanup,
        "ambiguous": ambiguous,
        "no_action_needed": no_action_needed,
    }
