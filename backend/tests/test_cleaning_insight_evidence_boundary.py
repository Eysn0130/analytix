from __future__ import annotations

import inspect
import json
import threading

import pytest
from pydantic import ValidationError

from app.api.v1.cleaning_insights import (
    get_cleaning_step_detail,
    list_cleaning_history,
    list_cleaning_logs,
    list_cleaning_step_summaries,
)
from app.api.v1.cleaning_jobs import _to_job_dto
from app.domain.cleaning_service import CleaningJobNotFoundError, CleaningService
from app.repositories.cleaning_repository import (
    CleaningRepository,
    CleaningStepSourceUnavailableError,
)
from app.repositories.cleaning_step_catalog import CLEANING_STEP_DEFINITIONS
from app.schemas.cleaning_insights import (
    CleaningStepDetailDTO,
    CleaningStepSummaryDTO,
    CleaningStepSummaryListDTO,
)
from app.schemas.cleaning_jobs import CleaningJobDTO
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskRuntimePayloadUnavailableError, TaskService


_CASE_ID = "case-cleaning-boundary"
_FULL_ACCOUNT = "6222020202020202020"


class _CaseService:
    @staticmethod
    def get_case(case_id: str) -> dict:
        assert case_id == _CASE_ID
        return {"case_id": case_id, "is_deleted": False}


class _ForbiddenCleaningService:
    @staticmethod
    def list_step_summaries(*_args, **_kwargs):
        raise AssertionError("ordinary cleaning boundary queried case facts")

    list_history = list_step_summaries
    list_log_events = list_step_summaries


def _response_data(response) -> dict:
    return json.loads(bytes(response.body))["data"]


def test_ordinary_step_summary_route_never_reads_or_publishes_case_counts() -> None:
    response = list_cleaning_step_summaries(
        case_id=_CASE_ID,
        case_service=_CaseService(),
        cleaning_service=_ForbiddenCleaningService(),
    )
    data = _response_data(response)

    assert data["contract"] == "CleaningStepSummaryPublicBoundaryV1"
    assert data["case_id"] == _CASE_ID
    assert data["semantic_status"] == "blocked"
    assert data["fact_answer_allowed"] is False
    assert len(data["items"]) == len(CLEANING_STEP_DEFINITIONS)
    assert all(item["affected_rows"] is None for item in data["items"])
    assert _FULL_ACCOUNT not in json.dumps(data)


def test_ordinary_step_detail_route_never_queries_or_returns_raw_rows() -> None:
    response = get_cleaning_step_detail(
        step=1,
        case_id=_CASE_ID,
        page=1,
        page_size=5000,
        offset=0,
        case_service=_CaseService(),
    )
    data = _response_data(response)

    assert data["contract"] == "CleaningStepDetailPublicBoundaryV1"
    assert data["raw_details_exposed"] is False
    assert data["fact_answer_allowed"] is False
    assert data["items"] == []
    assert data["page"] is None
    assert _FULL_ACCOUNT not in json.dumps(data)
    assert "known_total" not in inspect.signature(get_cleaning_step_detail).parameters


def test_ordinary_history_and_logs_do_not_replay_case_facts_or_messages() -> None:
    history = _response_data(list_cleaning_history(
        case_id=_CASE_ID,
        limit=500,
        case_service=_CaseService(),
        cleaning_service=_ForbiddenCleaningService(),
    ))
    logs = _response_data(list_cleaning_logs(
        case_id=_CASE_ID,
        limit=1000,
        case_service=_CaseService(),
        cleaning_service=_ForbiddenCleaningService(),
    ))

    assert history["contract"] == "CleaningHistoryPublicBoundaryV1"
    assert logs["contract"] == "CleaningLogPublicBoundaryV1"
    assert history["items"] == []
    assert logs["items"] == []
    assert history["fact_answer_allowed"] is False
    assert logs["fact_answer_allowed"] is False


def test_boundary_models_reject_zero_upgrade_rows_and_self_reported_fact_permission() -> None:
    with pytest.raises(ValidationError):
        CleaningStepSummaryDTO(
            step=1,
            key="step-1",
            title="step",
            kind="fix",
            description="desc",
            affected_rows=0,
        )
    with pytest.raises(ValidationError):
        CleaningStepSummaryListDTO(case_id=_CASE_ID, fact_answer_allowed=True)
    with pytest.raises(ValidationError):
        CleaningStepDetailDTO(
            case_id=_CASE_ID,
            step=1,
            key="step-1",
            title="step",
            kind="fix",
            description="desc",
            items=[{"values": [_FULL_ACCOUNT]}],
        )


def test_repository_count_and_detail_errors_never_collapse_to_zero_or_empty_success() -> None:
    repository = object.__new__(CleaningRepository)

    class _Cursor:
        def __init__(self) -> None:
            self.calls = 0

        def execute(self, *_args, **_kwargs) -> None:
            self.calls += 1
            if self.calls > 1:
                raise RuntimeError("query failed")

        @staticmethod
        def fetchone():
            return (1,)

    class _Connection:
        def __init__(self) -> None:
            self.cursor_instance = _Cursor()

        def cursor(self):
            return self.cursor_instance

        @staticmethod
        def close() -> None:
            return None

    connection = _Connection()
    repository._open_step_query_connection = lambda _case_id: connection

    with pytest.raises(CleaningStepSourceUnavailableError, match="cleaning_step_source_unavailable"):
        repository.get_step_detail(_CASE_ID, 1, page=1, page_size=20)

    class _MissingCountCursor:
        @staticmethod
        def execute(*_args, **_kwargs) -> None:
            return None

        @staticmethod
        def fetchone():
            return None

    with pytest.raises(CleaningStepSourceUnavailableError, match="cleaning_step_source_unavailable"):
        repository._fetch_step_count(_MissingCountCursor(), _CASE_ID, CLEANING_STEP_DEFINITIONS[0])


def test_cleaning_job_unknown_rows_remain_null_and_case_binding_is_exact(tmp_path) -> None:
    task_service = TaskService(persist_path=tmp_path / "task-store.json")
    task = task_service.create_task(
        TaskType.CLEANING,
        case_id=_CASE_ID,
        metadata={
            "request": {"case_id": _CASE_ID},
            "cleaned_rows": 2645472,
            "summary": {"account_no": _FULL_ACCOUNT},
        },
    )
    projected = _to_job_dto(task)

    assert projected["cleaned_rows"] is None
    assert projected["summary"] == {}
    assert projected["fact_answer_allowed"] is False
    assert _FULL_ACCOUNT not in json.dumps(projected)

    cleaning_service = object.__new__(CleaningService)
    cleaning_service._task_service = task_service
    assert cleaning_service.get_job(task.task_id, case_id=_CASE_ID).task_id == task.task_id
    with pytest.raises(CleaningJobNotFoundError):
        cleaning_service.get_job(task.task_id, case_id="case-bravo")

    with pytest.raises(ValidationError):
        CleaningJobDTO(**{**projected, "cleaned_rows": 0})


def test_cleaning_worker_setup_failure_discards_payload_and_releases_runtime_state(tmp_path) -> None:
    task_service = TaskService(persist_path=tmp_path / "task-store.json")
    task = task_service.create_task(
        TaskType.CLEANING,
        case_id=_CASE_ID,
        metadata={"requested_steps": [1], "account_no": _FULL_ACCOUNT},
    )
    with task_service._lock:
        task_service._runtime_payloads.pop(task.task_id, None)

    cleaning_service = object.__new__(CleaningService)
    cleaning_service._task_service = task_service
    cleaning_service._lock = threading.RLock()
    cleaning_service._threads = {task.task_id: object()}
    cleaning_service._cancel_flags = {}
    cleaning_service._job_event_seq = {}
    cleaning_service._event_dedup = {}
    cleaning_service._event_log_state = {}

    cleaning_service._run_job(task.task_id)

    failed = task_service.get_task(task.task_id)
    assert failed.status == TaskStatus.FAILED
    assert "runtime_payload_discarded" not in failed.metadata
    assert task_service._tasks[task.task_id].metadata["runtime_payload_discarded"] is True
    assert task.task_id not in cleaning_service._threads
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        task_service.read_runtime_payload(task.task_id, case_id=_CASE_ID)
