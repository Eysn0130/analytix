from __future__ import annotations

from datetime import datetime, timezone
from types import SimpleNamespace

import pytest
from pydantic import ValidationError

from app.api.v1.import_jobs import _to_job_dto
from app.schemas.import_jobs import ImportJobDTO, ImportJobFileDTO
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskService


def _task(*, status: TaskStatus, metadata: object) -> SimpleNamespace:
    now = datetime.now(timezone.utc)
    return SimpleNamespace(
        task_id="00000000-0000-4000-8000-000000000001",
        task_type=TaskType.IMPORT,
        case_id="case-alpha",
        status=status,
        progress=100 if status == TaskStatus.SUCCEEDED else 25,
        metadata=metadata,
        error=None,
        created_at=now,
        updated_at=now,
    )


def test_running_import_job_metadata_without_authority_never_publishes_zero() -> None:
    projected = _to_job_dto(
        _task(
            status=TaskStatus.RUNNING,
            metadata={
                "imported_files": 0,
                "summary": {"rows_total": 0, "rows_imported_norm": 0},
                "files": [
                    {
                        "file_id": "file-alpha",
                        "display_name": "alpha.csv",
                        "status": "running",
                        "size": 0,
                        "rows_total": 0,
                        "rows_seen": 0,
                        "rows_imported_raw": 0,
                        "rows_imported_norm": 0,
                        "rows_dedup": 0,
                        "rows_error": 0,
                        "rows_skipped_non_data": 0,
                        "attempts": 0,
                    }
                ],
            },
        ),
        expected_case_id="case-alpha",
    )

    assert projected["imported_files"] is None
    assert projected["summary"] == {
        "total_files": None,
        "rows_total": None,
        "rows_seen": None,
        "rows_imported_raw": None,
        "rows_imported_norm": None,
        "rows_dedup": None,
        "rows_error": None,
        "rows_skipped_non_data": None,
        "retry_count": None,
    }
    assert projected["files"] == []


def test_succeeded_import_job_metadata_without_authority_never_publishes_zero() -> None:
    projected = _to_job_dto(
        _task(
            status=TaskStatus.SUCCEEDED,
            metadata={
                "imported_files": 0,
                "summary": {
                    "total_files": 1,
                    "rows_total": 0,
                    "rows_imported_norm": 0,
                    "private_debug": "must-not-project",
                },
                "files": [
                    {
                        "file_id": "file-alpha",
                        "display_name": "alpha.csv",
                        "status": "succeeded",
                        "size": 0,
                        "source_size": 0,
                        "rows_total": 0,
                        "rows_seen": 0,
                        "rows_imported_raw": 0,
                        "rows_imported_norm": 0,
                        "rows_dedup": 0,
                        "rows_error": 0,
                        "rows_skipped_non_data": 0,
                        "attempts": 0,
                    }
                ],
            },
        ),
        expected_case_id="case-alpha",
    )

    assert projected["imported_files"] is None
    assert projected["summary"] == {
        "total_files": None,
        "rows_total": None,
        "rows_seen": None,
        "rows_imported_raw": None,
        "rows_imported_norm": None,
        "rows_dedup": None,
        "rows_error": None,
        "rows_skipped_non_data": None,
        "retry_count": None,
    }
    assert projected["files"] == []


@pytest.mark.parametrize("invalid", [None, True, -1, 1.5, "7", 9_007_199_254_740_992])
def test_invalid_import_counts_are_unresolved_instead_of_coerced(invalid: object) -> None:
    projected = _to_job_dto(
        _task(
            status=TaskStatus.SUCCEEDED,
            metadata={
                "imported_files": invalid,
                "summary": {"rows_total": invalid},
                "files": [
                    {
                        "file_id": "file-alpha",
                        "display_name": "alpha.csv",
                        "status": "succeeded",
                        "size": invalid,
                        "rows_total": invalid,
                        "attempts": invalid,
                    }
                ],
            },
        ),
        expected_case_id="case-alpha",
    )

    assert projected["imported_files"] is None
    assert all(value is None for value in projected["summary"].values())
    assert projected["files"] == []


def test_import_job_task_service_round_trip_removes_unverified_counts(tmp_path) -> None:
    service = TaskService(persist_path=tmp_path / "tasks.json")
    unverified_counts = {
        "imported_files": 0,
        "summary": {"total_files": 1, "rows_total": 0, "rows_imported_norm": 0},
        "files": [
            {
                "file_id": "file-alpha",
                "status": "succeeded",
                "rows_total": 0,
                "rows_imported_norm": 0,
            }
        ],
    }
    task = service.create_task(
        task_type=TaskType.IMPORT,
        case_id="case-alpha",
        metadata=unverified_counts,
    )
    service.transition_task(
        task.task_id,
        TaskStatus.RUNNING,
        progress=50,
        metadata=unverified_counts,
    )
    service.transition_task(
        task.task_id,
        TaskStatus.SUCCEEDED,
        progress=100,
        metadata=unverified_counts,
    )

    public_task = service.get_public_task(task.task_id, case_id="case-alpha")
    assert public_task.metadata == {}
    projected = _to_job_dto(public_task, expected_case_id="case-alpha")
    assert projected["imported_files"] is None
    assert all(value is None for value in projected["summary"].values())
    assert projected["files"] == []


def test_import_job_public_schemas_reject_boolean_counts() -> None:
    with pytest.raises(ValidationError):
        ImportJobFileDTO(file_id="file-alpha", display_name="alpha.csv", rows_total=True)

    with pytest.raises(ValidationError):
        ImportJobDTO(
            job_id="job-alpha",
            case_id="case-alpha",
            status="succeeded",
            progress=100,
            imported_files=True,
            created_at="2026-07-20T00:00:00Z",
            updated_at="2026-07-20T00:00:00Z",
        )

    with pytest.raises(ValidationError):
        ImportJobFileDTO(
            file_id="file-alpha",
            display_name="alpha.csv",
            untrusted_count=0,
        )


def test_import_job_external_schemas_forbid_additional_properties() -> None:
    schema = ImportJobDTO.model_json_schema()

    assert schema["additionalProperties"] is False
    assert schema["$defs"]["ImportJobFileDTO"]["additionalProperties"] is False
    assert schema["$defs"]["ImportJobSummaryDTO"]["additionalProperties"] is False
