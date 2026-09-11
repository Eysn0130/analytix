from __future__ import annotations

from app.core.import_accelerator import ImportAcceleratorUnavailableError
from app.repositories import import_repository as import_repository_module
from app.repositories.import_repository import ImportRepository, PreparedImportFile


def test_prepare_import_csv_metadata_batch_falls_back_without_accelerator(tmp_path, monkeypatch) -> None:
    csv_path = tmp_path / "交易明细信息.csv"
    csv_path.write_text("交易时间,交易金额\n2026-01-01,10\n2026-01-02,20\n", encoding="utf-8")
    item = PreparedImportFile(
        file_id="file-1",
        display_name=csv_path.name,
        display_path=csv_path.name,
        real_path=csv_path,
        file_type="CSV",
        size=csv_path.stat().st_size,
        rows_total=0,
    )

    def fail_batch(_items):
        raise ImportAcceleratorUnavailableError("missing accelerator")

    monkeypatch.setattr(import_repository_module, "prepare_csv_batch", fail_batch)

    ImportRepository()._prepare_import_csv_metadata_batch([item], tmp_path / "cache")

    assert item.rows_total == 2
    assert item.csv_encoding == "utf-8-sig"
    assert item.csv_headers == ["交易时间", "交易金额"]
    assert item.duckdb_csv_path is None
    assert item.duckdb_csv_encoding == ""
