from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.api.v1 import cleaning_insights as cleaning_insights_api
from app.api.v1 import cleaning_jobs as cleaning_jobs_api
from app.api.v1 import import_files as import_files_api
from app.api.v1 import import_jobs as import_jobs_api
from app.core import fc_import_file_log
from app.core import storage as storage_module
from app.core.failure_boundary import (
    CLEANING_CANCELLED,
    CLEANING_FAILED,
    IMPORT_FILE_PROCESSING_FAILED,
    IMPORT_FILE_WARNING,
    private_exception_is_retryable,
    project_import_job_error,
)
from app.core.storage import CaseStorage
from app.domain.import_service import ImportService
from app.domain.cleaning_service import CleaningNativeUnavailableError
from app.repositories import cleaning_duckdb_meta, cleaning_status_store
from app.repositories.cleaning_failure_status import build_cleaning_failure
from app.repositories.import_repository import ImportExecutionResult, ImportRepository, PreparedImportFile
from app.schemas.import_files import ImportBatchDeleteReq
from app.schemas.cleaning_jobs import CleaningJobReq


_HOSTILE = "TRACEBACK_TOKEN SQL_TOKEN SECRET_PATH_TOKEN ACCOUNT_TOKEN PROMPT_INJECTION_TOKEN"


class _HostileException(RuntimeError):
    def __str__(self) -> str:
        raise RuntimeError(_HOSTILE)


class _CaptureEngine:
    def __init__(self) -> None:
        self.executions: list[tuple[str, tuple]] = []

    def execute(self, sql: str, params=()) -> None:
        self.executions.append((sql, tuple(params)))


def _combined_params(engine: _CaptureEngine) -> str:
    return json.dumps([params for _, params in engine.executions], ensure_ascii=False, default=str)


def test_cleaning_failure_does_not_retain_exception_and_persists_only_fixed_code(monkeypatch) -> None:
    exc = RuntimeError(_HOSTILE)
    failure = build_cleaning_failure(current_step=_HOSTILE, exc=exc)

    assert failure.base_message == CLEANING_FAILED
    assert failure.detail == CLEANING_FAILED
    assert _HOSTILE not in repr(failure)

    class _Cursor:
        def __init__(self) -> None:
            self.params: tuple = ()

        def execute(self, _sql: str, params: tuple) -> None:
            self.params = params

    cursor = _Cursor()
    monkeypatch.setattr(cleaning_duckdb_meta, "table_exists", lambda *_args: True)
    monkeypatch.setattr(
        cleaning_duckdb_meta,
        "column_names",
        lambda *_args: {"cleaned_status", "cleaned_error"},
    )

    cleaning_status_store.update_clean_status(
        object(),
        cursor,
        ["file-safe"],
        "failed",
        error=_HOSTILE,
    )

    assert CLEANING_FAILED in cursor.params
    assert _HOSTILE not in json.dumps(cursor.params, ensure_ascii=False)

    cleaning_status_store.update_clean_status(
        object(),
        cursor,
        ["file-safe"],
        "failed",
        error=CLEANING_CANCELLED,
    )
    assert CLEANING_CANCELLED in cursor.params


def test_import_file_log_writer_replaces_hostile_failure_before_duckdb() -> None:
    engine = _CaptureEngine()

    fc_import_file_log.update_import_progress(
        engine,
        "file-safe",
        0,
        status="失败",
        error=_HOSTILE,
    )

    params = _combined_params(engine)
    assert IMPORT_FILE_PROCESSING_FAILED in params
    assert _HOSTILE not in params


def test_import_repository_never_returns_or_writes_exception_text(monkeypatch) -> None:
    class _ExplodingPath:
        @property
        def suffix(self) -> str:
            raise RuntimeError(_HOSTILE)

    repository = object.__new__(ImportRepository)
    monkeypatch.setattr(repository, "_validate_prepared_item", lambda *_args: None)
    item = PreparedImportFile(
        file_id="file-safe",
        display_name="safe.csv",
        display_path="controlled-source",
        real_path=_ExplodingPath(),  # type: ignore[arg-type]
        file_type="CSV",
        size=0,
        rows_total=0,
    )
    engine = _CaptureEngine()

    result = repository.run_file_import(engine=engine, case_id="case-safe", item=item)

    assert result.status == "failed"
    assert result.error == IMPORT_FILE_PROCESSING_FAILED
    assert result.note == ""
    assert result.retryable is False
    assert result.rows_total is None
    assert result.rows_seen is None
    assert result.rows_imported_raw is None
    assert result.rows_imported_norm is None
    assert result.rows_dedup is None
    assert result.rows_error is None
    assert result.rows_skipped_non_data is None
    combined = json.dumps(result.__dict__, ensure_ascii=False, default=str) + _combined_params(engine)
    assert _HOSTILE not in combined


def test_legacy_persisted_traceback_is_projected_on_restart_read(monkeypatch) -> None:
    columns = [
        "file_id",
        "case_id",
        "kind",
        "filename",
        "display_path",
        "stored_path",
        "file_type",
        "size",
        "md5",
        "sha256",
        "rows_total",
        "rows_imported",
        "rows_imported_raw",
        "rows_imported_norm",
        "rows_dedup",
        "rows_error",
        "rows_skipped_non_data",
        "import_counts_version",
        "status",
        "error",
        "cleaned_status",
        "cleaned_started_at",
        "cleaned_finished_at",
        "cleaned_error",
        "cleaned_rows_affected",
        "cleaning_counts_version",
        "created_at",
        "finished_at",
        "recycled_at",
    ]
    legacy_row = (
        "file-safe",
        "case-safe",
        "fc_transaction",
        "safe.csv",
        "controlled-source",
        "controlled-storage",
        "CSV",
        1,
        "",
        "",
        1,
        0,
        0,
        0,
        0,
        0,
        0,
        None,
        "失败",
        f"Traceback: {_HOSTILE}",
        "failed",
        "",
        "",
        f"Traceback: {_HOSTILE}",
        0,
        None,
        "",
        "",
        "",
    )

    class _ReadEngine:
        def query(self, sql: str, _params=()):
            if "information_schema.columns" in sql:
                return [(column,) for column in columns]
            if "COUNT(1)" in sql:
                return [(0,)]
            return [legacy_row]

        def close(self) -> None:
            pass

    storage = object.__new__(CaseStorage)
    storage.open_case_engine = lambda *_args, **_kwargs: _ReadEngine()  # type: ignore[method-assign]
    monkeypatch.setattr(storage_module, "_table_exists", lambda *_args: True)

    rows = storage.list_import_files("case-safe")

    assert rows[0]["error"] == IMPORT_FILE_PROCESSING_FAILED
    assert rows[0]["cleaned_error"] == CLEANING_FAILED
    assert _HOSTILE not in json.dumps(rows, ensure_ascii=False)


def test_import_service_projects_untrusted_repository_rows_and_results() -> None:
    service = object.__new__(ImportService)
    service._repository = type(
        "Repository",
        (),
        {
            "list_import_files": staticmethod(
                lambda *_args, **_kwargs: [
                    {
                        "file_id": "file-safe",
                        "status": "failed",
                        "error": _HOSTILE,
                        "cleaned_status": "failed",
                        "cleaned_error": _HOSTILE,
                    }
                ]
            )
        },
    )()

    rows = service.list_file_logs(case_id="case-safe")
    assert rows[0]["error"] == IMPORT_FILE_PROCESSING_FAILED
    assert rows[0]["cleaned_error"] == CLEANING_FAILED

    summaries = [{"file_id": "file-safe"}]
    ImportService._merge_file_result(
        summaries,
        ImportExecutionResult(
            file_id="file-safe",
            display_name="safe.csv",
            display_path="controlled-source",
            file_type="CSV",
            size=0,
            md5="",
            sha256="",
            kind="",
            status="failed",
            rows_total=None,
            rows_seen=None,
            rows_imported_raw=None,
            rows_imported_norm=None,
            rows_dedup=None,
            rows_error=None,
            note=_HOSTILE,
            error=_HOSTILE,
            attempts=1,
            rows_skipped_non_data=None,
        ),
    )
    assert summaries[0]["error"] == IMPORT_FILE_PROCESSING_FAILED
    assert summaries[0]["note"] == IMPORT_FILE_WARNING
    assert _HOSTILE not in json.dumps(summaries, ensure_ascii=False)


def test_import_http_never_reflects_hostile_exception_or_request_tokens() -> None:
    class _CaseService:
        @staticmethod
        def get_case(_case_id: str) -> dict:
            return {"is_deleted": False}

        @staticmethod
        def is_case_deleted(_case_id: str) -> bool:
            return False

    class _ImportService:
        @staticmethod
        def list_file_logs(**_kwargs):
            raise RuntimeError(_HOSTILE)

    response = import_files_api.list_import_files(
        case_id="case-safe",
        case_service=_CaseService(),
        import_service=_ImportService(),
    )
    payload = json.loads(response.body.decode("utf-8"))
    assert response.status_code == 500
    assert payload["error"] == {
        "code": "INTERNAL_ERROR",
        "message": "unexpected server error",
        "retryable": True,
        "details": {},
    }
    assert _HOSTILE not in response.body.decode("utf-8")

    invalid = import_files_api._handle_batch_action_error(
        payload=ImportBatchDeleteReq(case_id=_HOSTILE, file_ids=[_HOSTILE]),
        exc=ValueError(_HOSTILE),
    )
    assert invalid.status_code == 400
    assert _HOSTILE not in invalid.body.decode("utf-8")


def test_import_and_cleaning_job_http_boundaries_never_reflect_failures() -> None:
    class _FailingImportService:
        @staticmethod
        def list_jobs(**_kwargs):
            raise RuntimeError(_HOSTILE)

    import_response = import_jobs_api.list_import_jobs(
        case_id="case-safe",
        page=1,
        page_size=50,
        status=None,
        import_service=_FailingImportService(),
    )
    assert import_response.status_code == 500
    assert _HOSTILE not in import_response.body.decode("utf-8")

    class _FailingCleaningService:
        @staticmethod
        def list_jobs(**_kwargs):
            raise RuntimeError(_HOSTILE)

    cleaning_response = cleaning_jobs_api.list_cleaning_jobs(
        case_id="case-safe",
        page=1,
        page_size=50,
        status=None,
        cleaning_service=_FailingCleaningService(),
    )
    assert cleaning_response.status_code == 500
    assert _HOSTILE not in cleaning_response.body.decode("utf-8")

    class _CaseService:
        @staticmethod
        def get_case(_case_id: str) -> dict:
            return {"is_deleted": False}

    class _UnavailableCleaningService:
        @staticmethod
        def create_job(**_kwargs):
            raise CleaningNativeUnavailableError(
                health={"message": _HOSTILE, "path": _HOSTILE, "prompt": _HOSTILE}
            )

    unavailable = cleaning_jobs_api.create_cleaning_job(
        payload=CleaningJobReq(case_id="case-safe"),
        case_service=_CaseService(),
        cleaning_service=_UnavailableCleaningService(),
    )
    body = json.loads(unavailable.body.decode("utf-8"))
    assert unavailable.status_code == 503
    assert body["error"] == {
        "code": "NATIVE_CLEANING_UNAVAILABLE",
        "message": "native cleaning unavailable",
        "retryable": True,
        "details": {},
    }
    assert _HOSTILE not in unavailable.body.decode("utf-8")


def test_cleaning_insights_http_boundary_never_calls_or_reflects_private_failure_source() -> None:
    class _CaseService:
        @staticmethod
        def get_case(_case_id: str) -> dict:
            return {"is_deleted": False}

    class _CleaningService:
        @staticmethod
        def list_history(**_kwargs):
            raise RuntimeError(_HOSTILE)

    response = cleaning_insights_api.list_cleaning_history(
        case_id="case-safe",
        limit=50,
        case_service=_CaseService(),
        cleaning_service=_CleaningService(),
    )
    assert response.status_code == 200
    data = json.loads(response.body.decode("utf-8"))["data"]
    assert data == {
        "contract": "CleaningHistoryPublicBoundaryV1",
        "case_id": "case-safe",
        "semantic_status": "blocked",
        "fact_answer_allowed": False,
        "blocker": "host_evidence_receipt_required",
        "items": [],
    }
    assert _HOSTILE not in response.body.decode("utf-8")


def test_hostile_exception_classification_fails_closed_without_stringification_leak() -> None:
    assert private_exception_is_retryable(_HostileException()) is False
    assert project_import_job_error("task_failed", status="failed") == "task_failed"
    assert project_import_job_error(_HOSTILE, status="failed") == "import_job_failed"


@pytest.mark.parametrize(
    "relative_path, forbidden",
    [
        ("app/repositories/import_repository.py", ("traceback.format_exc", "{exc}")),
        ("app/repositories/cleaning_failure_status.py", ("traceback.format_exc", "{exc}")),
        ("app/api/v1/import_files.py", ("message=str(exc)",)),
        ("app/api/v1/import_jobs.py", ("message=str(exc)",)),
        ("app/api/v1/cleaning_jobs.py", ("message=str(exc)",)),
        ("app/api/v1/cleaning_insights.py", ("message=str(exc)",)),
    ],
)
def test_failure_sources_have_no_raw_exception_serialization(relative_path: str, forbidden: tuple[str, ...]) -> None:
    source = (Path(__file__).resolve().parents[1] / relative_path).read_text(encoding="utf-8")
    for token in forbidden:
        assert token not in source
