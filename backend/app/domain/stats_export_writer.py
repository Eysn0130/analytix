from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Iterable, Optional

from app.core.storage import CaseStorage
from app.domain.controlled_artifact_gate import require_controlled_artifact_publication
from app.utils.fs import safe_fs_name
from app.utils.time import utc_now


class StatsExportCanceled(RuntimeError):
    pass


@dataclass(frozen=True)
class StatsExportWriteResult:
    path: Path
    sheet_count: int


class StatsExportWriter:
    def __init__(self, *, storage: CaseStorage) -> None:
        self._storage = storage

    def resolve_output_path(self, request: dict) -> Path:
        require_controlled_artifact_publication()
        case_id = str(request.get("case_id") or "").strip()
        case_name = str(request.get("case_name") or "").strip() or case_id or "未命名案件"
        date_label = str(request.get("date") or "").strip() or utc_now().strftime("%Y-%m-%d")
        label = str(request.get("label") or "").strip() or "统计情况"
        output_path_raw = str(request.get("output_path") or "").strip()

        if output_path_raw:
            out_path = Path(output_path_raw).expanduser()
            if not out_path.is_absolute():
                out_path = (self._storage.case_dir(case_id) / out_path).resolve()
            return out_path

        base_name = safe_fs_name(f"{date_label} {case_name} {label}", f"{date_label}-stats-export")
        base_dir = self._storage.case_dir(case_id)
        base_dir.mkdir(parents=True, exist_ok=True)
        out_path = base_dir / f"{base_name}.xlsx"
        if out_path.exists():
            out_path = base_dir / f"{base_name}-{utc_now().strftime('%H%M%S')}.xlsx"
        return out_path

    def write_workbook(
        self,
        *,
        output_path: Path,
        sheets: Iterable[dict],
        should_cancel: Callable[[], bool],
        on_sheet_written: Optional[Callable[[int, int, int], None]] = None,
    ) -> StatsExportWriteResult:
        require_controlled_artifact_publication()
        try:
            from openpyxl import Workbook
        except Exception as exc:
            raise RuntimeError("missing dependency: openpyxl") from exc

        sheet_list = list(sheets or [])
        if not sheet_list:
            raise ValueError("sheets is empty")

        output_path.parent.mkdir(parents=True, exist_ok=True)
        workbook = Workbook(write_only=True)
        sheet_names: set[str] = set()
        total = max(1, len(sheet_list))

        try:
            for index, raw_sheet in enumerate(sheet_list):
                if should_cancel():
                    raise StatsExportCanceled("stats export canceled by user")

                sheet = raw_sheet if isinstance(raw_sheet, dict) else {}
                name = _safe_sheet_name(str(sheet.get("name") or "Sheet"), "Sheet", sheet_names)
                worksheet = workbook.create_sheet(title=name)

                headers = sheet.get("headers") if isinstance(sheet.get("headers"), list) else []
                if headers:
                    worksheet.append([str(item or "") for item in headers])
                rows = _sheet_rows(sheet)
                for row in rows:
                    if should_cancel():
                        raise StatsExportCanceled("stats export canceled by user")
                    worksheet.append(_export_row_values(row))

                progress = min(95, 10 + int(((index + 1) / total) * 80))
                if on_sheet_written is not None:
                    on_sheet_written(index + 1, total, progress)

            workbook.save(output_path)
        except Exception:
            try:
                workbook.close()
            except Exception:
                pass
            raise
        return StatsExportWriteResult(path=output_path, sheet_count=len(sheet_list))


def _safe_sheet_name(name: str, fallback: str, seen: set[str]) -> str:
    base = re.sub(r"[\[\]:*?/\\]", "_", str(name or "").strip())[:31] or fallback
    if base not in seen:
        seen.add(base)
        return base

    index = 2
    while index < 1000:
        suffix = f"_{index}"
        candidate = (base[: max(1, 31 - len(suffix))] + suffix)[:31]
        if candidate not in seen:
            seen.add(candidate)
            return candidate
        index += 1
    return base[:31]


def _export_row_values(row: Any) -> list[Any]:
    if isinstance(row, (list, tuple)):
        return list(row)
    return [row]


def _sheet_rows(sheet: dict) -> Iterable[Any]:
    row_iterable = sheet.get("row_iterable")
    if row_iterable is not None:
        return row_iterable
    rows = sheet.get("rows")
    return rows if isinstance(rows, list) else []
