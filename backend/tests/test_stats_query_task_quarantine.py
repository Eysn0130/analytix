from __future__ import annotations

import json
from datetime import datetime, timezone
from uuid import uuid4

import pytest

from app.tasks.failure_boundary import migrated_failure_correlation_id
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import (
    TaskNotFoundError,
    TaskRuntimePayloadUnavailableError,
    TaskService,
)


_FAILURE_CODE = "task_runtime_payload_unavailable"
_HOSTILE = "6222020202020202020 /private/case.csv sk-query-secret-123456"


def _persisted_stats_query_row(*, status: TaskStatus, index: int) -> dict:
    now = datetime(2026, 7, 18, 1, index, tzinfo=timezone.utc).isoformat()
    return {
        "task_id": str(uuid4()),
        "task_type": TaskType.STATS_QUERY.value,
        "status": status.value,
        "progress": 100 if status == TaskStatus.SUCCEEDED else index,
        "case_id": "case-stats-query-quarantine",
        "metadata": {
            "request": {
                "case_id": "case-stats-query-quarantine",
                "source_result_ref": {"path": f"/private/stats_query_results/{_HOSTILE}.json"},
            },
            "result": {"rows": [{"account_no": _HOSTILE}]},
            "error": _HOSTILE,
        },
        "error": _HOSTILE,
        "created_at": now,
        "updated_at": now,
    }


def _expected_metadata(task_id: str) -> dict:
    return {
        "failure_code": _FAILURE_CODE,
        "failure_phase": "failed",
        "failure_correlation_id": migrated_failure_correlation_id(
            task_id=task_id,
            failure_code=_FAILURE_CODE,
        ),
        "runtime_payload_required": True,
    }


def test_stats_query_task_creation_rejects_before_store_or_input_processing(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    service = TaskService(persist_path=persist_path)
    before = persist_path.read_bytes() if persist_path.exists() else None

    with pytest.raises(TaskRuntimePayloadUnavailableError, match=f"^{_FAILURE_CODE}$"):
        service.create_task(
            TaskType.STATS_QUERY,
            case_id=_HOSTILE,
            metadata={"request": {"case_id": _HOSTILE, "query": _HOSTILE}},
        )

    after = persist_path.read_bytes() if persist_path.exists() else None
    assert after == before
    assert service.list_tasks() == ([], 0)


def test_persisted_stats_query_tasks_are_terminally_quarantined_and_restart_stable(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    rows = [
        _persisted_stats_query_row(status=status, index=index)
        for index, status in enumerate(TaskStatus, start=1)
    ]
    persist_path.write_text(
        json.dumps({"version": 2, "tasks": rows}, ensure_ascii=False),
        encoding="utf-8",
    )

    service = TaskService(persist_path=persist_path)
    migrated_bytes = persist_path.read_bytes()
    assert _HOSTILE.encode("utf-8") not in migrated_bytes
    persisted_by_id = {
        row["task_id"]: row
        for row in json.loads(migrated_bytes)["tasks"]
    }

    internal, total = service.list_tasks(case_id="case-stats-query-quarantine")
    assert total == len(rows)
    assert {task.task_id for task in internal} == {row["task_id"] for row in rows}
    for task in internal:
        assert task.status == TaskStatus.FAILED
        assert task.error == _FAILURE_CODE
        assert task.progress >= 1
        assert task.metadata == _expected_metadata(task.task_id)
        assert persisted_by_id[task.task_id]["metadata"]["runtime_payload_discarded"] is True
        with pytest.raises(TaskRuntimePayloadUnavailableError, match=f"^{_FAILURE_CODE}$"):
            service.read_runtime_payload(task.task_id, case_id=task.case_id)
        with pytest.raises(TaskNotFoundError):
            service.get_public_task(task.task_id, case_id="case-stats-query-quarantine")

    assert service.list_public_tasks(case_id="case-stats-query-quarantine") == ([], 0)
    assert service.list_public_tasks(
        case_id="case-stats-query-quarantine",
        task_type=TaskType.STATS_QUERY,
    ) == ([], 0)
    assert service.claim_next_task() is None
    assert service.claim_next_task(task_types=[TaskType.STATS_QUERY]) is None

    restarted = TaskService(persist_path=persist_path)
    assert persist_path.read_bytes() == migrated_bytes
    restarted_again = TaskService(persist_path=persist_path)
    assert persist_path.read_bytes() == migrated_bytes
    assert restarted_again.list_public_tasks(case_id="case-stats-query-quarantine") == ([], 0)


def test_stats_query_tasks_cannot_be_claimed_requeued_or_returned_publicly() -> None:
    service = TaskService()
    now = datetime.now(timezone.utc)
    quarantined_id = str(uuid4())
    service._tasks[quarantined_id] = TaskRecord(
        task_id=quarantined_id,
        task_type=TaskType.STATS_QUERY,
        status=TaskStatus.QUEUED,
        progress=0,
        case_id="case-stats-query-quarantine",
        metadata={},
        created_at=now,
        updated_at=now,
    )

    assert service.claim_next_task() is None
    assert service.claim_next_task(task_types=[TaskType.STATS_QUERY]) is None
    assert service.list_public_tasks(case_id="case-stats-query-quarantine") == ([], 0)
    with pytest.raises(TaskNotFoundError):
        service.get_public_task(quarantined_id, case_id="case-stats-query-quarantine")
    for status in (
        TaskStatus.QUEUED,
        TaskStatus.RUNNING,
        TaskStatus.SUCCEEDED,
        TaskStatus.CANCELED,
    ):
        with pytest.raises(TaskRuntimePayloadUnavailableError, match=f"^{_FAILURE_CODE}$"):
            service.transition_task(quarantined_id, status)


def test_public_task_listing_hides_quarantined_rows_without_hiding_supported_tasks(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    row = _persisted_stats_query_row(status=TaskStatus.QUEUED, index=1)
    persist_path.write_text(
        json.dumps({"version": 2, "tasks": [row]}, ensure_ascii=False),
        encoding="utf-8",
    )
    service = TaskService(persist_path=persist_path)
    supported = service.create_task(TaskType.IMPORT, case_id="case-stats-query-quarantine")

    public, total = service.list_public_tasks(case_id="case-stats-query-quarantine")
    assert total == 1
    assert [task.task_id for task in public] == [supported.task_id]
