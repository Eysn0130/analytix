from __future__ import annotations

import asyncio
import json

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.api.v1 import analysis as analysis_api
from app.api.v1.analysis import (
    list_feature_mart_scopes,
    materialize_feature_mart,
    refresh_case_analysis,
)
from app.domain.case_service import CaseNotFoundError
from app.schemas.analysis import (
    AnalysisFeatureMartMaterializeReq,
    AnalysisMaintenancePublicDTO,
    AnalysisRefreshReq,
    AnalysisScopeCachePublicDTO,
)
from app.tasks.models import TaskType
from app.tasks.service import TaskService


_CASE_ID = "case-maintenance-boundary"
_FULL_ACCOUNT = "6222020000000000"


class _CaseService:
    def get_case(self, case_id: str) -> dict:
        assert case_id == _CASE_ID
        return {"case_id": case_id, "is_deleted": False}


class _AnalysisService:
    def __init__(
        self,
        *,
        job_id: str = "",
        reported_case_id: str = _CASE_ID,
        reported_status: str = "queued",
    ) -> None:
        self.scope_reads = 0
        self.job_id = job_id
        self.reported_case_id = reported_case_id
        self.reported_status = reported_status

    def refresh_case_analysis(self, case_id: str, **_kwargs) -> dict:
        assert case_id == _CASE_ID
        if _kwargs.get("mode") == "async":
            return {
                "case_id": self.reported_case_id,
                "status": self.reported_status,
                "job_id": self.job_id,
                "payload": {"account_no": _FULL_ACCOUNT},
            }
        return {
            "case_id": case_id,
            "status": "succeeded",
            "refreshed": ["transactions"],
            "stats": {"txn_count": 0, "account_no": _FULL_ACCOUNT},
        }

    def materialize_feature_mart(self, case_id: str, **_kwargs) -> dict:
        assert case_id == _CASE_ID
        return {
            "case_id": case_id,
            "materialized_scope_count": 1,
            "materialized_accounts": [{"account_key": _FULL_ACCOUNT, "txn_count": 0}],
            "query_ids": ["local-query-is-not-evidence"],
        }

    def list_scope_cache_entries(self, *_args, **_kwargs) -> list[dict]:
        self.scope_reads += 1
        raise AssertionError("scope cache facts were read without Go host authority")


def _response_data(response) -> dict:
    return json.loads(bytes(response.body))["data"]


def test_sync_refresh_strips_hostile_case_facts() -> None:
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(),
            task_service=TaskService(),
        )
    )
    data = _response_data(response)

    assert data == {
        "contract": "AnalysisMaintenancePublicBoundaryV1",
        "case_id": _CASE_ID,
        "operation": "analysis_refresh",
        "operation_status": "blocked",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "job_id": "",
    }
    assert _FULL_ACCOUNT not in json.dumps(data)


def test_async_refresh_exposes_only_an_opaque_queued_job() -> None:
    task_service = TaskService()
    task = task_service.create_task(TaskType.ANALYSIS_REFRESH, case_id=_CASE_ID)
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="async"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(
                job_id=task.task_id,
                reported_case_id="case-hostile-service-report",
                reported_status="succeeded",
            ),
            task_service=task_service,
        )
    )

    assert _response_data(response) == {
        "contract": "AnalysisMaintenancePublicBoundaryV1",
        "case_id": _CASE_ID,
        "operation": "analysis_refresh",
        "operation_status": "queued",
        "semantic_status": "blocked",
        "blocker": "host_evidence_receipt_required",
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "job_id": task.task_id,
    }


@pytest.mark.parametrize("candidate", ("job-refresh-a", "00000000-0000-0000-0000-000000000000", _FULL_ACCOUNT))
def test_async_refresh_rejects_untrusted_or_unknown_job_ids(candidate: str) -> None:
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="async"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(job_id=candidate),
            task_service=TaskService(),
        )
    )

    data = _response_data(response)
    assert data["operation_status"] == "blocked"
    assert data["job_id"] == ""
    assert candidate not in json.dumps(data)


@pytest.mark.parametrize(
    ("task_case_id", "task_type"),
    (
        ("case-other-maintenance", TaskType.ANALYSIS_REFRESH),
        (_CASE_ID, TaskType.IMPORT),
    ),
)
def test_async_refresh_requires_same_case_analysis_refresh_registry_task(
    task_case_id: str,
    task_type: TaskType,
) -> None:
    task_service = TaskService()
    task = task_service.create_task(task_type, case_id=task_case_id)
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="async"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(job_id=task.task_id),
            task_service=task_service,
        )
    )

    data = _response_data(response)
    assert data["operation_status"] == "blocked"
    assert data["job_id"] == ""


def test_async_refresh_canonicalizes_registered_task_id_before_publication() -> None:
    task_service = TaskService()
    task = task_service.create_task(TaskType.ANALYSIS_REFRESH, case_id=_CASE_ID)
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="async"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(job_id=f" {task.task_id}"),
            task_service=task_service,
        )
    )

    data = _response_data(response)
    assert data["operation_status"] == "queued"
    assert data["job_id"] == task.task_id


def test_refresh_registry_failure_is_a_nonleaking_blocked_boundary() -> None:
    task_id = "00000000-0000-0000-0000-000000000000"

    class _FailingTaskRegistry(TaskService):
        def get_public_task(self, *_args, **_kwargs):
            raise RuntimeError(f"registry secret {_FULL_ACCOUNT}")

    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="async"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(job_id=task_id),
            task_service=_FailingTaskRegistry(),
        )
    )

    data = _response_data(response)
    assert data["operation_status"] == "blocked"
    assert data["job_id"] == ""
    assert _FULL_ACCOUNT not in json.dumps(data)


def test_sync_refresh_cannot_publish_a_valid_registry_job() -> None:
    task_service = TaskService()
    task = task_service.create_task(TaskType.ANALYSIS_REFRESH, case_id=_CASE_ID)
    response = asyncio.run(
        refresh_case_analysis(
            AnalysisRefreshReq(case_id=_CASE_ID, mode="sync"),
            case_service=_CaseService(),
            analysis_service=_AnalysisService(job_id=task.task_id),
            task_service=task_service,
        )
    )

    data = _response_data(response)
    assert data["operation_status"] == "blocked"
    assert data["job_id"] == ""


def test_sync_materialize_never_returns_accounts_counts_or_query_ids() -> None:
    response = asyncio.run(materialize_feature_mart())
    data = _response_data(response)

    assert data["case_id"] == ""
    assert data["operation"] == "feature_mart_materialize"
    assert data["operation_status"] == "blocked"
    assert data["fact_answer_allowed"] is False
    assert not ({"materialized_accounts", "materialized_scope_count", "query_ids", "source_revision"} & data.keys())
    assert _FULL_ACCOUNT not in json.dumps(data)


@pytest.mark.parametrize("mode", ("async", "queue"))
def test_non_sync_materialize_never_creates_a_job(mode: str) -> None:
    del mode
    response = asyncio.run(materialize_feature_mart())
    data = _response_data(response)

    assert response.status_code == 200
    assert data["operation_status"] == "blocked"
    assert data["job_id"] == ""
    assert data["fact_answer_allowed"] is False


def test_materialize_is_body_opaque_and_never_echoes_or_parses_hostile_input() -> None:
    hostile = "6222020202020202020 /private/case.csv sk-materialize-secret"
    app = FastAPI()
    app.include_router(analysis_api.router, prefix="/api/v1")
    client = TestClient(app)

    for body in (
        f'{{"case_id":"{hostile}"',
        json.dumps({"case_id": _CASE_ID, "unexpected": hostile}),
    ):
        response = client.post(
            "/api/v1/analysis/feature-mart/materialize",
            content=body,
            headers={"content-type": "application/json"},
        )
        assert response.status_code == 200
        assert response.json()["data"]["operation_status"] == "blocked"
        assert hostile not in response.text


def test_materialize_route_has_no_body_or_service_dependency() -> None:
    route = next(
        item
        for item in analysis_api.router.routes
        if getattr(item, "path", "") == "/analysis/feature-mart/materialize"
    )

    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []


def test_scope_cache_route_never_queries_without_host_authority() -> None:
    service = _AnalysisService()
    response = asyncio.run(
        list_feature_mart_scopes(
            case_id=_CASE_ID,
            scope_kind="account",
            limit=50,
            case_service=_CaseService(),
            analysis_service=service,
        )
    )
    data = _response_data(response)

    assert service.scope_reads == 0
    assert data["items"] == []
    assert data["fact_answer_allowed"] is False


def test_boundary_dtos_reject_fact_fields_and_scope_items() -> None:
    with pytest.raises(ValidationError):
        AnalysisMaintenancePublicDTO(
            case_id=_CASE_ID,
            operation="analysis_refresh",
            operation_status="completed",
            stats={"txn_count": 0},
        )
    with pytest.raises(ValidationError):
        AnalysisScopeCachePublicDTO(case_id=_CASE_ID, items=[{"account_no": _FULL_ACCOUNT}])


@pytest.mark.parametrize(
    ("operation_status", "job_id"),
    (
        ("completed", ""),
        ("queued", "job-refresh-a"),
        ("queued", " 00000000-0000-0000-0000-000000000000"),
        ("blocked", "00000000-0000-0000-0000-000000000000"),
    ),
)
def test_maintenance_dto_rejects_noncanonical_or_inconsistent_job_state(
    operation_status: str,
    job_id: str,
) -> None:
    with pytest.raises(ValidationError):
        AnalysisMaintenancePublicDTO(
            case_id=_CASE_ID,
            operation="analysis_refresh",
            operation_status=operation_status,
            job_id=job_id,
        )


def test_materialize_request_rejects_unknown_fields() -> None:
    with pytest.raises(ValidationError):
        AnalysisFeatureMartMaterializeReq(case_id=_CASE_ID, context_epoch=7)
