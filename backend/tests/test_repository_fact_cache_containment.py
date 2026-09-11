from __future__ import annotations

import os
from pathlib import Path
from types import SimpleNamespace

import pytest

import app.repositories.import_repository as import_repository_module
import app.repositories.analysis_repository as analysis_repository_module
import app.repositories.txn_daily_aggregate as txn_daily_aggregate_module
from app.core.db_engine import DuckDBEngine
from app.core.import_accelerator import PreparedCsv, PreparedCsvWithProfile
from app.repositories.analysis_repository import AnalysisRepository
from app.repositories.flow_repository import (
    FlowJobResultManifestError,
    FlowRepository,
    FlowResultSnapshotError,
)
from app.repositories.import_repository import ImportRepository
from app.repositories.txn_daily_aggregate import TxnDailyAggregateStore
from app.utils.fs import remove_path_no_follow_under


_LEGACY_FACT_CACHE_TABLES = (
    "analysis_skill_cache",
    "analysis_scope_cache",
    "analysis_key_node_features",
    "analysis_signal_features",
    "analysis_refresh_state",
)


def _create_legacy_fact_cache_tables(engine: DuckDBEngine) -> None:
    engine.execute(
        "CREATE TABLE analysis_skill_cache(cache_key TEXT, case_id TEXT, skill_id TEXT, "
        "source_revision BIGINT, updated_at TEXT)"
    )
    engine.execute(
        "CREATE TABLE analysis_scope_cache(scope_cache_id TEXT, case_id TEXT, scope_kind TEXT, "
        "scope_signature TEXT, source_revision BIGINT, updated_at TEXT)"
    )
    engine.execute(
        "CREATE TABLE analysis_key_node_features(feature_id TEXT, case_id TEXT, "
        "scope_signature TEXT, source_revision BIGINT)"
    )
    engine.execute(
        "CREATE TABLE analysis_signal_features(signal_id TEXT, case_id TEXT, "
        "scope_signature TEXT, source_revision BIGINT)"
    )
    engine.execute(
        "CREATE TABLE analysis_refresh_state(refresh_key TEXT, case_id TEXT, state_type TEXT, "
        "source_signature TEXT, stats_json TEXT)"
    )


def _legacy_fact_cache_table_count(engine: DuckDBEngine) -> int:
    placeholders = ",".join(["?"] * len(_LEGACY_FACT_CACHE_TABLES))
    rows = engine.query(
        f"SELECT COUNT(1) FROM information_schema.tables WHERE table_name IN ({placeholders})",
        _LEGACY_FACT_CACHE_TABLES,
    )
    return int(rows[0][0]) if rows else -1


def test_flow_repository_has_no_process_fact_cache_hooks(tmp_path: Path) -> None:
    repository = object.__new__(FlowRepository)
    assert not hasattr(repository, "_result_snapshot_cache")
    assert not hasattr(repository, "_result_snapshot_cache_order")
    assert not hasattr(repository, "_read_result_snapshot_cache")
    assert not hasattr(repository, "_remember_result_snapshot_cache")


@pytest.mark.parametrize("case_id", ("", "active", " active "))
def test_flow_fact_storage_rejects_empty_or_active_case_alias(tmp_path: Path, case_id: str) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(app_dir=tmp_path)

    with pytest.raises(FlowResultSnapshotError, match="flow_result_snapshot_case_invalid"):
        repository._result_snapshots_dir(case_id)
    with pytest.raises(FlowJobResultManifestError, match="flow_job_result_case_invalid"):
        repository._job_result_manifests_dir(case_id)


def test_flow_fact_storage_digest_separates_sanitizer_collisions(tmp_path: Path) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(app_dir=tmp_path)

    slash = repository._result_snapshots_dir("case/a")
    colon = repository._result_snapshots_dir("case:a")

    assert slash.parent == colon.parent
    assert slash.name.startswith("case-") and len(slash.name) == len("case-") + 64
    assert colon.name.startswith("case-") and len(colon.name) == len("case-") + 64
    assert slash != colon


def test_flow_repository_purges_pre_digest_fact_aliases(tmp_path: Path) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(
        app_dir=tmp_path,
        list_cases=lambda *, include_deleted: [SimpleNamespace(case_id="case/a")],
    )
    legacy = tmp_path / "flow_result_snapshots" / "case_a"
    legacy.mkdir(parents=True)
    (legacy / "facts.json").write_text('{"amount":99}', encoding="utf-8")

    repository._purge_legacy_result_snapshot_aliases()

    assert not legacy.exists()
    assert not repository._result_snapshots_dir("case/a").exists()


def test_flow_repository_purges_literal_active_and_empty_fallback_fact_aliases(tmp_path: Path) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(
        app_dir=tmp_path,
        list_cases=lambda *, include_deleted: [],
    )
    for root_name in ("flow_result_snapshots", "flow_job_results"):
        for alias in ("active", "case"):
            path = tmp_path / root_name / alias
            path.mkdir(parents=True, exist_ok=True)
            (path / "facts.json").write_text('{"amount":99}', encoding="utf-8")
    for alias in ("active", "case"):
        path = tmp_path / "flow_view_metadata_v2" / f"{alias}.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('{"amount":99}', encoding="utf-8")

    repository._purge_legacy_result_snapshot_aliases()

    for root_name in ("flow_result_snapshots", "flow_job_results"):
        assert not (tmp_path / root_name / "active").exists()
        assert not (tmp_path / root_name / "case").exists()
    assert not (tmp_path / "flow_view_metadata_v2" / "active.json").exists()
    assert not (tmp_path / "flow_view_metadata_v2" / "case.json").exists()


def test_flow_fact_alias_purge_failure_blocks_repository_readiness(tmp_path: Path, monkeypatch) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(
        app_dir=tmp_path,
        list_cases=lambda *, include_deleted: [],
    )

    def fail(_root, _path) -> None:
        raise OSError("simulated purge failure")

    monkeypatch.setattr("app.repositories.flow_repository.remove_path_no_follow_under", fail)
    with pytest.raises(FlowResultSnapshotError, match="legacy_result_snapshot_purge_failed"):
        repository._purge_legacy_result_snapshot_aliases()


def test_legacy_stats_focus_fact_cache_purge_failure_blocks_readiness(tmp_path: Path, monkeypatch) -> None:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(app_dir=tmp_path)

    def fail(_root, _path) -> None:
        raise OSError("simulated purge failure")

    monkeypatch.setattr("app.repositories.flow_repository.remove_path_no_follow_under", fail)
    with pytest.raises(FlowResultSnapshotError, match="legacy_stats_focus_fact_cache_purge_failed"):
        repository._purge_all_legacy_stats_focus_fact_caches()


def test_fact_cache_purge_rejects_intermediate_symlink_without_touching_external_data(tmp_path: Path) -> None:
    app_dir = tmp_path / "app"
    app_dir.mkdir()
    external = tmp_path / "external"
    target = external / "case-a"
    target.mkdir(parents=True)
    sentinel = target / "facts.json"
    sentinel.write_text('{"amount":99}', encoding="utf-8")
    (app_dir / "flow_result_snapshots").symlink_to(external, target_is_directory=True)

    with pytest.raises(OSError, match="private_artifact_remove_invalid"):
        remove_path_no_follow_under(app_dir, app_dir / "flow_result_snapshots" / "case-a")

    assert sentinel.read_text(encoding="utf-8") == '{"amount":99}'


def test_fact_cache_purge_unlinks_leaf_symlink_without_following_it(tmp_path: Path) -> None:
    app_dir = tmp_path / "app"
    app_dir.mkdir()
    external = tmp_path / "external"
    external.mkdir()
    sentinel = external / "facts.json"
    sentinel.write_text('{"amount":99}', encoding="utf-8")
    link = app_dir / "legacy-facts"
    link.symlink_to(external, target_is_directory=True)

    remove_path_no_follow_under(app_dir, link)

    assert not link.exists()
    assert sentinel.read_text(encoding="utf-8") == '{"amount":99}'


def test_trace_and_scope_fact_cache_compatibility_hooks_are_absent() -> None:
    for name in (
        "_trace_find_cached_run",
        "_resolve_scope_fact_pack_engine",
        "_scope_fact_pack_cached_result",
        "_record_scope_fact_pack_result",
    ):
        assert not hasattr(AnalysisRepository, name), name


def test_repository_legacy_fact_caches_are_purged_and_inaccessible(tmp_path: Path) -> None:
    repository = object.__new__(AnalysisRepository)
    engine = DuckDBEngine(tmp_path / "case.duckdb")
    try:
        _create_legacy_fact_cache_tables(engine)
        engine.execute(
            "INSERT INTO analysis_skill_cache(cache_key, case_id, skill_id, source_revision, updated_at) "
            "VALUES ('skill-key', 'case-a', 'skill-a', 1, '2099-01-01')"
        )
        engine.execute(
            "INSERT INTO analysis_scope_cache(scope_cache_id, case_id, scope_kind, scope_signature, "
            "source_revision, updated_at) VALUES ('scope-key', 'case-a', 'account_stats', 'scope-a', 1, '2099-01-01')"
        )
        engine.execute(
            "INSERT INTO analysis_key_node_features(feature_id, case_id, scope_signature, source_revision) "
            "VALUES ('node-key', 'case-a', 'scope-a', 1)"
        )
        engine.execute(
            "INSERT INTO analysis_signal_features(signal_id, case_id, scope_signature, source_revision) "
            "VALUES ('signal-key', 'case-a', 'scope-a', 1)"
        )
        engine.execute(
            "INSERT INTO analysis_refresh_state(refresh_key, case_id, state_type, source_signature, stats_json) "
            "VALUES ('case-a:entity_graph', 'case-a', 'entity_graph', 'weak-signature', '{\"amount\":99}')"
        )

        repository.ensure_analysis_schema(engine)

        assert _legacy_fact_cache_table_count(engine) == 0
        for table_name in (
            "analysis_scope_cache",
            "analysis_key_node_features",
            "analysis_signal_features",
        ):
            assert analysis_repository_module._is_allowed_workbench_table(table_name) is False
    finally:
        engine.close()


def test_cache_janitor_never_retains_fresh_legacy_facts(tmp_path: Path) -> None:
    repository = object.__new__(AnalysisRepository)
    engine = DuckDBEngine(tmp_path / "case.duckdb")
    try:
        repository.ensure_analysis_schema(engine)
        _create_legacy_fact_cache_tables(engine)
        engine.execute(
            "INSERT INTO analysis_skill_cache(cache_key, case_id, skill_id, source_revision, updated_at) "
            "VALUES ('fresh-skill', 'case-a', 'skill-a', 999, '2099-01-01')"
        )
        engine.execute(
            "INSERT INTO analysis_scope_cache(scope_cache_id, case_id, scope_kind, scope_signature, "
            "source_revision, updated_at) VALUES ('fresh-scope', 'case-a', 'account_stats', 'scope-a', 999, '2099-01-01')"
        )

        summary = repository._cleanup_cache_retention(engine, case_id="case-a")

        assert summary["scanned_count"] == 0
        assert summary["stale_count"] == 0
        assert summary["deleted_count"] == 0
        assert summary["skipped_count"] == 1
        assert summary["skip_reasons"] == [{"reason": "legacy_fact_cache_tables_absent", "count": 1}]
        assert _legacy_fact_cache_table_count(engine) == 0
    finally:
        engine.close()


def test_repository_fact_cache_helpers_are_physically_absent() -> None:
    for name in (
        "_refresh_state_row",
        "_upsert_refresh_state",
        "_set_scope_feature_rows",
        "_load_scope_feature_rows",
    ):
        assert not hasattr(AnalysisRepository, name), name


def test_txn_materialization_never_skips_live_source_revalidation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class FakeEngine:
        path = Path("/tmp/analytix-test-case.duckdb")

        def close(self) -> None:
            return None

    class FakeStorage:
        def __init__(self) -> None:
            self.open_calls = 0

        def open_case_engine(self, case_id: str, *, read_only: bool):
            assert case_id == "case-a"
            assert read_only is True
            self.open_calls += 1
            return FakeEngine()

    storage = FakeStorage()
    store = object.__new__(TxnDailyAggregateStore)
    store._storage = storage
    validations = 0

    def is_current(*_args, **_kwargs) -> bool:
        nonlocal validations
        validations += 1
        return True

    monkeypatch.setattr(store, "_table_exists", lambda _engine, _table: True)
    monkeypatch.setattr(store, "_compute_source_snapshot", lambda _engine, _case_id: (2, "", 2))
    monkeypatch.setattr(store, "_is_current", is_current)
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "try_materialize_txn_daily",
        lambda **_kwargs: {"ok": True},
    )
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "_shutdown_stats_worker_before_duckdb_write",
        lambda: None,
    )
    monkeypatch.setattr(
        txn_daily_aggregate_module,
        "get_stats_flow_source_revision",
        lambda _engine: 7,
    )

    assert store.ensure_materialized("case-a") is True
    assert store.ensure_materialized("case-a") is True
    assert storage.open_calls == 4
    assert validations == 2


def test_csv_row_count_rescans_when_content_changes_with_same_size_and_mtime(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = object.__new__(ImportRepository)
    path = tmp_path / "rows.csv"
    path.write_text("v\n1\n2\n", encoding="utf-8")
    original_stat = path.stat()
    calls = 0

    def prepare_csv(source: Path, *, limit: int) -> PreparedCsv:
        nonlocal calls
        calls += 1
        assert limit == 0
        rows_total = max(0, source.read_text(encoding="utf-8").count("\n") - 1)
        return PreparedCsv(
            encoding="utf-8",
            rows_total=rows_total,
            columns_total=1,
            header_preview=["v"],
            sample_rows=[],
        )

    monkeypatch.setattr(import_repository_module, "prepare_csv", prepare_csv)
    assert repository._count_rows_fast(path) == 2

    path.write_text("v\n1,2\n", encoding="utf-8")
    os.utime(path, ns=(original_stat.st_atime_ns, original_stat.st_mtime_ns))
    assert path.stat().st_size == original_stat.st_size
    assert path.stat().st_mtime_ns == original_stat.st_mtime_ns
    assert repository._count_rows_fast(path) == 1
    assert calls == 2


def test_csv_profile_rescans_when_content_changes_with_same_size_and_mtime(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = object.__new__(ImportRepository)
    path = tmp_path / "profile.csv"
    path.write_text("a,b\n1,2\n", encoding="utf-8")
    original_stat = path.stat()
    calls = 0

    def prepare_with_profile(
        source: Path,
        *,
        limit: int,
        encoding: str | None,
    ) -> PreparedCsvWithProfile:
        nonlocal calls
        calls += 1
        headers = source.read_text(encoding="utf-8").splitlines()[0].split(",")
        return PreparedCsvWithProfile(
            prepared=PreparedCsv(
                encoding=encoding or "utf-8",
                rows_total=1,
                columns_total=len(headers),
                header_preview=headers,
                sample_rows=[],
            ),
            profiles=None,
        )

    monkeypatch.setattr(
        import_repository_module,
        "prepare_csv_with_profile",
        prepare_with_profile,
    )
    first = repository._prepare_csv_with_profile_for_preview(
        path,
        limit=10,
        encoding="utf-8",
        profiler=None,
        phase_name="profile",
    )

    path.write_text("x,y\n1,2\n", encoding="utf-8")
    os.utime(path, ns=(original_stat.st_atime_ns, original_stat.st_mtime_ns))
    assert path.stat().st_size == original_stat.st_size
    assert path.stat().st_mtime_ns == original_stat.st_mtime_ns
    second = repository._prepare_csv_with_profile_for_preview(
        path,
        limit=10,
        encoding="utf-8",
        profiler=None,
        phase_name="profile",
    )

    assert first.value.prepared.header_preview == ["a", "b"]
    assert second.value.prepared.header_preview == ["x", "y"]
    assert calls == 2
