from __future__ import annotations

import inspect
from pathlib import Path

import pytest

import app.repositories.stats_repository as stats_repository_module
from app.repositories.stats_repository import StatsRepository


class _ReadyAggregate:
    def ensure_materialized(self, case_id: str) -> bool:
        return case_id == "case-a"


class _Storage:
    def __init__(self, db_path: Path) -> None:
        self._db_path = db_path

    def case_db(self, case_id: str) -> Path:
        assert case_id == "case-a"
        return self._db_path


def test_stats_fact_cache_interfaces_are_physically_absent() -> None:
    source = inspect.getsource(StatsRepository)
    for name in (
        "_build_v2_tree_cache_key",
        "_build_date_range_cache_key",
        "_build_v2_chart_dashboard_cache_key",
        "_tree_cache",
        "_date_range_cache",
        "_chart_dashboard_cache",
    ):
        assert name not in source, name


def test_date_range_revalidates_source_instead_of_reusing_case_cache(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = object.__new__(StatsRepository)
    repository._daily_agg = _ReadyAggregate()
    repository._storage = _Storage(tmp_path / "case.duckdb")
    calls = 0

    def query_date_range(*, case_id: str, db_path: Path) -> dict:
        nonlocal calls
        calls += 1
        assert case_id == "case-a"
        assert db_path == tmp_path / "case.duckdb"
        return {"date_range": {"min": f"2026-01-0{calls}", "max": "2026-01-31"}}

    monkeypatch.setattr(stats_repository_module, "query_stats_date_range", query_date_range)

    first = repository.query_date_range(case_id="case-a")
    second = repository.query_date_range(case_id="case-a")

    assert first["min"] == "2026-01-01"
    assert second["min"] == "2026-01-02"
    assert calls == 2
