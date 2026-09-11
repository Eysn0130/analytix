from __future__ import annotations

import asyncio
import json
from types import SimpleNamespace

import pytest
from pydantic import ValidationError

from app.api.v1.analysis import run_runtime_retention_janitor, run_temp_scope_janitor
from app.schemas.analysis import AnalysisJanitorDTO, AnalysisJanitorReq


TASK_ID = "00000000-0000-4000-8000-000000000001"
COUNT_FIELDS = (
    "case_count",
    "affected_case_count",
    "affected_scope_count",
    "affected_item_count",
    "scanned_count",
    "stale_count",
    "deleted_count",
    "skipped_count",
)


class _AnalysisService:
    @staticmethod
    def run_temp_scope_janitor() -> dict:
        return {
            "job_kind": "temp_scope_janitor",
            "case_count": 0,
            "affected_case_count": 0,
            "affected_scope_count": 0,
            "items": [],
        }

    @staticmethod
    def run_runtime_retention_janitor(*, case_id: str) -> dict:
        assert case_id == "case-alpha"
        return {
            "job_kind": "runtime_retention_janitor",
            **{field: 0 for field in COUNT_FIELDS},
            "items": [],
        }


class _WorkerService:
    def __init__(self) -> None:
        self.calls: list[tuple[str, str, str]] = []

    def create_temp_scope_janitor_job(self, *, case_id: str, dispatch_mode: str):
        self.calls.append(("temp", case_id, dispatch_mode))
        return SimpleNamespace(task_id=TASK_ID)

    def create_runtime_retention_janitor_job(self, *, case_id: str, dispatch_mode: str):
        self.calls.append(("runtime", case_id, dispatch_mode))
        return SimpleNamespace(task_id=TASK_ID)


@pytest.mark.parametrize("mode", ("async", "queue"))
@pytest.mark.parametrize("kind", ("temp", "runtime"))
def test_pending_janitor_never_publishes_unexecuted_zero_counts(mode: str, kind: str) -> None:
    worker = _WorkerService()
    request = AnalysisJanitorReq(case_id="case-alpha", mode=mode)
    if kind == "temp":
        response = asyncio.run(
            run_temp_scope_janitor(
                request,
                analysis_service=_AnalysisService(),  # type: ignore[arg-type]
                analysis_worker_service=worker,  # type: ignore[arg-type]
            )
        )
        expected_job_kind = "temp_scope_janitor"
    else:
        response = asyncio.run(
            run_runtime_retention_janitor(
                request,
                analysis_service=_AnalysisService(),  # type: ignore[arg-type]
                analysis_worker_service=worker,  # type: ignore[arg-type]
            )
        )
        expected_job_kind = "runtime_retention_janitor"

    payload = json.loads(response.body)["data"]
    assert response.status_code == 202
    assert payload["result_status"] == "pending"
    assert payload["job_kind"] == expected_job_kind
    assert payload["job_id"] == TASK_ID
    assert payload["items"] == []
    assert {field: payload[field] for field in COUNT_FIELDS} == {
        field: None for field in COUNT_FIELDS
    }
    assert worker.calls == [
        (
            kind,
            "case-alpha",
            "queue" if mode == "queue" else "thread",
        )
    ]


def test_completed_janitor_preserves_verified_zero_and_unknown_fields() -> None:
    worker = _WorkerService()
    temp = asyncio.run(
        run_temp_scope_janitor(
            AnalysisJanitorReq(case_id="case-alpha", mode="sync"),
            analysis_service=_AnalysisService(),  # type: ignore[arg-type]
            analysis_worker_service=worker,  # type: ignore[arg-type]
        )
    )
    runtime = asyncio.run(
        run_runtime_retention_janitor(
            AnalysisJanitorReq(case_id="case-alpha", mode="sync"),
            analysis_service=_AnalysisService(),  # type: ignore[arg-type]
            analysis_worker_service=worker,  # type: ignore[arg-type]
        )
    )

    temp_payload = json.loads(temp.body)["data"]
    runtime_payload = json.loads(runtime.body)["data"]
    assert temp_payload["result_status"] == "completed"
    assert temp_payload["case_count"] == 0
    assert temp_payload["affected_scope_count"] == 0
    assert temp_payload["scanned_count"] is None
    assert runtime_payload["result_status"] == "completed"
    assert all(runtime_payload[field] == 0 for field in COUNT_FIELDS)
    assert worker.calls == []


def test_pending_janitor_dto_rejects_fabricated_zero_or_noncanonical_job() -> None:
    with pytest.raises(ValidationError):
        AnalysisJanitorDTO(
            result_status="pending",
            job_id=TASK_ID,
            case_count=0,
        )
    with pytest.raises(ValidationError):
        AnalysisJanitorDTO(
            result_status="pending",
            job_id="queued",
        )
