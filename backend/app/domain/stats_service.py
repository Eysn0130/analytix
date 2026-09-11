from __future__ import annotations

import time
from pathlib import Path
from typing import Optional, Sequence

from app.repositories.stats_repository import StatsRepository


class StatsService:
    """Stats domain service for v2 analysis stats APIs."""

    def __init__(self, repository: StatsRepository) -> None:
        self._repository = repository

    @property
    def storage(self):
        return self._repository.storage

    @staticmethod
    def _is_transient_case_db_conflict(exc: Exception) -> bool:
        message = str(exc or "").lower()
        return (
            "unique file handle conflict" in message
            or "already attached by database" in message
            or "same database file with a different configuration" in message
            or "could not set lock on file" in message
            or "conflicting lock is held" in message
            or "transactioncontext error" in message
            or "conflict on update" in message
        )

    def query_date_range(self, *, case_id: str) -> dict:
        return self._repository.query_date_range(case_id=case_id)

    def get_funds_status(self, *, case_id: str) -> str:
        return self._repository.get_funds_status(case_id=case_id)

    def query_v2_case_overview(self, *, case_id: str) -> dict:
        last_error: Exception | None = None
        for attempt in range(3):
            try:
                return self._repository.query_v2_case_overview(case_id=case_id)
            except Exception as exc:
                if not self._is_transient_case_db_conflict(exc) or attempt >= 2:
                    raise
                last_error = exc
                time.sleep(0.05 * (attempt + 1))
        if last_error is not None:
            raise last_error
        return self._repository.query_v2_case_overview(case_id=case_id)

    def append_skill_runtime_query_log(
        self,
        *,
        case_id: str,
        tool_name: str,
        params: dict,
        summary: dict,
        row_count: int,
        duration_ms: int,
    ) -> str:
        return self._repository.append_skill_runtime_query_log(
            case_id=case_id,
            tool_name=tool_name,
            params=params,
            summary=summary,
            row_count=row_count,
            duration_ms=duration_ms,
        )

    def query_v2_tree(self, *, case_id: str, tab: str) -> dict:
        last_error: Exception | None = None
        for attempt in range(3):
            try:
                return self._repository.query_v2_tree(case_id=case_id, tab=tab)
            except Exception as exc:
                if not self._is_transient_case_db_conflict(exc) or attempt >= 2:
                    raise
                last_error = exc
                time.sleep(0.05 * (attempt + 1))
        if last_error is not None:
            raise last_error
        return self._repository.query_v2_tree(case_id=case_id, tab=tab)

    def query_v2_account_txn_rows(
        self,
        *,
        case_id: str,
        account_key: str,
        date_start: str,
        date_end: str,
        start_time: str,
        end_time: str,
        sort_dir: str,
        limit: int,
        cursor: object | None = None,
    ) -> dict:
        return self._repository.query_v2_account_txn_rows(
            case_id=case_id,
            account_key=account_key,
            date_start=date_start,
            date_end=date_end,
            start_time=start_time,
            end_time=end_time,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
        )

    def query_v2_rows(
        self,
        *,
        case_id: str,
        mode: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        search_text: str = "",
        row_sort_col: str = "",
        row_sort_dir: str = "desc",
        row_offset: int = 0,
        row_limit: int = 0,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_rows(
            case_id=case_id,
            mode=mode,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            search_text=search_text,
            row_sort_col=row_sort_col,
            row_sort_dir=row_sort_dir,
            row_offset=row_offset,
            row_limit=row_limit,
            row_format=row_format,
            fields=fields,
        )

    def query_v2_rows_direct(
        self,
        *,
        case_id: str,
        mode: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        search_text: str = "",
        row_sort_col: str = "",
        row_sort_dir: str = "desc",
        row_offset: int = 0,
        row_limit: int = 0,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        return self.query_v2_rows(
            case_id=case_id,
            mode=mode,
            selected_keys=selected_keys,
            date_start=date_start,
            date_end=date_end,
            search_text=search_text,
            row_sort_col=row_sort_col,
            row_sort_dir=row_sort_dir,
            row_offset=row_offset,
            row_limit=row_limit,
            row_format=row_format,
            fields=fields,
        )

    def query_v2_rows_to_file(
        self,
        *,
        case_id: str,
        mode: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        search_text: str = "",
        row_sort_col: str = "",
        row_sort_dir: str = "desc",
        row_offset: int = 0,
        row_limit: int = 0,
        output_json: Path,
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_rows_to_file(
            case_id=case_id,
            mode=mode,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            search_text=search_text,
            row_sort_col=row_sort_col,
            row_sort_dir=row_sort_dir,
            row_offset=row_offset,
            row_limit=row_limit,
            row_format=row_format,
            fields=fields,
            output_json=output_json,
        )

    def query_v2_txn_rows(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict],
        key_values: Sequence[str] = (),
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_txn_rows(
            case_id=case_id,
            key_type=key_type,
            key_value=key_value,
            key_values=key_values,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            row_format=row_format,
            fields=fields,
        )

    def query_v2_txn_rows_to_file(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict],
        output_json: Path,
        key_values: Sequence[str] = (),
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_txn_rows_to_file(
            case_id=case_id,
            key_type=key_type,
            key_value=key_value,
            key_values=key_values,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            row_format=row_format,
            fields=fields,
            output_json=output_json,
        )

    def query_v2_txn_rows_direct(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        direction: str,
        sort_col: str,
        sort_dir: str,
        limit: int,
        cursor: Optional[dict],
        key_values: Sequence[str] = (),
        row_format: str = "object",
        fields: Sequence[str] = (),
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_txn_rows(
            case_id=case_id,
            key_type=key_type,
            key_value=key_value,
            key_values=key_values,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            direction=direction,
            sort_col=sort_col,
            sort_dir=sort_dir,
            limit=limit,
            cursor=cursor,
            row_format=row_format,
            fields=fields,
        )

    def query_v2_chart_dashboard(
        self,
        *,
        case_id: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        metric_mode: str,
        direction_mode: str,
        granularity: str,
        success_filter: str,
        cash_filter: str,
        chart_filters: Sequence[dict],
        panel_views: Sequence[dict],
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_chart_dashboard(
            case_id=case_id,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            metric_mode=metric_mode,
            direction_mode=direction_mode,
            granularity=granularity,
            success_filter=success_filter,
            cash_filter=cash_filter,
            chart_filters=chart_filters,
            panel_views=panel_views,
        )

    def query_v2_chart_detail_rows(
        self,
        *,
        case_id: str,
        selected_keys: Sequence[str],
        date_start: str,
        date_end: str,
        metric_mode: str,
        direction_mode: str,
        granularity: str,
        success_filter: str,
        cash_filter: str,
        chart_filters: Sequence[dict],
        panel_views: Sequence[dict],
        sort_col: str,
        sort_dir: str,
        page: int,
        limit: int,
        visible_columns: Sequence[str],
    ) -> dict:
        norm_keys = self._normalize_keys(selected_keys)
        return self._repository.query_v2_chart_detail_rows(
            case_id=case_id,
            selected_keys=norm_keys,
            date_start=date_start,
            date_end=date_end,
            metric_mode=metric_mode,
            direction_mode=direction_mode,
            granularity=granularity,
            success_filter=success_filter,
            cash_filter=cash_filter,
            chart_filters=chart_filters,
            panel_views=panel_views,
            sort_col=sort_col,
            sort_dir=sort_dir,
            page=page,
            limit=limit,
            visible_columns=visible_columns,
        )

    def get_account_delete_info(self, *, case_id: str, account_keys: Sequence[str]) -> dict:
        return self._repository.get_account_delete_info(case_id=case_id, account_keys=account_keys)

    def update_account_info(
        self,
        *,
        case_id: str,
        account_keys: Sequence[str],
        account_open_name: str,
        opener_id_no: str,
    ) -> dict:
        return self._repository.update_account_info(
            case_id=case_id,
            account_keys=account_keys,
            account_open_name=account_open_name,
            opener_id_no=opener_id_no,
        )

    def delete_accounts(self, *, case_id: str, account_keys: Sequence[str]) -> dict:
        return self._repository.delete_accounts(case_id=case_id, account_keys=account_keys)

    def set_doc_pending(
        self,
        *,
        case_id: str,
        key_type: str,
        key_value: str,
        pending: bool,
    ) -> dict:
        return self._repository.set_doc_pending(
            case_id=case_id,
            key_type=key_type,
            key_value=key_value,
            pending=pending,
        )

    def get_v2_log_config(self) -> dict:
        return self._repository.get_v2_log_config()

    def log_v2_debug(self, *, payload: Optional[dict]) -> dict:
        return self._repository.log_v2_debug(payload=payload or {})

    def get_v2_table_widths(self, *, tab: str, mode: str) -> dict:
        return self._repository.get_v2_table_widths(tab=tab, mode=mode)

    def set_v2_table_widths(self, *, tab: str, mode: str, version: str, widths: object) -> dict:
        return self._repository.set_v2_table_widths(
            tab=tab,
            mode=mode,
            version=version,
            widths=widths,
        )

    def _normalize_keys(self, values: Sequence[str]) -> list[str]:
        dedup: list[str] = []
        seen: set[str] = set()
        for raw in values or []:
            key = str(raw or "").strip()
            if not key or key in seen:
                continue
            dedup.append(key)
            seen.add(key)
        return dedup
