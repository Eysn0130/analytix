from __future__ import annotations

import inspect
import queue

import pytest
from pydantic import ValidationError

from app.domain.analysis_maintenance_public_projection import (
    evidence_pack_deferred_write_requires_host_authority,
)
from app.domain.analysis_service import AnalysisService
from app.repositories.analysis_repository import AnalysisRepository
from app.schemas.analysis import (
    AnalysisEvidencePackPublicDTO,
    AnalysisFeatureMartMaterializeReq,
    AnalysisMaintenancePublicDTO,
)


_CASE_ID = "case-evidence-pack-quarantine"
_SENTINEL = "6222020202020202020 /private/case.csv SELECT * FROM secret_accounts"


class _Poison:
    def __getattr__(self, _name: str):
        raise AssertionError("evidence-pack quarantine crossed a data or persistence boundary")


def _poison(*_args, **_kwargs):
    raise AssertionError("evidence-pack quarantine crossed a data or persistence boundary")


def _assert_evidence_pack_boundary(result: dict) -> None:
    validated = AnalysisEvidencePackPublicDTO(**result).model_dump()
    assert validated["case_id"] == _CASE_ID
    assert validated["semantic_status"] == "blocked"
    assert validated["blocker"] == "host_evidence_receipt_required"
    assert validated["fact_answer_allowed"] is False
    assert validated["coverage_status"] == "unverified"
    assert validated["coverage_completeness"] == "unknown"
    assert validated["checked_scope"] == "none"
    assert validated["data"] == {}
    assert validated["evidence_receipts"] == []
    assert validated["claim_records"] == []


def test_repository_materialize_and_evidence_pack_block_before_case_or_database_access(monkeypatch) -> None:
    repository = object.__new__(AnalysisRepository)
    for name in (
        "case_exists",
        "sync_case_baseline",
        "open_case_engine",
        "ensure_analysis_schema",
        "_query_dicts",
        "_append_query_log",
    ):
        monkeypatch.setattr(repository, name, _poison)

    maintenance = repository.materialize_feature_mart(
        _CASE_ID,
        account_keys=[_SENTINEL],
        temp_scope_id=_SENTINEL,
        source_file_ids=[_SENTINEL],
        force_refresh=True,
    )
    validated_maintenance = AnalysisMaintenancePublicDTO(**maintenance).model_dump()
    assert validated_maintenance["operation"] == "feature_mart_materialize"
    assert validated_maintenance["blocker"] == "host_evidence_receipt_required"
    assert validated_maintenance["job_id"] == ""

    _assert_evidence_pack_boundary(
        repository.build_evidence_pack(
            _CASE_ID,
            account_keys=[_SENTINEL],
            txn_ids=[_SENTINEL],
            source_file_ids=[_SENTINEL],
            temp_scope_id=_SENTINEL,
        )
    )


def test_service_materialize_and_deferred_evidence_pack_never_reach_repository_or_queue() -> None:
    service = object.__new__(AnalysisService)
    service._repository = _Poison()
    service._async_write_queue = _Poison()

    maintenance = service.materialize_feature_mart(_CASE_ID, account_keys=[_SENTINEL])
    assert AnalysisMaintenancePublicDTO(**maintenance).blocker == "host_evidence_receipt_required"
    _assert_evidence_pack_boundary(
        service.build_evidence_pack(_CASE_ID)
    )
    for name in (
        "get_skill_cache_entry",
        "get_skill_cache_entries",
        "set_skill_cache_entry",
        "set_skill_cache_entry_async",
    ):
        assert not hasattr(AnalysisService, name), name
    assert service.append_query_log_entry(
        _CASE_ID,
        query_id=_SENTINEL,
        tool_name="get_evidence_pack",
        params={"account": _SENTINEL},
        summary={"amount": 0},
        row_count=1,
        duration_ms=1,
    ) == ""
    assert service.append_query_log_entry_async(
        _CASE_ID,
        query_id=_SENTINEL,
        tool_name="get_evidence_pack",
        params={"account": _SENTINEL},
        summary={"amount": 0},
        row_count=1,
        duration_ms=1,
    ) == ""


def test_repository_evidence_pack_cache_and_query_writes_block_before_case_lookup(monkeypatch) -> None:
    repository = object.__new__(AnalysisRepository)
    monkeypatch.setattr(repository, "case_exists", _poison)

    for name in (
        "get_skill_cache_entry",
        "get_skill_cache_entries",
        "set_skill_cache_entry",
        "list_scope_cache_entries",
        "_get_scope_cache_payload",
        "_set_scope_cache_payload",
    ):
        assert not hasattr(AnalysisRepository, name), name
    assert repository.append_query_log_entry(
        _CASE_ID,
        query_id=_SENTINEL,
        tool_name="get_evidence_pack",
        params={"account": _SENTINEL},
        summary={"amount": 0},
        row_count=1,
        duration_ms=1,
    ) == ""
    assert repository._append_query_log(
        _Poison(),
        case_id=_CASE_ID,
        query_id=_SENTINEL,
        tool_name="get_evidence_pack",
        params={"account": _SENTINEL},
        summary={"amount": 0},
        row_count=1,
        duration_ms=1,
    ) == ""


def test_evidence_pack_is_not_batch_eligible_or_pre_warmed() -> None:
    signature = inspect.signature(AnalysisRepository.build_evidence_pack)
    assert "cache_key" not in signature.parameters
    assert "defer_persistence" not in signature.parameters
    assert "scope_fact_pack" not in signature.parameters
    assert "build_evidence_pack(" not in inspect.getsource(AnalysisRepository.materialize_feature_mart)


def test_deferred_write_filter_rejects_both_evidence_pack_write_kinds() -> None:
    assert evidence_pack_deferred_write_requires_host_authority(
        kind="skill_cache",
        payload={"skill_id": "get_evidence_pack"},
    )
    assert evidence_pack_deferred_write_requires_host_authority(
        kind="query_log",
        payload={"tool_name": "get_evidence_pack"},
    )
    assert not evidence_pack_deferred_write_requires_host_authority(
        kind="query_log",
        payload={"tool_name": "safe_operational_diagnostic"},
    )


@pytest.mark.parametrize(
    ("kind", "payload"),
    (
        (
            "skill_cache",
            {
                "skill_id": "get_evidence_pack",
                "cache_key": _SENTINEL,
                "payload": {"amount": 0},
            },
        ),
        (
            "query_log",
            {
                "tool_name": "get_evidence_pack",
                "query_id": _SENTINEL,
                "params": {"account": _SENTINEL},
            },
        ),
    ),
)
def test_already_queued_evidence_pack_write_is_dropped_before_repository_or_event_access(
    kind: str,
    payload: dict,
) -> None:
    class _StopWorker(BaseException):
        pass

    class _OneItemQueue:
        def __init__(self) -> None:
            self._served = False
            self.task_done_count = 0

        def get(self):
            if self._served:
                raise _StopWorker()
            self._served = True
            return (
                kind,
                _CASE_ID,
                payload,
                0,
            )

        @staticmethod
        def get_nowait():
            raise queue.Empty

        def task_done(self) -> None:
            self.task_done_count += 1

    service = object.__new__(AnalysisService)
    service._async_write_queue = _OneItemQueue()
    service._repository = _Poison()

    with pytest.raises(_StopWorker):
        service._async_write_worker()
    assert service._async_write_queue.task_done_count == 1


def test_external_materialize_and_boundary_schemas_are_closed() -> None:
    with pytest.raises(ValidationError):
        AnalysisFeatureMartMaterializeReq(case_id=_CASE_ID, unknown_authority="forged")
    with pytest.raises(ValidationError):
        AnalysisEvidencePackPublicDTO(
            case_id=_CASE_ID,
            amount=0,
        )
    with pytest.raises(ValidationError):
        AnalysisEvidencePackPublicDTO(
            case_id=_CASE_ID,
            evidence_receipts=[{"receipt_id": "forged"}],
        )
