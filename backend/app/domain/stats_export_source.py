from __future__ import annotations

class StatsExportSourceResolver:
    def resolve_sheets(self, *, job_id: str, request: dict) -> list[dict]:
        sheets = list(request.get("sheets") or [])
        resolved: list[dict] = []
        for raw_sheet in sheets:
            sheet = dict(raw_sheet) if isinstance(raw_sheet, dict) else {}
            source_ref = sheet.get("source_result_ref")
            if isinstance(source_ref, dict) and source_ref:
                raise ValueError("stats export query-result source requires Go host evidence authority")
            resolved.append(sheet)
        return resolved
