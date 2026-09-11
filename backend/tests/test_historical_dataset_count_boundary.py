from __future__ import annotations

import json
import pytest
from pydantic import ValidationError

from app.api.v1.import_files import list_import_historical_datasets
from app.core.storage import CaseStorage, _historical_dataset_count
from app.domain.case_source_state import CaseSourceUnavailableError
from app.schemas.import_files import ImportBatchActionResultDTO, ImportHistoricalDatasetDTO


def test_missing_counts_remain_unknown() -> None:
    payload = ImportHistoricalDatasetDTO(
        dataset_id="dataset-alpha",
        rows=_historical_dataset_count(None, field_name="rows"),
        cols=_historical_dataset_count(None, field_name="cols"),
    ).model_dump()
    assert payload["rows"] is None
    assert payload["cols"] is None

    omitted = ImportHistoricalDatasetDTO(dataset_id="dataset-omitted").model_dump()
    assert omitted["rows"] is None
    assert omitted["cols"] is None


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("rows", True),
        ("rows", -1),
        ("rows", 1.5),
        ("rows", "7"),
        ("rows", 9_007_199_254_740_992),
        ("cols", False),
        ("cols", -1),
        ("cols", 1.5),
        ("cols", "7"),
        ("cols", 9_007_199_254_740_992),
    ],
)
def test_malformed_counts_fail_closed(
    field: str,
    value: object,
) -> None:
    with pytest.raises(RuntimeError, match=rf"^historical_dataset_{field}_invalid$"):
        _historical_dataset_count(value, field_name=field)
    with pytest.raises(ValidationError):
        ImportHistoricalDatasetDTO(dataset_id="dataset-alpha", **{field: value})


def test_legacy_historical_dataset_catalog_is_quarantined_without_reading_storage() -> None:
    storage = object.__new__(CaseStorage)

    with pytest.raises(
        CaseSourceUnavailableError,
        match="^historical_dataset_case_binding_unavailable$",
    ):
        storage.list_datasets("case-alpha")

def test_historical_dataset_dto_rejects_extra_fields() -> None:
    with pytest.raises(ValidationError):
        ImportHistoricalDatasetDTO(
            dataset_id="dataset-alpha",
            rows=None,
            cols=None,
            untrusted=1,
        )


def test_historical_dataset_endpoint_projects_invalid_repository_rows_to_fixed_error() -> None:
    class _CaseService:
        @staticmethod
        def is_case_deleted(case_id: str) -> bool:
            assert case_id == "case-alpha"
            return False

    class _ImportService:
        @staticmethod
        def list_historical_datasets(*, case_id: str) -> list[dict[str, object]]:
            assert case_id == "case-alpha"
            return [{"dataset_id": "dataset-alpha", "rows": "0", "cols": None}]

    response = list_import_historical_datasets(
        case_id="case-alpha",
        case_service=_CaseService(),  # type: ignore[arg-type]
        import_service=_ImportService(),  # type: ignore[arg-type]
    )
    payload = json.loads(response.body)

    assert response.status_code == 500
    assert payload["error"]["code"] == "INTERNAL_ERROR"
    assert payload["error"]["message"] == "unexpected server error"
    assert "dataset-alpha" not in response.body.decode("utf-8")


def test_historical_dataset_source_unavailable_is_explicit() -> None:
    class _CaseService:
        @staticmethod
        def is_case_deleted(case_id: str) -> bool:
            assert case_id == "case-alpha"
            return False

    class _ImportService:
        @staticmethod
        def list_historical_datasets(*, case_id: str) -> list[dict[str, object]]:
            assert case_id == "case-alpha"
            raise CaseSourceUnavailableError("historical_dataset_case_binding_unavailable")

    response = list_import_historical_datasets(
        case_id="case-alpha",
        case_service=_CaseService(),  # type: ignore[arg-type]
        import_service=_ImportService(),  # type: ignore[arg-type]
    )
    payload = json.loads(response.body)

    assert response.status_code == 503
    assert payload["error"]["code"] == "SOURCE_UNAVAILABLE"
    assert payload["error"]["details"] == {"source_status": "unavailable"}


@pytest.mark.parametrize("invalid", [True, -1, 1.5, "0", 9_007_199_254_740_992])
def test_import_batch_action_result_rejects_noncanonical_counts(invalid: object) -> None:
    with pytest.raises(ValidationError):
        ImportBatchActionResultDTO(
            action="recycle",
            affected_count=invalid,
            file_ids=[],
        )


def test_import_batch_action_result_schema_is_closed() -> None:
    assert ImportBatchActionResultDTO.model_json_schema()["additionalProperties"] is False
    with pytest.raises(ValidationError):
        ImportBatchActionResultDTO(
            action="recycle",
            affected_count=0,
            file_ids=[],
            untrusted=1,
        )
