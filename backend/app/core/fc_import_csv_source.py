from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from app.core.fc_import_projection import sql_literal


@dataclass(frozen=True)
class ImportCsvSource:
    path: Path
    encoding: str
    duckdb_encoding: str

    @classmethod
    def from_path(cls, path: Path, encoding: str) -> "ImportCsvSource":
        normalized_encoding = str(encoding or "").strip()
        duckdb_encoding = (
            "utf-8" if normalized_encoding.lower() == "utf-8-sig" else normalized_encoding
        )
        return cls(path=path, encoding=normalized_encoding, duckdb_encoding=duckdb_encoding)

    @classmethod
    def from_prepared_clean_path(cls, path: Path) -> "ImportCsvSource":
        return cls.from_path(path, "utf-8-sig")

    @property
    def duckdb_path(self) -> str:
        return str(self.path).replace("\\", "/")

    @property
    def csv_sql(self) -> str:
        return sql_literal(self.duckdb_path)

    @property
    def encoding_sql(self) -> str:
        return sql_literal(self.duckdb_encoding)

    @property
    def requires_encoding_preclean(self) -> bool:
        return self.duckdb_encoding.lower() not in {"utf-8", "utf-8-sig"}
