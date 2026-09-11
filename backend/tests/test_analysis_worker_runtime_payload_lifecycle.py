from __future__ import annotations

import threading
from concurrent.futures import Future

import pytest

from app.domain.analysis_worker_service import (
    AnalysisMaterializationQuarantinedError,
    AnalysisWorkerService,
)
from app.tasks.models import TaskType


_SENTINEL = "6222020202020202020 /private/case.csv SELECT * FROM secret_accounts"


class _Poison:
    def __getattr__(self, _name: str):
        raise AssertionError("materialize quarantine crossed a task, worker, or data boundary")


@pytest.mark.parametrize("dispatch_mode", ("thread", "queue"))
def test_materialize_job_creation_fails_before_task_or_thread_effect(dispatch_mode: str) -> None:
    worker = object.__new__(AnalysisWorkerService)
    worker._task_service = _Poison()
    worker._executor = _Poison()

    with pytest.raises(AnalysisMaterializationQuarantinedError, match="^host_evidence_receipt_required$"):
        worker.create_feature_mart_materialize_job(
            case_id="case-alpha",
            account_keys=[_SENTINEL],
            dispatch_mode=dispatch_mode,
        )


def test_materialize_runner_fails_before_task_lookup_or_transition() -> None:
    worker = object.__new__(AnalysisWorkerService)
    worker._task_service = _Poison()
    worker._analysis_service = _Poison()

    with pytest.raises(AnalysisMaterializationQuarantinedError, match="^host_evidence_receipt_required$"):
        worker._run_feature_mart_materialize_job("task-poison")


def test_queue_maintenance_never_claims_materialize_tasks() -> None:
    claimed_types: list[list[TaskType]] = []

    class _TaskService:
        @staticmethod
        def claim_next_task(*, task_types, case_id):
            del case_id
            claimed_types.append(list(task_types))
            return None

    worker = object.__new__(AnalysisWorkerService)
    worker._task_service = _TaskService()

    assert worker.run_next_queued_job(case_id="case-alpha") is None
    assert claimed_types == [[TaskType.ANALYSIS_MAINTENANCE]]


def test_submit_cannot_retain_future_when_runner_finishes_before_registration() -> None:
    worker = object.__new__(AnalysisWorkerService)
    worker._lock = threading.RLock()
    worker._futures = {}

    class _SynchronousExecutor:
        @staticmethod
        def submit(runner, task_id: str) -> Future:
            future = Future()
            runner(task_id)
            future.set_result(None)
            return future

    worker._executor = _SynchronousExecutor()
    worker._submit("task-fast", lambda task_id: worker._forget_future(task_id))

    assert "task-fast" not in worker._futures
