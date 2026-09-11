from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Callable, Dict, List, Optional, Set, Tuple

from app.core.fc_import_projection import (
    FcProjectionSchema,
    ImportProjection,
    build_import_projection,
    sql_literal,
)


HeaderMapReader = Callable[
    [Path, str],
    Tuple[List[str], Dict[str, str], Set[str], Dict[str, int]],
]


@dataclass(frozen=True)
class ImportHeaderState:
    raw_headers: List[str]
    header_map: Dict[str, str]
    header_set: Set[str]
    header_index: Dict[str, int]
    use_positional: bool
    col_names: List[str]
    column_map_sql: str

    @classmethod
    def from_csv(
        cls,
        path: Path,
        encoding: str,
        *,
        read_header_map: HeaderMapReader,
    ) -> "ImportHeaderState":
        raw_headers, mapped_headers, header_set, indexed_headers = read_header_map(path, encoding)
        use_positional = any("\t" in (header or "") for header in raw_headers)
        col_names = [f"c{index}" for index in range(len(raw_headers))]
        column_map_sql = ""
        if use_positional and col_names:
            column_map_sql = (
                "{" + ", ".join(f"{sql_literal(name)}: 'VARCHAR'" for name in col_names) + "}"
            )
        return cls(
            raw_headers=raw_headers,
            header_map=mapped_headers,
            header_set=header_set,
            header_index=indexed_headers,
            use_positional=use_positional,
            col_names=col_names,
            column_map_sql=column_map_sql,
        )


@dataclass(frozen=True)
class ImportProjectionState:
    header: ImportHeaderState
    projection: ImportProjection

    @classmethod
    def from_csv(
        cls,
        path: Path,
        encoding: str,
        *,
        read_header_map: HeaderMapReader,
        schema: FcProjectionSchema,
        field_mapping: Optional[Dict[str, str]],
        alias_map: Dict[str, List[str]],
        precleaned_source: bool,
        supports_json_object: bool,
        derived_header_values: Optional[Dict[str, str]] = None,
    ) -> "ImportProjectionState":
        header = ImportHeaderState.from_csv(path, encoding, read_header_map=read_header_map)
        projection = build_import_projection(
            schema=schema,
            raw_headers=header.raw_headers,
            header_map_value=header.header_map,
            header_set=header.header_set,
            header_index_value=header.header_index,
            use_positional=header.use_positional,
            col_names=header.col_names,
            field_mapping=field_mapping,
            alias_map=alias_map,
            precleaned_source=precleaned_source,
            supports_json_object=supports_json_object,
            derived_header_values=derived_header_values,
        )
        return cls(header=header, projection=projection)
