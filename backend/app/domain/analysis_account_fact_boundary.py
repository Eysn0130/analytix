from __future__ import annotations

from typing import Literal


AccountFactOperation = Literal[
    "account_counterparty_rankings",
    "account_behavior_profile",
    "scope_account_snapshot",
]

_ACCOUNT_FACT_OPERATIONS = frozenset(
    {
        "account_counterparty_rankings",
        "account_behavior_profile",
        "scope_account_snapshot",
    }
)

_ACCOUNT_FACT_SKILLS = frozenset(
    {
        "get_account_behavior_profile",
        "get_account_counterparty_rankings",
        "account_behavior_profile",
        "account_counterparty_rankings",
        "analyze_account_full",
    }
)


def account_fact_operation_requires_host_authority(value: object) -> bool:
    return str(value or "").strip() in _ACCOUNT_FACT_OPERATIONS


def account_fact_skill_requires_host_authority(value: object) -> bool:
    return str(value or "").strip() in _ACCOUNT_FACT_SKILLS


def project_account_fact_boundary(case_id: str, operation: AccountFactOperation) -> dict:
    if not account_fact_operation_requires_host_authority(operation):
        raise ValueError("account_fact_operation_invalid")
    return {
        "contract": "AnalysisAccountFactBoundaryV1",
        "case_id": str(case_id or "").strip(),
        "operation": operation,
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
        "coverage": {
            "contract": "AnalysisAccountScopeCoverageV1",
            "status": "unverified",
            "completeness": "unknown",
            "checked_scope": "none",
            "transaction_rows": None,
            "amount_present_rows": None,
            "amount_missing_rows": None,
            "amount_parse_failed_rows": None,
            "direction_covered_rows": None,
        },
        "data": {},
        "evidence_receipts": [],
        "claim_records": [],
        "summary_text": "",
    }


__all__ = [
    "account_fact_operation_requires_host_authority",
    "account_fact_skill_requires_host_authority",
    "project_account_fact_boundary",
]
