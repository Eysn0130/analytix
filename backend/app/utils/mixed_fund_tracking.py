from __future__ import annotations

from typing import Any, Mapping


def mixed_fund_scope_label(scope_source: str) -> str:
    normalized = str(scope_source or "").strip().lower()
    return {
        "db": "当前案件库可见后续流水口径",
        "temp_scope": "当前临时范围口径",
        "uploaded_file": "当前上传文件范围口径",
    }.get(normalized, "当前 scope 口径")


def mixed_fund_cutoff_scope_label(scope_source: str) -> str:
    normalized = str(scope_source or "").strip().lower()
    return {
        "db": "当前案件库可见后续流水",
        "temp_scope": "当前临时范围可见后续流水",
        "uploaded_file": "当前上传文件范围可见后续流水",
    }.get(normalized, "当前 scope 可见后续流水")


def build_mixed_fund_conclusion_text(mixed_fund_tracking: Mapping[str, Any] | None) -> str:
    # P0 quarantine: this helper has no access to the host Evidence Registry or
    # Final Evidence Gate. It must not turn internal calculations (or caller
    # supplied prose) into a publishable case conclusion.
    del mixed_fund_tracking
    return ""


def enrich_mixed_fund_tracking(
    mixed_fund_tracking: Mapping[str, Any] | None,
    *,
    scope_source: str = "db",
) -> dict[str, Any]:
    tracking = dict(mixed_fund_tracking or {})
    if not tracking:
        return {}
    cutoff_scope_label = mixed_fund_cutoff_scope_label(scope_source)
    enriched = {
        **tracking,
        "rule_basis": str(tracking.get("rule_basis") or "最低余额法/混同资金认定"),
        "scope_source": scope_source,
        "scope_label": mixed_fund_scope_label(scope_source),
        "cutoff_basis": str(tracking.get("cutoff_basis") or "scope_latest_txn"),
        "cutoff_label": str(tracking.get("cutoff_label") or f"截至{cutoff_scope_label}的最后一笔时间"),
        "cutoff_time": str(tracking.get("cutoff_time") or tracking.get("latest_balance_time") or ""),
    }
    enriched["conclusion_text"] = build_mixed_fund_conclusion_text(enriched)
    return enriched
