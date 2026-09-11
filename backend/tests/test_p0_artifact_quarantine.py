from __future__ import annotations

from pathlib import Path
from unittest.mock import Mock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.v1 import cases, export_jobs, import_files, import_jobs, stats
from app.domain.case_service import CaseService
from app.domain.controlled_artifact_gate import (
    CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
    CONTROLLED_SOURCE_INGESTION_REQUIRED,
    ControlledArtifactPublicationRequiredError,
    ControlledSourceIngestionRequiredError,
)
from app.domain.export_service import ExportService
from app.domain.import_service import ImportService
from app.domain.stats_export_request_store import StatsExportRequestStore
from app.domain.stats_export_writer import StatsExportWriter
from app.domain.stats_job_service import StatsJobService
from app.middleware.local_backend_auth import (
    DATA_ANALYSIS_AUTH_HEADER,
    LocalBackendAuthMiddleware,
)
from app.repositories.case_repository import CaseRepository
from app.repositories.export_repository import ExportRepository
from app.repositories.import_repository import ImportFileInput, ImportRepository
from app.repositories.document_repository import DocumentRepository


AUTH_TOKEN = "A" * 43
AUTH_HEADERS = {DATA_ANALYSIS_AUTH_HEADER: AUTH_TOKEN}


def _guarded_app() -> tuple[FastAPI, dict[str, Mock]]:
    app = FastAPI()
    app.add_middleware(LocalBackendAuthMiddleware, expected_token=AUTH_TOKEN)
    app.include_router(cases.router, prefix="/api/v1")
    app.include_router(export_jobs.router, prefix="/api/v1")
    app.include_router(import_files.router, prefix="/api/v1")
    app.include_router(import_jobs.router, prefix="/api/v1")
    app.include_router(stats.router, prefix="/api/v1")
    services = {
        "case_service": Mock(name="case_service"),
        "analysis_service": Mock(name="analysis_service"),
        "export_service": Mock(name="export_service"),
        "import_service": Mock(name="import_service"),
        "stats_service": Mock(name="stats_service"),
        "stats_job_service": Mock(name="stats_job_service"),
    }
    for name, service in services.items():
        setattr(app.state, name, service)
    return app, services


def _assert_blocked(response, *, code: str, message: str) -> None:
    assert response.status_code == 409
    payload = response.json()
    assert payload["error"]["code"] == code
    assert payload["error"]["message"] == message
    assert payload["error"]["retryable"] is False
    assert payload["error"]["details"]["phase"] == "p0_quarantine"


def _tree(root: Path) -> set[str]:
    return {str(path.relative_to(root)) for path in root.rglob("*")}


def test_authenticated_export_requests_are_quarantined_before_job_or_file_side_effects(tmp_path: Path) -> None:
    app, services = _guarded_app()
    client = TestClient(app)
    arbitrary_target = tmp_path / "renderer-controlled" / "report.xlsx"
    initial_tree = _tree(tmp_path)
    requests = [
        ("/api/v1/export/raw", {
            "case_id": "case_a",
            "export_format": "xlsx",
            "filters": {},
            "output_name": "raw",
            "target_dir": str(arbitrary_target.parent),
        }),
        ("/api/v1/export/cleaned", {
            "case_id": "case_a",
            "export_format": "xlsx",
            "filters": {},
            "output_name": "cleaned",
        }),
        ("/api/v1/analysis/stats/v2/export/default-path", {
            "case_id": "case_a",
            "date": "2026-07-15",
            "label": "stats",
        }),
        ("/api/v1/analysis/stats/v2/export/jobs", {
            "case_id": "case_a",
            "date": "2026-07-15",
            "label": "stats",
            "output_path": str(arbitrary_target),
            "write_report": False,
            "sheets": [{"name": "stats", "headers": [], "rows": []}],
        }),
        ("/api/v1/cases/case_a/archive/export", {
            "target_dir": str(arbitrary_target.parent),
        }),
    ]

    for path, body in [*requests, *requests]:
        response = client.post(path, json=body, headers=AUTH_HEADERS)
        _assert_blocked(
            response,
            code="CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED",
            message=CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        )

    assert _tree(tmp_path) == initial_tree
    assert services["case_service"].mock_calls == []
    assert services["export_service"].mock_calls == []
    assert services["stats_job_service"].mock_calls == []


def test_stats_export_request_store_is_side_effect_free_until_controlled_publication(tmp_path: Path) -> None:
    request_dir = tmp_path / "stats_export_requests"
    store = StatsExportRequestStore(request_dir=request_dir)

    assert not request_dir.exists()
    with pytest.raises(
        ControlledArtifactPublicationRequiredError,
        match=f"^{CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED}$",
    ):
        store.persist(
            job_id="job-hostile",
            request={"account_no": "6222020202020202020", "output_path": "/private/report.xlsx"},
        )
    assert not request_dir.exists()


def test_authenticated_renderer_source_paths_are_quarantined_before_local_read_or_job_creation(tmp_path: Path) -> None:
    app, services = _guarded_app()
    client = TestClient(app)
    source = tmp_path / "private-source.txt"
    source.write_text("must-not-be-read", encoding="utf-8")
    before_stat = source.stat()
    initial_tree = _tree(tmp_path)
    requests = [
        ("/api/v1/import/files/preview", {
            "case_id": "case_a",
            "files": [{"file_name": source.name, "source_path": str(source)}],
        }),
        ("/api/v1/import/jobs", {
            "case_id": "case_a",
            "files": [{"file_name": source.name, "source_path": str(source)}],
            "auto_cleaning": False,
        }),
        ("/api/v1/cases/archive/import", {"archive_path": str(source)}),
    ]

    for path, body in requests:
        response = client.post(path, json=body, headers=AUTH_HEADERS)
        _assert_blocked(
            response,
            code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
            message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        )

    after_stat = source.stat()
    assert _tree(tmp_path) == initial_tree
    assert (after_stat.st_size, after_stat.st_mtime_ns) == (before_stat.st_size, before_stat.st_mtime_ns)
    assert services["case_service"].mock_calls == []
    assert services["analysis_service"].mock_calls == []
    assert services["import_service"].mock_calls == []


def test_import_runtime_paths_are_quarantined_before_query_or_repository_access() -> None:
    app, services = _guarded_app()
    client = TestClient(app)

    for path in [
        "/api/v1/import/files/runtime-paths",
        "/api/v1/import/files/runtime-paths?case_id=case_a",
    ]:
        response = client.get(path, headers=AUTH_HEADERS)
        _assert_blocked(
            response,
            code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
            message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
        )
        assert "case_dir" not in response.text
        assert "db_path" not in response.text
        assert "raw_dir" not in response.text

    assert services["case_service"].mock_calls == []
    assert services["import_service"].mock_calls == []


@pytest.mark.parametrize(
    "path",
    [
        "/api/v1/import/files/preview",
        "/api/v1/import/jobs",
        "/api/v1/cases/archive/import",
    ],
)
def test_source_ingestion_quarantine_precedes_body_schema_validation(path: str) -> None:
    app, services = _guarded_app()
    client = TestClient(app)

    response = client.post(
        path,
        content=b'{"malformed":',
        headers={**AUTH_HEADERS, "content-type": "application/json"},
    )

    _assert_blocked(
        response,
        code="CONTROLLED_SOURCE_INGESTION_REQUIRED",
        message=CONTROLLED_SOURCE_INGESTION_REQUIRED,
    )
    assert services["case_service"].mock_calls == []
    assert services["analysis_service"].mock_calls == []
    assert services["import_service"].mock_calls == []


@pytest.mark.parametrize(
    ("call", "error_type", "message"),
    [
        (
            lambda: object.__new__(ExportService).create_job(
                case_id="case_a",
                mode="raw",
                export_format="xlsx",
                output_name="x",
                target_dir="/tmp/x",
                filters={},
            ),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(StatsJobService).create_export_job(
                case_id="case_a",
                case_name="case",
                date="2026-07-15",
                label="stats",
                output_path="/tmp/x.xlsx",
                sheets=[{"name": "stats"}],
            ),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(StatsExportWriter).resolve_output_path({"case_id": "case_a"}),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(ExportRepository).run_export(
                case_id="case_a",
                mode="raw",
                export_format="xlsx",
            ),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(CaseService).export_case_archive("case_a"),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(CaseRepository).export_case_archive("case_a"),
            ControlledArtifactPublicationRequiredError,
            CONTROLLED_ARTIFACT_PUBLICATION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportService).preview_files(
                case_id="case_a",
                files=[{"file_name": "x", "source_path": "/etc/passwd"}],
            ),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportService).get_case_paths(case_id="case_a"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportService).create_job(
                case_id="case_a",
                files=[{"file_name": "x", "source_path": "/etc/passwd"}],
                auto_cleaning=False,
            ),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(CaseService).import_case_archive("/etc/passwd"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(CaseRepository).import_case_archive("/etc/passwd"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportRepository).preview_files(
                "case_a",
                [ImportFileInput(file_name="x", source_path="/etc/passwd")],
            ),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportRepository).get_case_paths("case_a"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportRepository).prepare_files(
                "case_a",
                [ImportFileInput(file_name="x", source_path="/etc/passwd")],
            ),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(ImportService)._run_job("legacy-job"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(DocumentRepository).ingest_imported_file(
                engine=object(),
                case_id="case_a",
                file_id="file-a",
                filename="x.doc",
                display_path="x.doc",
                stored_path="/etc/passwd",
                file_type="doc",
                size=1,
                md5="",
                sha256="0" * 64,
                kind="support_file",
            ),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
        (
            lambda: object.__new__(DocumentRepository).sync_case_documents("case_a"),
            ControlledSourceIngestionRequiredError,
            CONTROLLED_SOURCE_INGESTION_REQUIRED,
        ),
    ],
)
def test_service_and_repository_entrypoints_fail_before_touching_dependencies(call, error_type, message) -> None:
    with pytest.raises(error_type, match=f"^{message}$"):
        call()
