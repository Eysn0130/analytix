from __future__ import annotations

from typing import Any


def build_stats_rows_direct_payload(data: dict[str, Any]) -> dict[str, Any]:
    _ = data
    return {
        "contract": "StatsRowsPublicBoundaryV1",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "rows": [],
        "row_fields": [],
        "status": "controlled_projection_required",
        "total": None,
        "row_summary": {},
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
    }


def build_stats_txn_rows_direct_payload(data: dict[str, Any]) -> dict[str, Any]:
    _ = data
    return {
        "contract": "StatsTxnRowsPublicBoundaryV1",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "rows": [],
        "row_fields": [],
        "done": False,
        "next_cursor": None,
        "fact_answer_allowed": False,
        "raw_details_exposed": False,
    }
