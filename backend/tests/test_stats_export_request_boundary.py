from __future__ import annotations

import os
import threading
from concurrent.futures import Future
from uuid import uuid4

import pytest

import app.domain.stats_export_request_store as request_store_module
import app.domain.stats_job_service as stats_job_module
from app.domain.stats_export_request_store import (
    StatsExportRequestStore,
    StatsExportRequestUnavailableError,
)
from app.domain.stats_job_service import StatsJobService
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskService


def _allow_controlled_artifacts(monkeypatch) -> None:
    monkeypatch.setattr(request_store_module, "require_controlled_artifact_publication", lambda: None)
    monkeypatch.setattr(stats_job_module, "require_controlled_artifact_publication", lambda: None)


def test_stats_export_request_is_private_exact_job_case_and_content_bound(tmp_path, monkeypatch) -> None:
    _allow_controlled_artifacts(monkeypatch)
    store = StatsExportRequestStore(request_dir=tmp_path / "requests")
    job_id = str(uuid4())
    other_job_id = str(uuid4())
    request = {
        "case_id": "case-alpha",
        "output_path": "/private/report.xlsx",
        "sheets": [{"name": "accounts", "rows": [{"account": "6222020202020202020"}]}],
    }

    ref = store.persist(job_id=job_id, request=request)

    path = tmp_path / "requests" / f"{job_id}.json"
    assert os.stat(path).st_mode & 0o077 == 0
    assert "path" not in ref
    assert store.load(job_id=job_id, request_ref=ref) == request
    with pytest.raises(StatsExportRequestUnavailableError):
        store.load(job_id=other_job_id, request_ref=ref)
    store.delete_for_job(job_id=other_job_id)
    assert path.exists()

    with pytest.raises(StatsExportRequestUnavailableError, match="stats_export_request_conflict"):
        store.persist(job_id=job_id, request={**request, "case_id": "case-bravo"})

    path.write_text("{}", encoding="utf-8")
    with pytest.raises(StatsExportRequestUnavailableError):
        store.load(job_id=job_id, request_ref=ref)


def test_stats_export_create_update_failure_deletes_persisted_private_request(tmp_path, monkeypatch) -> None:
    _allow_controlled_artifacts(monkeypatch)
    tasks = TaskService()
    service = object.__new__(StatsJobService)
    service._task_service = tasks
    service._export_request_store = StatsExportRequestStore(request_dir=tmp_path / "requests")
    monkeypatch.setattr(tasks, "update_task_metadata", lambda *_args, **_kwargs: (_ for _ in ()).throw(RuntimeError("update failed")))

    with pytest.raises(RuntimeError, match="update failed"):
        service.create_export_job(
            case_id="case-alpha",
            case_name="Alpha",
            date="2026-07-20",
            label="stats",
            output_path="/private/report.xlsx",
            sheets=[{"name": "stats", "rows": []}],
        )

    records, total = tasks.list_tasks(case_id="case-alpha")
    assert total == 1
    assert records[0].status == TaskStatus.FAILED
    assert list((tmp_path / "requests").glob("*.json")) == []


def test_stats_running_transition_failure_still_cleans_exact_request_and_runtime_maps(tmp_path, monkeypatch) -> None:
    _allow_controlled_artifacts(monkeypatch)
    tasks = TaskService()
    task = tasks.create_task(
        TaskType.STATS_EXPORT,
        case_id="case-alpha",
        metadata={"request": {"case_id": "case-alpha"}},
    )
    store = StatsExportRequestStore(request_dir=tmp_path / "requests")
    ref = store.persist(
        job_id=task.task_id,
        request={"case_id": "case-alpha", "output_path": "/private/report.xlsx", "sheets": [{}]},
    )
    tasks.update_task_metadata(task.task_id, {"request": {"case_id": "case-alpha", "request_ref": ref}})
    reads: list[str] = []
    read_runtime_payload = tasks.read_runtime_payload

    def tracked_read(job_id: str, *, case_id: str | None):
        reads.append(job_id)
        return read_runtime_payload(job_id, case_id=case_id)

    monkeypatch.setattr(tasks, "read_runtime_payload", tracked_read)
    service = object.__new__(StatsJobService)
    service._task_service = tasks
    service._export_request_store = store
    service._lock = threading.RLock()
    service._cancel_flags = {task.task_id: threading.Event()}
    service._futures = {task.task_id: Future()}
    service._job_event_seq = {task.task_id: 1}
    service._event_dedup = {task.task_id: {"queued"}}
    monkeypatch.setattr(service, "_safe_transition", lambda *_args, **_kwargs: None)

    service._run_export_job(task.task_id)

    assert reads == [task.task_id]
    assert not (tmp_path / "requests" / f"{task.task_id}.json").exists()
    assert task.task_id not in service._cancel_flags
    assert task.task_id not in service._futures
    assert task.task_id not in service._job_event_seq
    assert task.task_id not in service._event_dedup


def test_stats_completed_future_cannot_reappear_after_inline_cleanup(tmp_path, monkeypatch) -> None:
    _allow_controlled_artifacts(monkeypatch)

    class _InlineExecutor:
        @staticmethod
        def submit(fn, *args):
            future: Future[None] = Future()
            fn(*args)
            future.set_result(None)
            return future

    service = object.__new__(StatsJobService)
    service._task_service = TaskService()
    service._export_request_store = StatsExportRequestStore(request_dir=tmp_path / "requests")
    service._executor = _InlineExecutor()
    service._lock = threading.RLock()
    service._cancel_flags = {}
    service._futures = {}
    service._job_event_seq = {}
    service._event_dedup = {}
    service._emit_event = lambda **_kwargs: None
    service._run_export_job = lambda job_id: service._cleanup_runtime(job_id)

    task = service.create_export_job(
        case_id="case-alpha",
        case_name="Alpha",
        date="2026-07-20",
        label="stats",
        output_path="/private/report.xlsx",
        sheets=[{"name": "stats", "rows": []}],
    )

    assert task.task_id not in service._futures
    assert task.task_id not in service._cancel_flags
    assert not (tmp_path / "requests" / f"{task.task_id}.json").exists()
