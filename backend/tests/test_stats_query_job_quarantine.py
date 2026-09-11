from __future__ import annotations

import asyncio
import json
from types import SimpleNamespace

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.v1 import stats as stats_api
from app.api.v1.stats import (
    cancel_stats_v2_query_job,
    create_stats_v2_query_job,
    get_stats_v2_query_job,
    get_stats_v2_query_job_result,
    query_stats_v2_rows_direct,
    query_stats_v2_txn_rows_direct,
)
from app.domain.stats_export_source import StatsExportSourceResolver
from app.domain.stats_job_service import StatsJobService, StatsQueryJobQuarantinedError
from app.schemas.stats import StatsV2RowsResultPayload


_CASE_ID = "case-stats-query-quarantine"
_HOSTILE = "6222020202020202020 /private/case.csv sk-query-secret-123456"


class _Poison:
    def __getattr__(self, _name: str):
        raise AssertionError("stats query quarantine crossed a task, filesystem, event, or data boundary")


def _error_payload(response) -> dict:
    return json.loads(bytes(response.body))["error"]


def test_stats_query_create_returns_fixed_quarantine_without_job_creation() -> None:
    response = asyncio.run(create_stats_v2_query_job())

    assert response.status_code == 409
    assert _error_payload(response) == {
        "code": "HOST_EVIDENCE_RECEIPT_REQUIRED",
        "message": "stats query jobs require Go host evidence authority",
        "retryable": False,
        "details": {},
    }
    assert _HOSTILE not in bytes(response.body).decode("utf-8")


def test_stats_query_create_is_body_opaque_for_malformed_or_hostile_json() -> None:
    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    client = TestClient(app)

    for body in (
        f'{{"case_id":"{_HOSTILE}"',
        json.dumps({"case_id": _CASE_ID, "unexpected": _HOSTILE}),
    ):
        response = client.post(
            "/api/v1/analysis/stats/v2/query/jobs",
            content=body,
            headers={"content-type": "application/json"},
        )
        assert response.status_code == 409
        assert response.json()["error"]["code"] == "HOST_EVIDENCE_RECEIPT_REQUIRED"
        assert _HOSTILE not in response.text


def test_stats_query_create_route_has_no_body_or_service_dependency() -> None:
    route = next(
        item
        for item in stats_api.router.routes
        if getattr(item, "path", "") == "/analysis/stats/v2/query/jobs"
    )

    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []


@pytest.mark.parametrize(
    ("path", "handler", "contract"),
    (
        (
            "/analysis/stats/v2/query/rows/direct",
            query_stats_v2_rows_direct,
            "StatsRowsPublicBoundaryV1",
        ),
        (
            "/analysis/stats/v2/query/txn-rows/direct",
            query_stats_v2_txn_rows_direct,
            "StatsTxnRowsPublicBoundaryV1",
        ),
    ),
)
def test_stats_direct_query_routes_block_before_body_or_data_access(path, handler, contract) -> None:
    direct_response = asyncio.run(handler())
    direct_data = json.loads(bytes(direct_response.body))["data"]
    assert direct_data["contract"] == contract
    assert direct_data["semantic_status"] == "blocked"
    assert direct_data["rows"] == []
    assert direct_data["fact_answer_allowed"] is False

    route = next(item for item in stats_api.router.routes if getattr(item, "path", "") == path)
    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []


@pytest.mark.parametrize(
    "path",
    (
        "/api/v1/analysis/stats/v2/query/rows/direct",
        "/api/v1/analysis/stats/v2/query/txn-rows/direct",
    ),
)
def test_stats_direct_query_routes_are_body_opaque(path: str) -> None:
    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    response = TestClient(app).post(
        path,
        content=f'{{"case_id":"{_HOSTILE}"',
        headers={"content-type": "application/json"},
    )

    assert response.status_code == 200
    assert response.json()["data"]["semantic_status"] == "blocked"
    assert _HOSTILE not in response.text


@pytest.mark.parametrize(
    "route_path",
    (
        "/analysis/stats/v2/meta",
        "/analysis/stats/v2/case-overview",
        "/analysis/stats/v2/chart-dashboard",
        "/analysis/stats/v2/chart-detail-rows",
        "/analysis/stats/v2/tree",
        "/analysis/stats/v2/account-txn-rows",
    ),
)
def test_stats_case_fact_routes_are_body_opaque_before_any_data_access(route_path: str) -> None:
    route = next(item for item in stats_api.router.routes if getattr(item, "path", "") == route_path)
    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []

    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    response = TestClient(app).post(
        "/api/v1" + route_path,
        content=f'{{"case_id":"{_HOSTILE}"',
        headers={"content-type": "application/json"},
    )
    assert response.status_code == 409
    assert response.json()["error"] == {
        "code": "HOST_EVIDENCE_RECEIPT_REQUIRED",
        "message": "stats case facts require Go host evidence authority",
        "retryable": False,
        "details": {},
    }
    assert _HOSTILE not in response.text


@pytest.mark.parametrize(
    "route_path",
    (
        "/analysis/stats/v2/account-delete-info",
        "/analysis/stats/v2/update-account-info",
        "/analysis/stats/v2/delete-accounts",
        "/analysis/stats/v2/doc-pending",
    ),
)
def test_stats_mutation_routes_require_host_grant_before_body_or_service_access(route_path: str) -> None:
    route = next(item for item in stats_api.router.routes if getattr(item, "path", "") == route_path)
    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []

    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    response = TestClient(app).post(
        "/api/v1" + route_path,
        content=f'{{"case_id":"{_HOSTILE}"',
        headers={"content-type": "application/json"},
    )
    assert response.status_code == 409
    assert response.json()["error"] == {
        "code": "HOST_EXECUTION_GRANT_REQUIRED",
        "message": "stats mutation requires a current Go host execution grant",
        "retryable": False,
        "details": {},
    }
    assert _HOSTILE not in response.text


@pytest.mark.parametrize(
    "route_path",
    (
        "/analysis/stats/v2/export/default-path",
        "/analysis/stats/v2/export/jobs",
    ),
)
def test_stats_export_create_routes_are_body_opaque_before_artifact_access(route_path: str) -> None:
    route = next(item for item in stats_api.router.routes if getattr(item, "path", "") == route_path)
    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []

    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    response = TestClient(app).post(
        "/api/v1" + route_path,
        content=f'{{"case_id":"{_HOSTILE}"',
        headers={"content-type": "application/json"},
    )
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED"
    assert _HOSTILE not in response.text


@pytest.mark.parametrize("method", ("get", "post"))
def test_stats_export_job_routes_do_not_resolve_dependencies_or_echo_job_id(method: str) -> None:
    hostile_job_id = "6222020202020202020-sk-export-secret"
    suffix = "" if method == "get" else "/cancel"
    route_path = "/analysis/stats/v2/export/jobs/{job_id}" + suffix
    route = next(item for item in stats_api.router.routes if getattr(item, "path", "") == route_path)
    assert route.dependant.body_params == []
    assert route.dependant.dependencies == []

    app = FastAPI()
    app.include_router(stats_api.router, prefix="/api/v1")
    client = TestClient(app)
    request = getattr(client, method)
    response = request(f"/api/v1/analysis/stats/v2/export/jobs/{hostile_job_id}{suffix}")
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED"
    assert hostile_job_id not in response.text


@pytest.mark.parametrize(
    "handler",
    (
        get_stats_v2_query_job,
        get_stats_v2_query_job_result,
        cancel_stats_v2_query_job,
    ),
)
def test_stats_query_job_routes_do_not_lookup_or_echo_unbound_job_id(handler) -> None:
    response = asyncio.run(handler(_HOSTILE))

    assert response.status_code == 409
    assert _error_payload(response)["code"] == "HOST_EVIDENCE_RECEIPT_REQUIRED"
    assert _HOSTILE not in bytes(response.body).decode("utf-8")


def test_stats_query_service_methods_fail_before_task_result_or_event_access() -> None:
    service = object.__new__(StatsJobService)
    service._task_service = _Poison()
    service._executor = _Poison()
    service._ws_manager = _Poison()

    with pytest.raises(StatsQueryJobQuarantinedError, match="^host_evidence_receipt_required$"):
        service.create_query_job(
            case_id=_CASE_ID,
            query="rows",
            request_id=_HOSTILE,
            mode="inAccount",
            selected=[_HOSTILE],
            date_start="",
            date_end="",
            key_type="account",
            key_value=_HOSTILE,
            direction="all",
            sort_col="txn_time",
            sort_dir="asc",
            limit=200,
            cursor=None,
            search_text=_HOSTILE,
            row_sort_col="",
            row_sort_dir="desc",
            row_offset=0,
            row_limit=100,
        )
    for operation in (
        service.get_query_job,
        service.get_query_job_result,
        service.cancel_query_job,
        service._run_query_job,
    ):
        with pytest.raises(StatsQueryJobQuarantinedError, match="^host_evidence_receipt_required$"):
            operation(_HOSTILE)


def test_stats_job_startup_does_not_create_or_gc_legacy_query_result_directory(tmp_path) -> None:
    legacy_dir = tmp_path / "stats_query_results"
    legacy_file = legacy_dir / "job-legacy.json"
    legacy_dir.mkdir()
    legacy_file.write_text(_HOSTILE, encoding="utf-8")

    service = StatsJobService(
        task_service=_Poison(),
        stats_service=SimpleNamespace(storage=SimpleNamespace(app_dir=tmp_path)),
        ws_manager=_Poison(),
        ws_sequence=lambda: 1,
        max_workers=1,
    )
    try:
        assert legacy_file.read_text(encoding="utf-8") == _HOSTILE
        assert not (tmp_path / "stats_export_requests").exists()
    finally:
        service.shutdown()


def test_legacy_query_result_cannot_be_reused_as_an_export_source() -> None:
    resolver = StatsExportSourceResolver()
    with pytest.raises(ValueError, match="requires Go host evidence authority"):
        resolver.resolve_sheets(
            job_id="job-export",
            request={
                "sheets": [
                    {
                        "source_result_ref": {"path": "/private/stats_query_results/job.json"},
                        "row_keys": ["account_no"],
                    }
                ]
            },
        )


def test_unchecked_rows_never_default_missing_total_to_zero() -> None:
    payload = StatsV2RowsResultPayload(rows=[], status="no-selection")
    assert payload.total is None
