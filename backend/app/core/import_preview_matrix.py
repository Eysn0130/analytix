from __future__ import annotations

from collections import Counter
from typing import Any, Mapping, Sequence


def preview_mapping_records(items: Sequence[Mapping[str, Any]]) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for item in items:
        children = item.get("archive_children") or []
        iterable = children if children else [item]
        for child in iterable:
            if not isinstance(child, Mapping):
                continue
            if not str(child.get("suggested_kind") or "").startswith("fc_"):
                continue
            records.append(
                {
                    "file_name": str(child.get("file_name") or ""),
                    "archive_path": str(child.get("archive_path") or ""),
                    "suggested_kind": str(child.get("suggested_kind") or ""),
                    "status": str(child.get("status") or ""),
                    "mapping_status": str(child.get("mapping_status") or ""),
                    "mapping_method": str(child.get("mapping_method") or ""),
                    "mapping_required_missing": list(child.get("mapping_required_missing") or []),
                    "mapping_message": str(child.get("mapping_message") or ""),
                    "issue": str(child.get("issue") or ""),
                }
            )
    return records


def preview_mapping_matrix(records: Sequence[Mapping[str, Any]]) -> dict[str, Any]:
    categories: Counter[str] = Counter()
    status_counts: Counter[str] = Counter()
    method_counts: Counter[str] = Counter()
    manual_items: list[dict[str, Any]] = []
    missing_items: list[dict[str, Any]] = []
    blocked_items: list[dict[str, Any]] = []

    for record in records:
        normalized = dict(record)
        status = str(normalized.get("mapping_status") or "")
        method = str(normalized.get("mapping_method") or "")
        missing = list(normalized.get("mapping_required_missing") or [])
        status_counts[status or "none"] += 1
        method_counts[method or "none"] += 1
        if status == "ready" and method == "exact_header":
            categories["rule_exact"] += 1
        elif status == "ready" and method == "rule":
            categories["rule_confirmed"] += 1
        elif status == "ready" and method == "ai_validated":
            categories["ai_confirmed"] += 1
        elif status == "review":
            categories["manual_review"] += 1
            manual_items.append(normalized)
        else:
            categories["blocked"] += 1
            blocked_items.append(normalized)
        if missing:
            missing_items.append(normalized)

    return {
        "total_structured": len(records),
        "categories": dict(categories),
        "status_counts": dict(status_counts),
        "method_counts": dict(method_counts),
        "manual_items": manual_items,
        "missing_items": missing_items,
        "blocked_items": blocked_items,
    }
