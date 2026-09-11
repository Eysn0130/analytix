from __future__ import annotations

import json
import threading
from datetime import datetime, timezone
from uuid import UUID, uuid4

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.data_analysis_deps import get_task_service
from app.api.v1.system import router as system_router
from app.core.diagnostic_file_transaction import (
    DiagnosticFileTransactionError,
    DiagnosticFileTransactionLease,
)
from app.tasks.failure_boundary import closed_task_failure_event_payload
from app.tasks.models import TaskRecord, TaskStatus, TaskType
from app.tasks.service import (
    TaskCaseBindingMismatchError,
    TaskRuntimePayloadUnavailableError,
    TaskService,
)


_SENTINEL = (
    "TRACE_SENTINEL 6222020202020202020 /Users/private/input.csv "
    "SELECT * FROM secret_accounts Traceback..."
)


def _serialized(value: object) -> str:
    return json.dumps(value, ensure_ascii=False, default=str, sort_keys=True)


def _assert_sentinel_absent(value: object) -> None:
    serialized = _serialized(value)
    for part in (
        "TRACE_SENTINEL",
        "6222020202020202020",
        "/Users/private/input.csv",
        "SELECT * FROM secret_accounts",
        "Traceback",
    ):
        assert part not in serialized


def test_private_runtime_payload_is_case_bound_and_never_enters_task_record_or_store(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    service = TaskService(persist_path=persist_path)

    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={
            "request": {
                "case_id": "case-alpha",
                "account_keys": ["6222020202020202020"],
                "source_path": "/Users/private/input.csv",
                "query": "SELECT * FROM secret_accounts",
            },
            "row_count": 2645472,
            "node_count": 19,
            "edge_count": 27,
            "caller_note": _SENTINEL,
        },
    )

    _assert_sentinel_absent(task.model_dump(mode="json"))
    _assert_sentinel_absent(service.get_task(task.task_id).model_dump(mode="json"))
    _assert_sentinel_absent(persist_path.read_text(encoding="utf-8"))
    assert "row_count" not in task.metadata
    assert "node_count" not in task.metadata
    assert "edge_count" not in task.metadata
    assert service.read_runtime_payload(task.task_id, case_id="case-alpha")["caller_note"] == _SENTINEL

    with pytest.raises(TaskCaseBindingMismatchError, match="task_case_binding_mismatch"):
        service.read_runtime_payload(task.task_id, case_id="case-bravo")

    with pytest.raises(TaskCaseBindingMismatchError, match="task_case_binding_mismatch"):
        service.create_task(
            TaskType.ANALYSIS_MATERIALIZE,
            case_id="case-alpha",
            metadata={"request": {"case_id": "case-bravo"}},
        )


def test_terminal_analysis_transition_atomically_discards_sensitive_runtime_payload(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    service = TaskService(persist_path=persist_path)
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={
            "request": {"case_id": "case-alpha", "account_key": "6222020202020202020"},
            "caller_note": _SENTINEL,
        },
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING, metadata={"stage": "processing"})

    completed = service.transition_task(
        task.task_id,
        TaskStatus.SUCCEEDED,
        progress=100,
        metadata={"stage": "completed", "result": {"account_key": _SENTINEL}},
    )

    assert completed.status == TaskStatus.SUCCEEDED
    assert completed.metadata == {"stage": "completed"}
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")
    _assert_sentinel_absent(completed.model_dump(mode="json"))
    _assert_sentinel_absent(persist_path.read_text(encoding="utf-8"))


def test_runtime_payload_discard_is_not_caller_controlled(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )

    with pytest.raises(TypeError, match="unexpected keyword argument"):
        service.transition_task(
            task.task_id,
            TaskStatus.RUNNING,
            discard_runtime_payload=True,
        )

    assert service.read_runtime_payload(task.task_id, case_id="case-alpha")["caller_note"] == _SENTINEL


@pytest.mark.parametrize(
    "terminal_status",
    (TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED),
)
def test_every_terminal_transition_unconditionally_discards_runtime_payload(
    tmp_path,
    terminal_status: TaskStatus,
) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING)

    terminal = service.transition_task(task.task_id, terminal_status)

    assert "runtime_payload_discarded" not in terminal.metadata
    assert service._tasks[task.task_id].metadata["runtime_payload_discarded"] is True
    assert task.task_id not in service._runtime_payloads
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")


@pytest.mark.parametrize("terminal_status", (TaskStatus.FAILED, TaskStatus.CANCELED))
def test_queued_terminal_transition_discards_without_caller_authority(
    tmp_path,
    terminal_status: TaskStatus,
) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )

    terminal = service.transition_task(task.task_id, terminal_status)

    assert "runtime_payload_discarded" not in terminal.metadata
    assert service._tasks[task.task_id].metadata["runtime_payload_discarded"] is True
    assert task.task_id not in service._runtime_payloads
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")


def test_terminal_runtime_payload_read_is_rejected_even_when_none_was_created(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(TaskType.ANALYSIS_MAINTENANCE, case_id="case-alpha")
    service.transition_task(task.task_id, TaskStatus.RUNNING)
    terminal = service.transition_task(task.task_id, TaskStatus.SUCCEEDED)

    assert "runtime_payload_discarded" not in terminal.metadata
    assert service._tasks[task.task_id].metadata["runtime_payload_discarded"] is True
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")


def test_version_two_terminal_without_tombstone_is_atomically_migrated(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    task_id = str(uuid4())
    now = datetime.now(timezone.utc).isoformat()
    persist_path.write_text(
        json.dumps(
            {
                "version": 2,
                "tasks": [
                    {
                        "task_id": task_id,
                        "task_type": "analysis_maintenance",
                        "status": "succeeded",
                        "progress": 100,
                        "case_id": "case-alpha",
                        "metadata": {},
                        "error": None,
                        "created_at": now,
                        "updated_at": now,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    service = TaskService(persist_path=persist_path)
    restored = service.get_task(task_id)
    persisted = json.loads(persist_path.read_text(encoding="utf-8"))

    assert restored.metadata == {}
    assert persisted["tasks"][0]["metadata"] == {"runtime_payload_discarded": True}
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task_id, case_id="case-alpha")


def test_terminal_callbacks_purge_even_manually_injected_runtime_payload() -> None:
    service = TaskService()
    task = service.create_task(TaskType.ANALYSIS_MAINTENANCE, case_id="case-alpha")
    service.transition_task(task.task_id, TaskStatus.RUNNING)
    completed = service.transition_task(task.task_id, TaskStatus.SUCCEEDED)

    service._runtime_payloads[task.task_id] = ("case-alpha", {"caller_note": _SENTINEL})
    assert service.update_task_metadata(task.task_id, {"caller_note": _SENTINEL}) == completed
    assert task.task_id not in service._runtime_payloads

    service._runtime_payloads[task.task_id] = ("case-alpha", {"caller_note": _SENTINEL})
    assert service.transition_task(task.task_id, TaskStatus.SUCCEEDED) == completed
    assert task.task_id not in service._runtime_payloads


def test_caller_cannot_self_issue_runtime_payload_discard_tombstone(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={
            "request": {"case_id": "case-alpha"},
            "runtime_payload_discarded": True,
            "caller_note": _SENTINEL,
        },
    )

    assert "runtime_payload_discarded" not in task.metadata
    assert service.read_runtime_payload(task.task_id, case_id="case-alpha")["caller_note"] == _SENTINEL


@pytest.mark.parametrize("terminal_status", (TaskStatus.FAILED, TaskStatus.CANCELED))
def test_discarded_failure_or_cancel_payload_cannot_be_retried_without_new_authority(
    tmp_path,
    terminal_status: TaskStatus,
) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING)

    terminal = service.transition_task(
        task.task_id,
        terminal_status,
    )

    assert terminal.metadata["runtime_payload_required"] is True
    assert "runtime_payload_discarded" not in terminal.metadata
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.transition_task(task.task_id, TaskStatus.QUEUED)
    assert service.get_task(task.task_id).status == terminal_status


def test_terminal_payload_discard_rolls_back_with_failed_persistence(tmp_path, monkeypatch) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING)
    persist = service._persist_locked

    def fail_persist(*_args, **_kwargs) -> None:
        raise DiagnosticFileTransactionError("simulated_persist_failure")

    monkeypatch.setattr(service, "_persist_locked", fail_persist)
    with pytest.raises(DiagnosticFileTransactionError):
        service.transition_task(
            task.task_id,
            TaskStatus.SUCCEEDED,
            metadata={"stage": "completed"},
        )
    monkeypatch.setattr(service, "_persist_locked", persist)

    assert service.get_task(task.task_id).status == TaskStatus.RUNNING
    assert service.read_runtime_payload(task.task_id, case_id="case-alpha")["caller_note"] == _SENTINEL


def test_same_terminal_late_callback_cannot_resurrect_discarded_payload(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING)
    completed = service.transition_task(
        task.task_id,
        TaskStatus.SUCCEEDED,
        metadata={"stage": "completed"},
    )

    late = service.transition_task(
        task.task_id,
        TaskStatus.SUCCEEDED,
        metadata={"result": {"account_key": _SENTINEL}},
    )

    assert late == completed
    assert late.metadata == {"stage": "completed"}
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")


@pytest.mark.parametrize(
    "terminal_status",
    (TaskStatus.SUCCEEDED, TaskStatus.FAILED, TaskStatus.CANCELED),
)
def test_target_visible_crash_cannot_recombine_terminal_task_with_old_sensitive_payload(
    tmp_path,
    monkeypatch,
    terminal_status: TaskStatus,
) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    task = service.create_task(
        TaskType.ANALYSIS_MATERIALIZE,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}, "caller_note": _SENTINEL},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING)
    replace = DiagnosticFileTransactionLease.replace

    def crash_after_target_visible(self, data, *, fault_hook=None):
        del fault_hook

        def crash(point: str) -> None:
            if point == "target_visible":
                raise RuntimeError("simulated_target_visible_crash")

        return replace(self, data, fault_hook=crash)

    monkeypatch.setattr(DiagnosticFileTransactionLease, "replace", crash_after_target_visible)
    with pytest.raises(DiagnosticFileTransactionError):
        service.transition_task(
            task.task_id,
            terminal_status,
            metadata={"stage": "completed"},
        )
    monkeypatch.setattr(DiagnosticFileTransactionLease, "replace", replace)

    recovered = service.get_task(task.task_id)
    assert recovered.status == terminal_status
    assert "runtime_payload_discarded" not in recovered.metadata
    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")
    if terminal_status != TaskStatus.SUCCEEDED:
        assert recovered.metadata["runtime_payload_required"] is True
        with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
            service.transition_task(task.task_id, TaskStatus.QUEUED)
    _assert_sentinel_absent(recovered.model_dump(mode="json"))


def test_failure_input_is_replaced_by_stable_closed_record_in_memory_file_and_restart(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    service = TaskService(persist_path=persist_path)
    task = service.create_task(
        TaskType.FLOW_BUILD,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha", "depth": 2}},
    )
    service.transition_task(task.task_id, TaskStatus.RUNNING, metadata={"stage": "processing"})

    failed = service.transition_task(
        task.task_id,
        TaskStatus.FAILED,
        progress=100,
        error=_SENTINEL,
        metadata={
            "stage": "processing",
            "nested": {"error": _SENTINEL, "traceback": ["frame", _SENTINEL]},
        },
    )

    assert failed.error == "task_failed"
    assert failed.metadata["failure_code"] == "task_failed"
    assert failed.metadata["failure_phase"] == "processing"
    assert "runtime_payload_discarded" not in failed.metadata
    correlation_id = failed.metadata["failure_correlation_id"]
    assert UUID(correlation_id).version == 4
    _assert_sentinel_absent(failed.model_dump(mode="json"))
    _assert_sentinel_absent(service.get_task(task.task_id).model_dump(mode="json"))
    _assert_sentinel_absent(service.list_tasks(case_id="case-alpha")[0][0].model_dump(mode="json"))
    _assert_sentinel_absent(persist_path.read_text(encoding="utf-8"))

    with pytest.raises(TaskRuntimePayloadUnavailableError, match="task_runtime_payload_unavailable"):
        service.read_runtime_payload(task.task_id, case_id="case-alpha")

    restarted = TaskService(persist_path=persist_path)
    restored = restarted.get_task(task.task_id)
    assert restored.error == "task_failed"
    assert restored.metadata["failure_correlation_id"] == correlation_id
    _assert_sentinel_absent(restored.model_dump(mode="json"))
    _assert_sentinel_absent(persist_path.read_text(encoding="utf-8"))


def test_legacy_nested_failure_is_scrubbed_and_atomically_migrated_without_reflection(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    task_id = str(uuid4())
    now = datetime.now(timezone.utc).isoformat()
    persist_path.write_text(
        json.dumps(
            {
                "version": 1,
                "tasks": [
                    {
                        "task_id": task_id,
                        "task_type": "import",
                        "status": "failed",
                        "progress": 37,
                        "case_id": "case-alpha",
                        "metadata": {
                            "stage": "processing",
                            "error": _SENTINEL,
                            "nested": {
                                "sql": "SELECT * FROM secret_accounts",
                                "rows": [{"account": "6222020202020202020"}],
                                "traceback": _SENTINEL,
                            },
                        },
                        "error": _SENTINEL,
                        "created_at": now,
                        "updated_at": now,
                    }
                ],
            },
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    service = TaskService(persist_path=persist_path)
    task = service.get_task(task_id)

    assert task.error == "task_failed"
    assert task.metadata["stage"] == "processing"
    assert task.metadata["failure_code"] == "task_failed"
    assert UUID(task.metadata["failure_correlation_id"]).version == 4
    _assert_sentinel_absent(task.model_dump(mode="json"))

    migrated = json.loads(persist_path.read_text(encoding="utf-8"))
    assert migrated["version"] == 2
    _assert_sentinel_absent(migrated)
    assert set(migrated["tasks"][0]["metadata"]) == {
        "stage",
        "failure_code",
        "failure_phase",
        "failure_correlation_id",
        "runtime_payload_discarded",
        "runtime_payload_required",
    }


@pytest.mark.parametrize(
    "raw",
    (
        b'{"version":1,"tasks":[],"tasks":[{"error":"TRACEBACK 6222020202020202020"}]}',
        b'{"version":1,"tasks":{"error":"TRACEBACK 6222020202020202020"}}',
        b'not-json TRACEBACK 6222020202020202020',
    ),
)
def test_corrupt_task_store_blocks_startup_without_silent_record_loss(tmp_path, raw: bytes) -> None:
    persist_path = tmp_path / "task-store.json"
    persist_path.write_bytes(raw)

    with pytest.raises(DiagnosticFileTransactionError, match="diagnostic_migration_input_invalid"):
        TaskService(persist_path=persist_path)

    assert persist_path.read_bytes() == raw


@pytest.mark.parametrize(
    "payload",
    (
        {"version": 3, "tasks": []},
        {
            "version": 2,
            "tasks": [
                {
                    "task_id": "171e9fb1-e923-4a95-b283-e3bade9ac39a",
                    "task_type": "import",
                    "status": "queued",
                    "progress": 0,
                    "case_id": "case-alpha",
                    "metadata": {},
                    "error": None,
                    "created_at": "2026-07-14T00:00:00+00:00",
                    "updated_at": "2026-07-14T00:00:00+00:00",
                },
                {
                    "task_id": "171e9fb1-e923-4a95-b283-e3bade9ac39a",
                    "task_type": "import",
                    "status": "queued",
                    "progress": 0,
                    "case_id": "case-alpha",
                    "metadata": {},
                    "error": None,
                    "created_at": "2026-07-14T00:00:00+00:00",
                    "updated_at": "2026-07-14T00:00:00+00:00",
                },
            ],
        },
    ),
)
def test_unknown_store_version_and_duplicate_task_ids_fail_closed(tmp_path, payload: dict) -> None:
    persist_path = tmp_path / "task-store.json"
    original = json.dumps(payload, separators=(",", ":"), sort_keys=True).encode("utf-8")
    persist_path.write_bytes(original)

    with pytest.raises(DiagnosticFileTransactionError, match="diagnostic_migration_input_invalid"):
        TaskService(persist_path=persist_path)

    assert persist_path.read_bytes() == original


def test_two_task_services_publish_without_lost_update_or_double_claim(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    alpha = TaskService(persist_path=persist_path)
    bravo = TaskService(persist_path=persist_path)
    start = threading.Barrier(3)
    created: list[str] = []
    errors: list[BaseException] = []

    def create(service: TaskService, case_id: str) -> None:
        try:
            start.wait(timeout=5)
            created.append(service.create_task(TaskType.IMPORT, case_id=case_id).task_id)
        except BaseException as exc:  # pragma: no cover - assertion below reports the exact failure
            errors.append(exc)

    threads = [
        threading.Thread(target=create, args=(alpha, "case-alpha")),
        threading.Thread(target=create, args=(bravo, "case-bravo")),
    ]
    for thread in threads:
        thread.start()
    start.wait(timeout=5)
    for thread in threads:
        thread.join(timeout=5)

    assert errors == []
    assert len(created) == 2
    restored = TaskService(persist_path=persist_path)
    tasks, total = restored.list_tasks()
    assert total == 2
    assert {task.task_id for task in tasks} == set(created)

    claimers = [TaskService(persist_path=persist_path), TaskService(persist_path=persist_path)]
    claim_start = threading.Barrier(3)
    claimed: list[TaskRecord | None] = []

    def claim(service: TaskService) -> None:
        claim_start.wait(timeout=5)
        claimed.append(service.claim_next_task())

    claim_threads = [threading.Thread(target=claim, args=(service,)) for service in claimers]
    for thread in claim_threads:
        thread.start()
    claim_start.wait(timeout=5)
    for thread in claim_threads:
        thread.join(timeout=5)

    claimed_ids = [task.task_id for task in claimed if task is not None]
    assert len(claimed_ids) == 2
    assert len(set(claimed_ids)) == 2


def test_restart_fails_closed_when_queued_runtime_payload_was_not_persisted(tmp_path) -> None:
    persist_path = tmp_path / "task-store.json"
    service = TaskService(persist_path=persist_path)
    created = service.create_task(
        TaskType.EXPORT,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha", "target": _SENTINEL}},
    )

    restarted = TaskService(persist_path=persist_path)
    restored = restarted.get_task(created.task_id)

    assert restored.status == TaskStatus.FAILED
    assert restored.error == "task_runtime_payload_unavailable"
    assert restored.metadata["failure_code"] == "task_runtime_payload_unavailable"
    _assert_sentinel_absent(restored.model_dump(mode="json"))
    _assert_sentinel_absent(persist_path.read_text(encoding="utf-8"))


def test_system_task_endpoints_require_valid_case_and_never_cross_case_or_project_payload(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "task-store.json")
    alpha = service.create_task(
        TaskType.IMPORT,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha", "value": _SENTINEL}},
    )
    service.create_task(
        TaskType.IMPORT,
        case_id="case-bravo",
        metadata={"request": {"case_id": "case-bravo", "value": "other"}},
    )

    app = FastAPI()
    app.include_router(system_router)
    app.dependency_overrides[get_task_service] = lambda: service

    with TestClient(app) as client:
        assert client.get("/system/tasks").status_code == 422
        assert client.get("/system/tasks", params={"case_id": " active "}).status_code == 422

        listed = client.get("/system/tasks", params={"case_id": "case-alpha"})
        assert listed.status_code == 200
        listed_payload = listed.json()
        assert listed_payload["data"]["total"] == 1
        assert listed_payload["data"]["items"][0]["task_id"] == alpha.task_id
        _assert_sentinel_absent(listed_payload)

        found = client.get(
            f"/system/tasks/{alpha.task_id}",
            params={"case_id": "case-alpha"},
        )
        assert found.status_code == 200
        _assert_sentinel_absent(found.json())

        cross_case = client.get(
            f"/system/tasks/{alpha.task_id}",
            params={"case_id": "case-bravo"},
        )
        assert cross_case.status_code == 404


def test_job_failure_event_projection_is_closed_and_cannot_reflect_caller_text() -> None:
    correlation_id = str(uuid4())

    payload = closed_task_failure_event_payload(
        service="import",
        code=_SENTINEL,
        retryable=True,
        correlation_id=correlation_id,
        details={
            "imported_files": 3,
            "failed_files": 1,
            "error": _SENTINEL,
            "nested": {"traceback": _SENTINEL},
        },
    )

    assert payload == {
        "code": "INTERNAL_ERROR",
        "message": "import job failed",
        "retryable": True,
        "details": {"imported_files": 3, "failed_files": 1},
        "correlation_id": correlation_id,
    }
    _assert_sentinel_absent(payload)

    with pytest.raises(ValueError, match="task_failure_event_service_invalid"):
        closed_task_failure_event_payload(
            service=_SENTINEL,
            code="INTERNAL_ERROR",
            retryable=False,
            correlation_id=correlation_id,
        )
