from __future__ import annotations

from pathlib import Path
import threading

import pytest

from app.core.import_count_semantics import IMPORT_COUNTS_VERSION
from app.domain import import_service as import_service_module
from app.domain.import_service import ImportService
from app.repositories.import_repository import (
    ImportExecutionResult,
    ImportRepository,
    PreparedImportFile,
)


class _TransactionEngine:
    def __init__(self) -> None:
        self.events: list[str] = []
        self.transaction_open = False

    def execute(self, sql: str, _params=()) -> None:
        command = sql.strip().upper()
        if command == "BEGIN TRANSACTION":
            assert self.transaction_open is False
            self.transaction_open = True
            self.events.append("BEGIN")
            return
        if command == "ROLLBACK":
            assert self.transaction_open is True
            self.transaction_open = False
            self.events.append("ROLLBACK")
            return
        if command == "COMMIT":
            assert self.transaction_open is True
            self.transaction_open = False
            self.events.append("COMMIT")
            return
        raise AssertionError(f"unexpected SQL: {sql}")


def _prepared(file_id: str = "file-a") -> PreparedImportFile:
    return PreparedImportFile(
        file_id=file_id,
        display_name=f"{file_id}.csv",
        display_path=f"{file_id}.csv",
        real_path=Path(f"{file_id}.csv"),
        file_type="CSV",
        size=1,
        rows_total=1,
        kind_hint="fc_transaction",
    )


def _result(
    item: PreparedImportFile,
    *,
    status: str,
    attempts: int,
    retryable: bool = False,
) -> ImportExecutionResult:
    succeeded = status == "succeeded"
    return ImportExecutionResult(
        file_id=item.file_id,
        display_name=item.display_name,
        display_path=item.display_path,
        file_type=item.file_type,
        size=item.size,
        md5="",
        sha256="",
        kind="fc_transaction",
        status=status,
        rows_total=1 if succeeded else None,
        rows_seen=1 if succeeded else None,
        rows_imported_raw=1 if succeeded else None,
        rows_imported_norm=1 if succeeded else None,
        rows_dedup=0 if succeeded else None,
        rows_error=0 if succeeded else None,
        note="",
        error="" if succeeded else "import_file_processing_failed",
        attempts=attempts,
        rows_skipped_non_data=0 if succeeded else None,
        retryable=retryable,
    )


class _RetryRepository:
    def __init__(self, engine: _TransactionEngine, outcomes: list[object]) -> None:
        self.engine = engine
        self.outcomes = list(outcomes)

    def run_file_import(self, *, engine, item, attempts, **_kwargs):
        assert engine is self.engine
        assert engine.transaction_open is True
        engine.events.append(f"RUN:{attempts}")
        outcome = self.outcomes.pop(0)
        if isinstance(outcome, BaseException):
            raise outcome
        if outcome == "success":
            return _result(item, status="succeeded", attempts=attempts)
        return _result(item, status="failed", attempts=attempts, retryable=bool(outcome))

    def validate_import_persistence(self, case_id, *, file_ids, engine=None):
        assert case_id == "case-a"
        assert file_ids == ["file-a"]
        assert engine is self.engine
        assert engine.transaction_open is True
        engine.events.append("VALIDATE")
        return {
            "case_id": case_id,
            "file_ids": file_ids,
            "import_file_log_count": 1,
            "rows_imported_norm": 1,
            "norm_rows_by_kind": {"fc_transaction": 1},
        }


def _service(repository, *, retries: int = 1, cancel_checks=None) -> ImportService:
    service = object.__new__(ImportService)
    service._repository = repository
    service._max_retries_per_file = retries
    service._lock = threading.RLock()
    service._emit_event = lambda **_kwargs: None  # type: ignore[method-assign]
    checks = iter(cancel_checks or [])
    service._is_cancel_requested = lambda _job_id: next(checks, False)  # type: ignore[method-assign]
    return service


def test_retry_uses_fresh_transaction_and_validates_before_commit(monkeypatch) -> None:
    engine = _TransactionEngine()
    repository = _RetryRepository(engine, [True, "success"])
    service = _service(repository, retries=1)
    monkeypatch.setattr(import_service_module.time, "sleep", lambda _seconds: None)

    results, persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=[_prepared()],
        index=1,
        total_files=1,
        auto_cleaning=False,
    )

    assert engine.events == [
        "BEGIN",
        "RUN:1",
        "ROLLBACK",
        "BEGIN",
        "RUN:2",
        "VALIDATE",
        "COMMIT",
    ]
    assert engine.transaction_open is False
    assert results[0].status == "succeeded"
    assert persistence is not None


def test_exception_rolls_back_before_retry_starts(monkeypatch) -> None:
    engine = _TransactionEngine()
    repository = _RetryRepository(engine, [TimeoutError("temporary timeout"), "success"])
    service = _service(repository, retries=1)
    monkeypatch.setattr(import_service_module.time, "sleep", lambda _seconds: None)

    results, _persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=[_prepared()],
        index=1,
        total_files=1,
        auto_cleaning=False,
    )

    assert engine.events[:4] == ["BEGIN", "RUN:1", "ROLLBACK", "BEGIN"]
    assert engine.events[-2:] == ["VALIDATE", "COMMIT"]
    assert results[0].status == "succeeded"


def test_persistence_validation_failure_rolls_back_and_never_commits() -> None:
    engine = _TransactionEngine()

    class _Repository(_RetryRepository):
        def validate_import_persistence(self, *_args, **_kwargs):
            assert self.engine.transaction_open is True
            self.engine.events.append("VALIDATE")
            raise RuntimeError("persistence mismatch")

    repository = _Repository(engine, ["success"])
    service = _service(repository, retries=0)

    results, persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=[_prepared()],
        index=1,
        total_files=1,
        auto_cleaning=False,
    )

    assert engine.events == ["BEGIN", "RUN:1", "VALIDATE", "ROLLBACK"]
    assert persistence is None
    assert results[0].status == "failed"
    assert results[0].rows_total is None
    assert results[0].rows_imported_norm is None


def test_cancel_after_unit_execution_rolls_back_without_validation_or_commit() -> None:
    engine = _TransactionEngine()
    repository = _RetryRepository(engine, ["success"])
    service = _service(repository, retries=0, cancel_checks=[False, True])

    results, persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=[_prepared()],
        index=1,
        total_files=1,
        auto_cleaning=False,
    )

    assert engine.events == ["BEGIN", "RUN:1", "ROLLBACK"]
    assert persistence is None
    assert results[0].status == "canceled"
    assert results[0].rows_seen is None
    assert results[0].rows_error is None


def test_cancel_after_validation_still_rolls_back_before_commit() -> None:
    engine = _TransactionEngine()
    repository = _RetryRepository(engine, ["success"])
    service = _service(repository, retries=0, cancel_checks=[False, False, True])

    results, _persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=[_prepared()],
        index=1,
        total_files=1,
        auto_cleaning=False,
    )

    assert engine.events == ["BEGIN", "RUN:1", "VALIDATE", "ROLLBACK"]
    assert results[0].status == "canceled"


def test_group_unit_is_one_atomic_validated_transaction() -> None:
    engine = _TransactionEngine()
    items = [_prepared("file-a"), _prepared("file-b")]

    class _GroupRepository:
        def run_file_import_group(self, *, engine: object, items, attempts, **_kwargs):
            assert engine is engine_instance
            assert engine_instance.transaction_open is True
            engine_instance.events.append("RUN_GROUP")
            return [
                _result(item, status="succeeded", attempts=attempts)
                for item in items
            ]

        def validate_import_persistence(self, case_id, *, file_ids, engine=None):
            assert engine is engine_instance
            assert engine_instance.transaction_open is True
            engine_instance.events.append("VALIDATE_GROUP")
            return {
                "case_id": case_id,
                "file_ids": file_ids,
                "import_file_log_count": 2,
                "rows_imported_norm": 2,
                "norm_rows_by_kind": {"fc_transaction": 2},
            }

    engine_instance = engine
    service = _service(_GroupRepository(), retries=0)

    results, persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=items,
        index=1,
        total_files=2,
        auto_cleaning=False,
    )

    assert engine.events == ["BEGIN", "RUN_GROUP", "VALIDATE_GROUP", "COMMIT"]
    assert [result.status for result in results] == ["succeeded", "succeeded"]
    assert persistence is not None


def test_partial_group_result_rolls_back_and_downgrades_every_file() -> None:
    engine = _TransactionEngine()
    items = [_prepared("file-a"), _prepared("file-b")]

    class _GroupRepository:
        def run_file_import_group(self, *, engine: object, items, attempts, **_kwargs):
            assert engine is engine_instance
            engine_instance.events.append("RUN_GROUP")
            return [
                _result(items[0], status="succeeded", attempts=attempts),
                _result(items[1], status="failed", attempts=attempts),
            ]

        def validate_import_persistence(self, *_args, **_kwargs):
            pytest.fail("partial group must not be validated or committed")

    engine_instance = engine
    service = _service(_GroupRepository(), retries=0)

    results, persistence = service._run_import_unit_with_retry(
        engine=engine,
        case_id="case-a",
        job_id="job-a",
        items=items,
        index=1,
        total_files=2,
        auto_cleaning=False,
    )

    assert engine.events == ["BEGIN", "RUN_GROUP", "ROLLBACK"]
    assert persistence is None
    assert [result.status for result in results] == ["failed", "failed"]
    assert all(result.rows_imported_norm is None for result in results)


def test_failed_or_canceled_counts_are_not_accumulated_as_zero() -> None:
    summary = ImportService._new_summary(total_files=2)
    item = _prepared()
    failed = _result(item, status="failed", attempts=2, retryable=False)

    ImportService._accumulate(summary, failed)

    assert summary["retry_count"] == 1
    assert summary["rows_total"] is None
    assert summary["rows_seen"] is None
    assert summary["rows_imported_norm"] is None
    broken_success = _result(item, status="succeeded", attempts=1)
    broken_success.rows_imported_norm = None
    with pytest.raises(RuntimeError, match="rows_imported_norm is unavailable"):
        ImportService._accumulate(summary, broken_success)
    assert summary["retry_count"] == 1
    assert summary["rows_total"] is None
    assert summary["rows_seen"] is None


@pytest.mark.parametrize(
    "summary",
    [
        {},
        {"total_rows": None, "scope_rows": None},
        {"total_rows": 0},
        {"scope_rows": 0},
        {"total_rows": True, "scope_rows": 1},
        {"total_rows": 1, "scope_rows": False},
        {"total_rows": 1, "scope_rows": 2},
    ],
)
def test_auto_cleaning_missing_invalid_or_mismatched_counts_never_become_zero(summary) -> None:
    with pytest.raises(RuntimeError, match="auto cleaning row count"):
        ImportService._verified_auto_cleaning_count(summary)


def test_auto_cleaning_preserves_explicit_zero_and_positive_counts() -> None:
    assert ImportService._verified_auto_cleaning_count({"total_rows": 0, "scope_rows": 0}) == 0
    assert ImportService._verified_auto_cleaning_count({"total_rows": 7, "scope_rows": 7}) == 7


def test_persistence_unit_coverage_is_required_for_every_success() -> None:
    files = [
        {
            "file_id": "file-a",
            "status": "succeeded",
            "rows_imported_norm": 1,
        }
    ]

    with pytest.raises(RuntimeError, match="unit coverage mismatch"):
        ImportService._combine_persistence_units(
            case_id="case-a",
            files_summary=files,
            persistence_units=[],
        )


def test_repository_validation_uses_caller_transaction_without_closing(monkeypatch) -> None:
    class _ActiveEngine:
        def __init__(self) -> None:
            self.closed = False

        def query(self, sql: str, _params=()):
            if "FROM import_file_log" in sql:
                return [
                    (
                        "file-a",
                        "fc_transaction",
                        1,
                        IMPORT_COUNTS_VERSION,
                        "已完成",
                        "pending",
                    )
                ]
            if "FROM fc_transaction_norm" in sql:
                return [(1,)]
            raise AssertionError(f"unexpected query: {sql}")

        def close(self) -> None:
            self.closed = True

    engine = _ActiveEngine()
    repository = object.__new__(ImportRepository)
    repository._storage = type(
        "Storage",
        (),
        {"open_case_engine": staticmethod(lambda _case_id: pytest.fail("opened a second engine"))},
    )()
    monkeypatch.setattr(repository, "_table_exists", lambda _engine, _table: True)

    result = repository.validate_import_persistence(
        "case-a",
        file_ids=["file-a"],
        engine=engine,  # type: ignore[arg-type]
    )

    assert result["rows_imported_norm"] == 1
    assert engine.closed is False
