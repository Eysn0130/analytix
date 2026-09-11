from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Dict, List, Optional

from app.core.db_engine import DuckDBEngine
from app.core.fc_import_bank_name import (
    bank_name_from_file_name_sql,
    derive_bank_name_from_file_name,
    derived_bank_headers_for_kind,
)
from app.core.fc_import_csv_source import ImportCsvSource
from app.core.fc_import_duckdb import supports_json_object
from app.core.fc_import_file_identity import detect_file_encoding, read_csv_header_map
from app.core.fc_import_projection import FcProjectionSchema, sql_literal
from app.core.fc_import_projection_state import HeaderMapReader, ImportProjectionState


EncodingDetector = Callable[[Path], str]
ProjectionStateFactory = Callable[[ImportCsvSource], ImportProjectionState]


@dataclass(frozen=True)
class ImportProjectionContext:
    source: ImportCsvSource
    projection_state: ImportProjectionState
    projection_state_factory: ProjectionStateFactory


def create_import_projection_context(
    *,
    engine: DuckDBEngine,
    schema: FcProjectionSchema,
    path: Path,
    csv_encoding: Optional[str],
    field_mapping: Optional[Dict[str, str]],
    alias_map: Dict[str, List[str]],
    precleaned_csv: bool,
    display_name: str = "",
    detect_encoding: EncodingDetector = detect_file_encoding,
    read_header_map: HeaderMapReader = read_csv_header_map,
) -> ImportProjectionContext:
    source = ImportCsvSource.from_path(
        path,
        str(csv_encoding or "").strip() or detect_encoding(path),
    )
    json_object_supported = supports_json_object(engine)
    derived_header_values: Dict[str, str] = {}
    derived_bank = derive_bank_name_from_file_name(display_name, kind=schema.table)
    if derived_bank:
        derived_header_values = {
            header: bank_name_from_file_name_sql(sql_literal(derived_bank))
            for header in derived_bank_headers_for_kind(schema.table)
        }

    def projection_state_factory(csv_source: ImportCsvSource) -> ImportProjectionState:
        return ImportProjectionState.from_csv(
            csv_source.path,
            csv_source.encoding,
            read_header_map=read_header_map,
            schema=schema,
            field_mapping=field_mapping,
            alias_map=alias_map,
            precleaned_source=precleaned_csv,
            supports_json_object=json_object_supported,
            derived_header_values=derived_header_values,
        )

    return ImportProjectionContext(
        source=source,
        projection_state=projection_state_factory(source),
        projection_state_factory=projection_state_factory,
    )
