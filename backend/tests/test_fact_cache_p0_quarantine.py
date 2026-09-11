from __future__ import annotations

import importlib.util

from app.domain.analysis_service import AnalysisService
from app.domain.flow_service import FlowService
from app.repositories.analysis_repository import AnalysisRepository


_CASE_ID = "case-cache-quarantine"
_ACCOUNT = "6222020202020202020"
_PATH = "/private/case.csv"


def _poison(*_args, **_kwargs):
    raise AssertionError("fact cache quarantine crossed a data, persistence, or engine boundary")


def test_persisted_skill_and_scope_cache_interfaces_are_physically_absent() -> None:
    for name in (
        "get_case_profile_cache_token",
        "get_skill_cache_entry",
        "get_skill_cache_entries",
        "set_skill_cache_entry",
        "_scope_signature",
        "_get_scope_cache_payload",
        "_set_scope_cache_payload",
        "list_scope_cache_entries",
        "_trace_find_cached_run",
        "_set_scope_feature_rows",
        "_load_scope_feature_rows",
    ):
        assert not hasattr(AnalysisRepository, name), name


def test_analysis_service_never_queues_or_delegates_fact_cache_work() -> None:
    for name in (
        "get_case_profile_cache_token",
        "get_skill_cache_entry",
        "get_skill_cache_entries",
        "set_skill_cache_entry",
        "set_skill_cache_entry_async",
        "list_scope_cache_entries",
    ):
        assert not hasattr(AnalysisService, name), name


def test_batch_transaction_and_latest_fact_alias_module_is_removed() -> None:
    assert importlib.util.find_spec("app.domain.skill_batching") is None


def test_flow_service_never_reads_a_fact_graph_cache() -> None:
    class _Repository:
        get_cached_graph = staticmethod(_poison)

        @staticmethod
        def build_graph(**_kwargs) -> dict:
            return {"nodes": [], "edges": [], "stats": {}}

    service = FlowService(repository=_Repository())  # type: ignore[arg-type]

    assert service.build_graph(
        case_id=_CASE_ID,
        seeds=[_ACCOUNT],
        depth=1,
        direction="both",
        min_amount=0,
    ) == {"nodes": [], "edges": [], "stats": {}}
