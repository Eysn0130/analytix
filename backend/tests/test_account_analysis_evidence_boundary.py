from __future__ import annotations

import json
from pathlib import Path

import pytest
from pydantic import ValidationError

from app.domain.analysis_service import AnalysisService
from app.repositories.analysis_repository import AnalysisRepository
from app.schemas.analysis import AnalysisAccountFactBoundaryDTO


@pytest.fixture
def repository(monkeypatch) -> AnalysisRepository:
    instance = object.__new__(AnalysisRepository)
    monkeypatch.setattr(instance, "case_exists", lambda case_id: case_id == "case-a")

    def poison(*_args, **_kwargs):
        raise AssertionError("account fact boundary performed a forbidden data or persistence operation")

    for name in (
        "sync_case_baseline",
        "open_case_engine",
        "ensure_analysis_schema",
        "_resolve_active_temp_scope_or_raise",
        "_resolve_scope_account_keys",
        "_query_dicts",
        "_append_query_log",
    ):
        monkeypatch.setattr(instance, name, poison)
    return instance


@pytest.mark.parametrize(
    ("method_name", "operation"),
    (
        ("build_account_counterparty_rankings", "account_counterparty_rankings"),
        ("build_account_behavior_profile", "account_behavior_profile"),
        ("build_scope_account_snapshot", "scope_account_snapshot"),
    ),
)
def test_account_analysis_requires_go_host_evidence_before_any_data_access(
    repository: AnalysisRepository,
    method_name: str,
    operation: str,
) -> None:
    result = getattr(repository, method_name)(
        "case-a",
        account_keys=[f"account-{index}" for index in range(65)],
        source_file_ids=["file-a"],
        temp_scope_id="temp-a",
    )
    validated = AnalysisAccountFactBoundaryDTO(**result).model_dump()

    assert validated["operation"] == operation
    assert validated["semantic_status"] == "blocked"
    assert validated["blocker"] == "host_evidence_receipt_required"
    assert validated["fact_answer_allowed"] is False
    assert validated["coverage"]["checked_scope"] == "none"
    assert validated["data"] == {}
    assert validated["evidence_receipts"] == []
    assert validated["claim_records"] == []
    assert validated["summary_text"] == ""
    serialized = json.dumps(validated, ensure_ascii=False, sort_keys=True)
    for forbidden in ("selected_accounts", "inflow_total", "outflow_total", "未见", "暂无", "cache_hit"):
        assert forbidden not in serialized


@pytest.mark.parametrize(
    "method_name",
    (
        "build_account_behavior_profile",
        "build_scope_account_snapshot",
    ),
)
def test_account_fact_boundary_does_not_hide_missing_case(
    repository: AnalysisRepository,
    method_name: str,
) -> None:
    with pytest.raises(KeyError, match="case-missing"):
        getattr(repository, method_name)("case-missing")


def test_account_fact_boundary_schema_rejects_facts_and_unknown_authority() -> None:
    baseline = {
        "case_id": "case-a",
        "operation": "account_behavior_profile",
    }
    for mutation in (
        {"data": {"inflow_total": 0}},
        {"evidence_receipts": [{"receipt_id": "forged"}]},
        {"claim_records": [{"claim": "forged"}]},
        {"summary_text": "未见异常"},
        {"fact_answer_allowed": True},
        {"unknown": "field"},
    ):
        with pytest.raises(ValidationError):
            AnalysisAccountFactBoundaryDTO(**{**baseline, **mutation})


def test_account_fact_cache_interfaces_are_never_read_written_or_queued() -> None:
    for owner, names in (
        (
            AnalysisRepository,
            ("get_skill_cache_entry", "get_skill_cache_entries", "set_skill_cache_entry"),
        ),
        (
            AnalysisService,
            (
                "get_skill_cache_entry",
                "get_skill_cache_entries",
                "set_skill_cache_entry",
                "set_skill_cache_entry_async",
            ),
        ),
    ):
        for name in names:
            assert not hasattr(owner, name), name


def test_account_scope_cache_api_is_physically_absent() -> None:
    assert not hasattr(AnalysisRepository, "_get_scope_cache_payload")
    assert not hasattr(AnalysisRepository, "_set_scope_cache_payload")


def test_production_api_router_keeps_dormant_skill_execution_unwired() -> None:
    router_source = (Path(__file__).parents[1] / "app" / "api" / "router.py").read_text(encoding="utf-8")

    assert "skills" not in router_source
    assert "account_behavior_profile" not in router_source
    assert "account_counterparty_rankings" not in router_source
