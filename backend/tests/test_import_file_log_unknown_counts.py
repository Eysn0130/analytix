from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from app.api.v1 import import_files as import_files_api
from app.core.db_engine import DuckDBEngine
from app.core.fc_import_file_log import update_import_progress, upsert_import_file_log
from app.core.fc_import_tables import ImportFileLogSchemaMigrationError, ensure_fc_tables
from app.core.import_count_semantics import (
    CLEANING_COUNTS_VERSION,
    IMPORT_COUNTS_VERSION,
    MAX_PUBLIC_IMPORT_COUNT,
    project_import_file_log_counts,
    project_persisted_import_file_log_counts,
)
from app.domain.case_service import CaseService
from app.repositories import cleaning_status_store
from app.schemas.cases import CaseImportLogDTO
from app.schemas.import_files import ImportFileLogDTO


_ROW_COUNT_FIELDS = (
    "rows_total",
    "rows_imported",
    "rows_imported_raw",
    "rows_imported_norm",
    "rows_dedup",
    "rows_error",
    "rows_skipped_non_data",
)


def _completed_row(**overrides: object) -> dict[str, object]:
    row: dict[str, object] = {
        "file_id": "file-alpha",
        "status": "已完成",
        "finished_at": "2026-07-21 00:00:00",
        "cleaned_status": "done",
        "cleaned_finished_at": "2026-07-21 00:01:00",
    }
    row.update(overrides)
    return row


def test_legacy_missing_import_log_counts_remain_unknown() -> None:
    projected = project_import_file_log_counts(_completed_row())

    assert projected["size"] is None
    assert projected["cleaned_rows_affected"] is None
    assert all(projected[field] is None for field in _ROW_COUNT_FIELDS)


def test_mixed_and_invalid_import_log_counts_never_become_zero() -> None:
    projected = project_import_file_log_counts(
        _completed_row(
            size=0,
            rows_total=5,
            rows_imported=None,
            rows_imported_raw=True,
            rows_imported_norm=0,
            rows_dedup=-1,
            rows_error="2",
            rows_skipped_non_data=MAX_PUBLIC_IMPORT_COUNT + 1,
            cleaned_rows_affected=1.5,
        )
    )

    assert projected["size"] == 0
    assert projected["rows_total"] == 5
    assert projected["rows_imported"] is None
    assert projected["rows_imported_raw"] is None
    assert projected["rows_imported_norm"] == 0
    assert projected["rows_dedup"] is None
    assert projected["rows_error"] is None
    assert projected["rows_skipped_non_data"] is None
    assert projected["cleaned_rows_affected"] is None


def test_nonterminal_and_failed_placeholder_zero_counts_are_unknown() -> None:
    for status in ("导入中", "running", "失败", "failed", "已取消"):
        projected = project_import_file_log_counts(
            {
                **_completed_row(),
                "status": status,
                **{field: 0 for field in _ROW_COUNT_FIELDS},
                "cleaned_status": "failed",
                "cleaned_rows_affected": 0,
            }
        )
        assert all(projected[field] is None for field in _ROW_COUNT_FIELDS)
        assert projected["cleaned_rows_affected"] is None


def test_successful_explicit_zero_counts_are_preserved() -> None:
    projected = project_import_file_log_counts(
        _completed_row(
            size=0,
            cleaned_rows_affected=0,
            **{field: 0 for field in _ROW_COUNT_FIELDS},
        )
    )

    assert projected["size"] == 0
    assert projected["cleaned_rows_affected"] == 0
    assert all(projected[field] == 0 for field in _ROW_COUNT_FIELDS)


def test_persisted_legacy_default_zeros_require_current_verification_markers() -> None:
    projected = project_persisted_import_file_log_counts(
        _completed_row(
            cleaned_rows_affected=0,
            **{field: 0 for field in _ROW_COUNT_FIELDS},
        )
    )

    assert projected["cleaned_rows_affected"] is None
    assert all(projected[field] is None for field in _ROW_COUNT_FIELDS)
    assert "import_counts_version" not in projected
    assert "cleaning_counts_version" not in projected


def test_persisted_verified_mixed_counts_preserve_only_valid_values() -> None:
    projected = project_persisted_import_file_log_counts(
        _completed_row(
            import_counts_version=IMPORT_COUNTS_VERSION,
            cleaning_counts_version=CLEANING_COUNTS_VERSION,
            rows_total=5,
            rows_imported=None,
            rows_imported_norm=0,
            rows_error="2",
            cleaned_rows_affected=0,
        )
    )

    assert projected["rows_total"] == 5
    assert projected["rows_imported"] is None
    assert projected["rows_imported_norm"] == 0
    assert projected["rows_error"] is None
    assert projected["cleaned_rows_affected"] == 0


@pytest.mark.parametrize("invalid", (True, -1, 1.5, "0", MAX_PUBLIC_IMPORT_COUNT + 1))
def test_import_log_public_models_reject_invalid_counts(invalid: object) -> None:
    with pytest.raises(ValidationError):
        ImportFileLogDTO(file_id="file-alpha", rows_total=invalid)
    with pytest.raises(ValidationError):
        CaseImportLogDTO(
            title="alpha.csv",
            time="",
            status="unknown",
            msg="导入数量未知",
            rows_imported=invalid,
        )


def test_import_log_public_models_default_missing_counts_to_none() -> None:
    file_log = ImportFileLogDTO(file_id="file-alpha").model_dump()
    case_log = CaseImportLogDTO(
        title="alpha.csv",
        time="",
        status="unknown",
        msg="导入数量未知",
    ).model_dump()

    assert all(file_log[field] is None for field in _ROW_COUNT_FIELDS)
    assert file_log["cleaned_rows_affected"] is None
    assert all(
        case_log[field] is None
        for field in (
            "rows_total",
            "rows_imported",
            "rows_dedup",
            "rows_error",
            "rows_skipped_non_data",
        )
    )
    assert ImportFileLogDTO.model_json_schema()["additionalProperties"] is False
    assert CaseImportLogDTO.model_json_schema()["additionalProperties"] is False


def test_import_file_api_projects_failure_missing_invalid_and_explicit_zero() -> None:
    class _CaseService:
        @staticmethod
        def is_case_deleted(_case_id: str) -> bool:
            return False

    class _ImportService:
        @staticmethod
        def list_file_logs(**_kwargs):
            return [
                {
                    "file_id": "failed",
                    "status": "失败",
                    "finished_at": "2026-07-21 00:00:00",
                    "rows_total": 0,
                },
                _completed_row(file_id="legacy"),
                _completed_row(file_id="invalid", rows_total="0"),
                _completed_row(
                    file_id="empty",
                    cleaned_rows_affected=0,
                    **{field: 0 for field in _ROW_COUNT_FIELDS},
                ),
            ]

    response = import_files_api.list_import_files(
        case_id="case-alpha",
        case_service=_CaseService(),
        import_service=_ImportService(),
    )
    payload = json.loads(response.body.decode("utf-8"))

    assert response.status_code == 200
    failed, legacy, invalid, empty = payload["data"]["items"]
    assert failed["rows_total"] is None
    assert legacy["rows_total"] is None
    assert invalid["rows_total"] is None
    assert empty["rows_total"] == 0
    assert empty["cleaned_rows_affected"] == 0


def test_case_import_summary_distinguishes_unknown_from_explicit_zero() -> None:
    class _Repository:
        @staticmethod
        def get_case_import_overview(_case_id: str, *, limit: int) -> dict:
            assert limit == 5
            return {
                "source_status": "available",
                "recent": [
                    {
                        "filename": "failed.csv",
                        "status": "失败",
                        "finished_at": "2026-07-21 00:00:00",
                        "rows_imported": 0,
                    },
                    _completed_row(
                        filename="empty.csv",
                        rows_total=0,
                        rows_imported=0,
                        rows_dedup=0,
                        rows_error=0,
                        rows_skipped_non_data=0,
                    ),
                ]
            }

    items = CaseService(_Repository())._to_import_logs("case-alpha", limit=5)

    assert items[0]["msg"] == "导入数量未知"
    assert items[0]["rows_imported"] is None
    assert items[0]["rows_skipped_non_data"] is None
    assert items[1]["msg"] == "导入 0 条"
    assert items[1]["rows_imported"] == 0
    assert items[1]["rows_skipped_non_data"] == 0
    assert items[0]["title"] == "Imported source"
    assert items[1]["title"] == "Imported source"


def test_import_log_storage_has_no_zero_defaults_and_settles_verified_zero(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "import-log-counts.duckdb")
    try:
        ensure_fc_tables(engine, include_secondary_indexes=False)
        defaults = dict(
            engine.query(
                "SELECT column_name, column_default FROM information_schema.columns "
                "WHERE table_schema='main' AND table_name='import_file_log'"
            )
        )
        for field in (
            *_ROW_COUNT_FIELDS,
            "import_counts_version",
            "cleaned_rows_affected",
            "cleaning_counts_version",
        ):
            assert defaults[field] is None

        upsert_import_file_log(
            engine,
            file_id="file-alpha",
            case_id="case-alpha",
            kind="fc_transaction",
            filename="alpha.csv",
            display_path="alpha.csv",
            stored_path="controlled-source",
            file_type="CSV",
            size=None,
            md5="",
            sha256="a" * 64,
            rows_total=None,
            status="导入中",
        )
        before = engine.query(
            "SELECT size, rows_total, rows_imported, rows_imported_raw, rows_imported_norm, "
            "rows_dedup, rows_error, rows_skipped_non_data, import_counts_version, "
            "cleaned_rows_affected, cleaning_counts_version "
            "FROM import_file_log WHERE file_id='file-alpha'"
        )[0]
        assert all(value is None for value in before)

        update_import_progress(
            engine,
            "file-alpha",
            0,
            status="已完成",
            rows_total=0,
            rows_imported_raw=0,
            rows_imported_norm=0,
            rows_dedup=0,
            rows_error=0,
            rows_skipped_non_data=0,
        )
        imported = engine.query(
            "SELECT rows_total, rows_imported, rows_imported_raw, rows_imported_norm, "
            "rows_dedup, rows_error, rows_skipped_non_data, import_counts_version "
            "FROM import_file_log "
            "WHERE file_id='file-alpha'"
        )[0]
        assert imported == (0, 0, 0, 0, 0, 0, 0, IMPORT_COUNTS_VERSION)

        cursor = engine.connection.cursor()
        cleaning_status_store.update_clean_status(
            object(),
            cursor,
            ["file-alpha"],
            "running",
            rows_affected=None,
        )
        assert engine.query(
            "SELECT cleaned_rows_affected, cleaning_counts_version FROM import_file_log "
            "WHERE file_id='file-alpha'"
        )[0] == (None, None)
        cleaning_status_store.update_clean_status(
            object(),
            cursor,
            ["file-alpha"],
            "done",
            finished_at="2026-07-21 00:01:00",
            rows_affected=0,
        )
        assert engine.query(
            "SELECT cleaned_rows_affected, cleaning_counts_version FROM import_file_log "
            "WHERE file_id='file-alpha'"
        )[0] == (0, CLEANING_COUNTS_VERSION)
        with pytest.raises(ValueError, match="^cleaned_rows_affected_invalid$"):
            cleaning_status_store.update_clean_status(
                object(),
                cursor,
                ["file-alpha"],
                "done",
                rows_affected=True,
            )

        upsert_import_file_log(
            engine,
            file_id="file-alpha",
            case_id="case-alpha",
            kind="fc_transaction",
            filename="alpha.csv",
            display_path="alpha.csv",
            stored_path="controlled-source",
            file_type="CSV",
            size=0,
            md5="",
            sha256="a" * 64,
            rows_total=0,
            status="running",
            rows_skipped_non_data=0,
        )
        reset = engine.query(
            "SELECT import_counts_version, cleaning_counts_version, rows_imported, "
            "cleaned_rows_affected FROM import_file_log WHERE file_id='file-alpha'"
        )[0]
        assert reset == (None, None, None, None)
    finally:
        engine.close()


def test_legacy_schema_adds_missing_count_columns_as_null_without_zero_default(tmp_path) -> None:
    engine = DuckDBEngine(tmp_path / "legacy-import-log-counts.duckdb")
    try:
        engine.execute(
            """CREATE TABLE import_file_log(
                   file_id TEXT PRIMARY KEY,
                   case_id TEXT,
                   created_at TEXT,
                   rows_total BIGINT DEFAULT 0,
                   rows_imported BIGINT DEFAULT 0,
                   status TEXT,
                   finished_at TEXT,
                   cleaned_status TEXT,
                   cleaned_finished_at TEXT,
                   cleaned_rows_affected BIGINT DEFAULT 0
               )"""
        )
        engine.execute(
            "INSERT INTO import_file_log(file_id, case_id, created_at, status, finished_at, "
            "cleaned_status, cleaned_finished_at) VALUES "
            "('legacy', 'case-a', '', '已完成', '2026-07-20', 'done', '2026-07-20')"
        )

        ensure_fc_tables(engine, include_secondary_indexes=False)

        defaults = dict(
            engine.query(
                "SELECT column_name, column_default FROM information_schema.columns "
                "WHERE table_schema='main' AND table_name='import_file_log'"
            )
        )
        for field in (
            *_ROW_COUNT_FIELDS,
            "import_counts_version",
            "cleaned_rows_affected",
            "cleaning_counts_version",
        ):
            assert defaults[field] is None
        added_values = engine.query(
            "SELECT rows_imported_raw, rows_imported_norm, rows_dedup, rows_error, "
            "rows_skipped_non_data FROM import_file_log "
            "WHERE file_id='legacy'"
        )[0]
        assert all(value is None for value in added_values)
        legacy_row = engine.query(
            "SELECT rows_total, rows_imported, rows_imported_raw, rows_imported_norm, "
            "rows_dedup, rows_error, rows_skipped_non_data, import_counts_version, status, "
            "finished_at, cleaned_rows_affected, cleaning_counts_version, cleaned_status, "
            "cleaned_finished_at FROM import_file_log WHERE file_id='legacy'"
        )[0]
        projected = project_persisted_import_file_log_counts(
            dict(
                zip(
                    (
                        *_ROW_COUNT_FIELDS,
                        "import_counts_version",
                        "status",
                        "finished_at",
                        "cleaned_rows_affected",
                        "cleaning_counts_version",
                        "cleaned_status",
                        "cleaned_finished_at",
                    ),
                    legacy_row,
                )
            )
        )
        assert all(projected[field] is None for field in _ROW_COUNT_FIELDS)
        assert projected["cleaned_rows_affected"] is None
    finally:
        engine.close()


def test_import_count_schema_migration_failure_is_not_suppressed(tmp_path) -> None:
    delegate = DuckDBEngine(tmp_path / "failed-import-log-migration.duckdb")

    class _FailingMigrationEngine:
        def execute(self, sql: str, params=None) -> None:
            if "ALTER COLUMN rows_total DROP DEFAULT" in sql:
                raise RuntimeError("private migration detail")
            delegate.execute(sql, params)

        def query(self, sql: str, params=None) -> list[tuple]:
            return delegate.query(sql, params)

    try:
        with pytest.raises(
            ImportFileLogSchemaMigrationError,
            match="^import_file_log_count_schema_migration_failed$",
        ):
            ensure_fc_tables(_FailingMigrationEngine(), include_secondary_indexes=False)  # type: ignore[arg-type]
    finally:
        delegate.close()
