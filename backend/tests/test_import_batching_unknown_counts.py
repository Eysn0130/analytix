from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from app.domain.import_batching import (
    import_result_row_weight,
    plan_next_import_unit,
    should_commit_import_transaction,
)
from app.repositories import import_repository as import_repository_module
from app.repositories.import_repository import ImportRepository, PreparedImportFile


@dataclass
class _Prepared:
    rows_total: object


@dataclass
class _Result:
    rows_seen: object
    rows_imported_raw: object
    rows_imported_norm: object


def test_import_batching_preserves_explicit_zero_counts() -> None:
    prepared = [_Prepared(0), _Prepared(0)]

    assert plan_next_import_unit(
        prepared,
        0,
        group_key=lambda _item: ("same",),
        file_limit=2,
        row_limit=10,
    ) == prepared
    assert import_result_row_weight(_Result(0, 0, 0)) == 0
    assert (
        should_commit_import_transaction(
            files_since_begin=0,
            rows_since_begin=0,
            file_limit=0,
            row_limit=0,
        )
        is False
    )


@pytest.mark.parametrize("invalid", (None, True, -1, 1.5, "0"))
def test_import_batching_rejects_unknown_or_invalid_counts(invalid: object) -> None:
    with pytest.raises(ValueError, match="^import_batching_rows_total_invalid$"):
        plan_next_import_unit(
            [_Prepared(invalid)],
            0,
            group_key=lambda _item: ("same",),
            file_limit=1,
            row_limit=1,
        )
    with pytest.raises(ValueError, match="^import_batching_rows_seen_invalid$"):
        import_result_row_weight(_Result(invalid, 0, 0))
    with pytest.raises(ValueError, match="^import_batching_rows_since_begin_invalid$"):
        should_commit_import_transaction(
            files_since_begin=0,
            rows_since_begin=invalid,  # type: ignore[arg-type]
            file_limit=1,
            row_limit=1,
        )


def _prepared_import_file() -> PreparedImportFile:
    return PreparedImportFile(
        file_id="file-alpha",
        display_name="alpha.csv",
        display_path="alpha.csv",
        real_path=Path("alpha.csv"),
        file_type="CSV",
        size=0,
        rows_total=2,
        kind_hint="fc_transaction",
        sha256="a" * 64,
        csv_encoding="utf-8",
        csv_headers=["交易账号", "交易时间", "交易金额"],
    )


def _run_import_with_norm_count(
    monkeypatch: pytest.MonkeyPatch,
    norm_count: object,
) -> tuple[object, list[tuple[tuple, dict]]]:
    repository = object.__new__(ImportRepository)
    repository._validate_prepared_item = lambda _case_id, _item: None  # type: ignore[method-assign]
    repository._count_rows_fast = lambda _path: 2  # type: ignore[method-assign]
    repository._record_confirmed_mapping_template = lambda **_kwargs: None  # type: ignore[method-assign]
    updates: list[tuple[tuple, dict]] = []

    monkeypatch.setattr(import_repository_module, "upsert_import_file_log", lambda *_args, **_kwargs: None)
    monkeypatch.setattr(
        import_repository_module,
        "update_import_progress",
        lambda *args, **kwargs: updates.append((args, kwargs)),
    )
    monkeypatch.setattr(
        import_repository_module,
        "import_fc_csv_into_db",
        lambda **_kwargs: (2, 2, norm_count, ""),
    )

    result = repository.run_file_import(
        engine=object(),  # type: ignore[arg-type]
        case_id="case-alpha",
        item=_prepared_import_file(),
    )
    return result, updates


def test_single_file_import_preserves_verified_zero_normalized_rows(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    result, updates = _run_import_with_norm_count(monkeypatch, 0)

    assert result.status == "succeeded"
    assert result.rows_imported_raw == 2
    assert result.rows_imported_norm == 0
    assert len(updates) == 1
    assert updates[0][0][2] == 0
    assert updates[0][1]["rows_imported_norm"] == 0


@pytest.mark.parametrize("invalid", (None, True, -1, 1.5, "0"))
def test_single_file_import_does_not_settle_invalid_normalized_count_as_zero(
    monkeypatch: pytest.MonkeyPatch,
    invalid: object,
) -> None:
    result, updates = _run_import_with_norm_count(monkeypatch, invalid)

    assert result.status == "failed"
    assert len(updates) == 1
    assert updates[0][0][2] is None
    assert updates[0][1]["status"] == "失败"
