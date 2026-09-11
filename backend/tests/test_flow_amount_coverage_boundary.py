from __future__ import annotations

import hashlib
import json
import os
import threading
import time
from datetime import datetime
from pathlib import Path
from uuid import uuid4

import pytest

import app.core.storage as storage_module
import app.repositories.flow_repository as flow_repository_module
from app.core.storage import CaseStorage
from app.domain.flow_service import FlowService
from app.repositories.flow_repository import (
    FlowJobResultManifestError,
    FlowRepository,
    FlowResultSnapshotError,
    _attach_candidate_amount_coverage,
    _build_projected_result,
    _clone_json,
    _result_snapshot_envelope_digest,
    _runtime_graph_to_entities,
)
from app.repositories.txn_daily_aggregate import (
    TxnAmountCoverageIncompleteError,
    TxnAmountCoverageV1,
)
from app.utils.strict_json import dumps_canonical_json
from app.tasks.models import TaskStatus, TaskType
from app.tasks.service import TaskService


class _Storage:
    def __init__(self, root: Path) -> None:
        self.app_dir = root

    def case_db(self, case_id: str) -> Path:
        assert case_id == "case-a"
        return self.app_dir / "case-a.duckdb"

    def get_case(self, case_id: str):
        return _CaseRecord() if case_id == "case-a" else None

    def get_case_lifecycle_generation(self, case_id: str) -> int | None:
        return 1 if case_id == "case-a" else None

    def get_case_binding_state(self, case_id: str) -> dict | None:
        if case_id != "case-a":
            return None
        return {"case_id": "case-a", "deleted_at": "", "lifecycle_generation": 1}


class _CaseRecord:
    case_id = "case-a"


class _StorageWithCases(_Storage):
    def list_cases(self, *, include_deleted: bool = False) -> list[_CaseRecord]:
        assert include_deleted is True
        return [_CaseRecord()]


class _CompleteDailyAggregate:
    def __init__(self, coverage: TxnAmountCoverageV1) -> None:
        self.coverage = coverage

    def require_complete_amount_coverage(self, *, case_id: str, **_kwargs) -> TxnAmountCoverageV1:
        assert case_id == self.coverage.case_id
        return self.coverage

    def ensure_materialized(self, *_args, **_kwargs) -> bool:
        return True


class _IncompleteDailyAggregate:
    def require_complete_amount_coverage(self, **_kwargs) -> TxnAmountCoverageV1:
        raise TxnAmountCoverageIncompleteError()


def _repository(tmp_path: Path, daily_agg=None) -> FlowRepository:
    repository = object.__new__(FlowRepository)
    repository._storage = _Storage(tmp_path)
    repository._daily_agg = daily_agg
    return repository


def _case_storage(tmp_path: Path, registry: dict[str, dict] | None = None) -> CaseStorage:
    storage = object.__new__(CaseStorage)
    storage.app_dir = tmp_path
    storage.cases_dir = tmp_path / "cases"
    storage.cases_dir.mkdir(mode=0o700, exist_ok=True)
    storage.registry_path = tmp_path / "case_registry.json"
    storage._registry = {
        case_id: {"status": "进行中", **meta}
        for case_id, meta in dict(registry or {}).items()
    }
    storage._case_engine_open_locks_guard = threading.RLock()
    storage._case_engine_open_locks = {}
    storage._audit = lambda *_args, **_kwargs: None
    storage.sync_case_project_doc = lambda *_args, **_kwargs: tmp_path / "case.md"
    return storage


def _complete_coverage() -> TxnAmountCoverageV1:
    return TxnAmountCoverageV1(
        case_id="case-a",
        materialization_identity="txn_daily_snapshot:v12:" + "a" * 64,
        total_rows=1,
        amount_valid_rows=1,
        amount_missing_rows=0,
        amount_parse_failed_rows=0,
        direction_covered_rows=1,
    )


def _runtime_graph(*, amount=0.0) -> dict:
    edge = {
        "id": "acct-a==acct-b",
        "source": "acct-a",
        "target": "acct-b",
        "count": 1,
    }
    if amount is not ...:
        edge["amount"] = amount
    return {
        "nodes": [
            {"id": "acct-a", "ntype": "account", "display_id": "acct-a"},
            {"id": "acct-b", "ntype": "account", "display_id": "acct-b"},
        ],
        "edges": [edge],
        "stats": {"total_amount": amount if amount is not ... else None},
    }


def test_runtime_graph_edge_count_is_required_and_never_coerced_to_zero() -> None:
    graph = _runtime_graph(amount=12.5)
    graph["edges"][0].pop("count")

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        _runtime_graph_to_entities(graph["nodes"], graph["edges"])


def test_clone_json_honors_typed_fallback_for_null_or_wrong_type() -> None:
    assert _clone_json(None, {}) == {}
    assert _clone_json([], {}) == {}
    assert _clone_json(None, []) == []
    assert _clone_json({}, []) == []


@pytest.mark.parametrize("missing_field", ("total_amount", "total_count"))
def test_projection_totals_are_required_and_never_coerced_to_zero(monkeypatch, missing_field: str) -> None:
    graph = _runtime_graph(amount=12.5)
    edge_plan = {"edges": graph["edges"], "total_amount": 12.5, "total_count": 1}
    edge_plan.pop(missing_field)
    monkeypatch.setattr(
        flow_repository_module,
        "project_flow_skeleton_clusters",
        lambda **_kwargs: {
            "base_projection_node_ids": [],
            "explicit_entity_ids": [],
            "requested_cluster_materializations": [],
            "cluster_visibility_plan": {},
            "projected_node_plan": {
                "nodes": graph["nodes"],
                "clusters": [],
                "dot_count": 0,
                "entity_count": 2,
                "expanded_cluster_ids": [],
                "expanded_tile_ids": [],
                "expanded_node_ids": [],
                "point_layer_buckets": [],
                "point_layer_total": 0,
            },
            "projected_edge_plan": edge_plan,
        },
    )
    full_result = {
        "nodes": graph["nodes"],
        "edges": graph["edges"],
        "stats": {},
        "runtime_graph": graph,
        "graph_tier": "large",
        "render_hints": None,
    }

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        _build_projected_result(
            full_result=full_result,
            source_snapshot_ref=None,
            request_context={"render_mode": "skeleton"},
        )


def test_stats_focus_v3_cache_is_removed_without_reading_facts(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    legacy_key = "b" * 64
    legacy_path = tmp_path / "flow_stats_focus_cache" / "case-a" / f"{legacy_key}.json"
    legacy_path.parent.mkdir(parents=True, exist_ok=True)
    legacy_path.write_text(
        json.dumps({"version": 1, "cache_key": legacy_key, "snapshot_ref": {"snapshot_id": "c" * 64}}),
        encoding="utf-8",
    )

    repository._load_result_snapshot = lambda *_args, **_kwargs: pytest.fail("legacy cache loaded a fact snapshot")
    repository._purge_all_legacy_stats_focus_fact_caches()

    assert not legacy_path.exists()
    assert not legacy_path.parent.exists()


def test_legacy_fact_cache_purge_never_follows_symlinks(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    external = tmp_path / "external"
    external.mkdir()
    sentinel = external / "keep.json"
    sentinel.write_text('{"amount":999}', encoding="utf-8")
    cache_root = tmp_path / "flow_stats_focus_cache"
    cache_root.mkdir()
    cache_link = cache_root / "case-a"
    cache_link.symlink_to(external, target_is_directory=True)

    repository._purge_all_legacy_stats_focus_fact_caches()

    assert sentinel.read_text(encoding="utf-8") == '{"amount":999}'
    assert not cache_link.exists()
    assert not cache_root.exists()


def test_legacy_v1_result_snapshot_is_rejected_by_every_read_path(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    snapshot_id = "c" * 64
    ref = {"snapshot_id": snapshot_id, "graph_hash": snapshot_id, "graph_storage_mode": "full"}
    path = repository._result_snapshot_path("case-a", snapshot_id)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(
            {
                "version": 1,
                "snapshot_id": snapshot_id,
                "graph_hash": snapshot_id,
                "graph": _runtime_graph(amount=999999.0),
                "result": {"projection": {"source_result_snapshot_ref": {"snapshot_id": "d" * 64}}},
            }
        ),
        encoding="utf-8",
    )

    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=ref)
    assert loaded["nodes"] == []
    assert loaded["edges"] == []
    assert loaded["runtime_graph"] == {"nodes": [], "edges": []}
    assert repository.get_result_snapshot_source_ref(case_id="case-a", snapshot_ref=ref) is None
    assert repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=ref) == ""
    assert not repository._result_snapshot_compute_graph_path("case-a", snapshot_id).exists()


def test_result_without_candidate_amount_coverage_is_not_persisted(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    graph = _runtime_graph(amount=10.0)
    result = {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph}

    assert repository.persist_result_snapshot(case_id="case-a", result=result) is None
    assert repository._persist_result_snapshot(
        "case-a",
        result,
        return_storage_metadata=True,
    ) == (None, {})
    assert not repository._result_snapshots_dir("case-a").exists()


def test_forged_or_cross_case_candidate_coverage_is_not_persisted(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    graph = _runtime_graph(amount=10.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        _complete_coverage(),
    )
    result["stats"]["amount_valid_rows"] = 2

    assert repository.persist_result_snapshot(case_id="case-a", result=result) is None

    honest = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        _complete_coverage(),
    )
    assert repository.persist_result_snapshot(case_id="case-b", result=honest) is None


def test_snapshot_publication_rejects_missing_projection_source_dependency(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=10.0)
    missing_id = "d" * 64
    result = _attach_candidate_amount_coverage(
        {
            "nodes": graph["nodes"],
            "edges": graph["edges"],
            "stats": {},
            "runtime_graph": graph,
            "projection": {
                "source_result_snapshot_ref": {
                    "snapshot_id": missing_id,
                    "graph_hash": missing_id,
                }
            },
        },
        coverage,
    )

    assert repository.persist_result_snapshot(case_id="case-a", result=result) is None
    assert not repository._result_snapshots_dir("case-a").exists()


def test_malformed_projection_source_dependency_fails_closed_without_exception(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=10.0)
    result = _attach_candidate_amount_coverage(
        {
            "nodes": graph["nodes"],
            "edges": graph["edges"],
            "stats": {},
            "runtime_graph": graph,
            "projection": {
                "source_result_snapshot_ref": {
                    "snapshot_id": "d" * 64,
                    "graph_hash": "d" * 64,
                    "node_count": "not-an-integer",
                }
            },
        },
        coverage,
    )

    assert repository.persist_result_snapshot(case_id="case-a", result=result) is None
    assert not repository._result_snapshots_dir("case-a").exists()


def test_build_graph_propagates_incomplete_coverage_before_any_compute_path(tmp_path) -> None:
    repository = _repository(tmp_path, _IncompleteDailyAggregate())

    def poison(*_args, **_kwargs):
        pytest.fail("compute or cache path ran before amount coverage admission")

    repository._build_materialized_graph_result_bundle = poison
    repository._build_runtime_graph = poison

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        repository.build_graph(
            case_id="case-a",
            seeds=["acct-a"],
            depth=1,
            direction="both",
            min_amount=0,
            request_context={},
        )


def test_native_amount_coverage_failure_is_never_downgraded_to_python_fallback(tmp_path, monkeypatch) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))

    def fail_coverage(**_kwargs):
        raise TxnAmountCoverageIncompleteError()

    monkeypatch.setattr(flow_repository_module, "query_flow_focus_graph_to_file", fail_coverage)
    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        repository._build_stats_focus_account_fast_graph_native(
            case_id="case-a",
            depth=1,
            direction="both",
            min_amount=0,
            view_mode="relation",
            seed_ids=["acct-a"],
            left_seed_ids=["acct-a"],
            focus_id_raw="acct-b",
            focus_label="acct-b",
            focus_ids=["acct-b"],
            focus_placeholder_kinds=[],
            request_id="request-a",
            source="stats",
            focus_only=True,
            focus_key_type="account",
            payload_focus_account_strict=True,
            payload_include_missing=False,
            expected_total_amount=None,
            expected_row_count=None,
            date_start=datetime(2026, 1, 1),
            date_end_excl=datetime(2026, 2, 1),
        )


def test_real_zero_round_trips_but_remains_unpublishable(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._build_runtime_graph = lambda **_kwargs: _runtime_graph(amount=0.0)

    result = repository.build_graph(
        case_id="case-a",
        seeds=["acct-a"],
        depth=1,
        direction="both",
        min_amount=0,
        request_context={},
    )
    ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert ref is not None

    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=ref)
    assert loaded["edges"][0]["amount_total"] == 0.0
    assert loaded["runtime_graph"]["edges"][0]["amount"] == 0.0
    assert loaded["stats"]["zero_result_status"] == "unresolved"
    assert loaded["publication_status"] == "blocked"
    assert loaded["fact_answer_allowed"] is False


def test_flow_job_result_manifest_is_private_immutable_case_bound_and_cas_verified(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=0.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert ref is not None
    job_id = str(uuid4())

    manifest = repository.persist_job_result_manifest(
        case_id="case-a",
        job_id=job_id,
        snapshot_ref=ref,
    )

    snapshot_path = repository._result_snapshot_path("case-a", ref["snapshot_id"])
    manifest_path = repository._job_result_manifest_path("case-a", job_id)
    assert os.stat(snapshot_path).st_mode & 0o077 == 0
    assert os.stat(manifest_path).st_mode & 0o077 == 0
    assert manifest["case_id"] == "case-a"
    assert manifest["job_id"] == job_id
    assert repository.has_job_result(case_id="case-a", job_id=job_id) is True
    assert repository.get_job_result_manifest(case_id="case-b", job_id=job_id) is None

    second_graph = _runtime_graph(amount=1.0)
    second_result = _attach_candidate_amount_coverage(
        {
            "nodes": second_graph["nodes"],
            "edges": second_graph["edges"],
            "stats": {},
            "runtime_graph": second_graph,
        },
        coverage,
    )
    second_ref = repository.persist_result_snapshot(case_id="case-a", result=second_result)
    assert second_ref is not None and second_ref["snapshot_id"] != ref["snapshot_id"]
    with pytest.raises(FlowJobResultManifestError, match="flow_job_result_manifest_conflict"):
        repository.persist_job_result_manifest(
            case_id="case-a",
            job_id=job_id,
            snapshot_ref=second_ref,
        )

    snapshot_path.write_text("{}", encoding="utf-8")
    assert repository.get_job_result_manifest(case_id="case-a", job_id=job_id) is None


def test_manifest_rejects_snapshot_with_dangling_projection_dependency(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    source_graph = _runtime_graph(amount=1.0)
    source_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": source_graph["nodes"],
                "edges": source_graph["edges"],
                "stats": {},
                "runtime_graph": source_graph,
            },
            coverage,
        ),
    )
    assert source_ref is not None
    target_graph = _runtime_graph(amount=2.0)
    target_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
                "projection": {"source_result_snapshot_ref": source_ref},
            },
            coverage,
        ),
    )
    assert target_ref is not None
    repository._result_snapshot_path("case-a", source_ref["snapshot_id"]).unlink()

    with pytest.raises(FlowJobResultManifestError, match="flow_job_result_snapshot_invalid"):
        repository.persist_job_result_manifest(
            case_id="case-a",
            job_id=str(uuid4()),
            snapshot_ref=target_ref,
        )


def test_projection_layout_sync_publishes_new_cas_snapshot_without_mutating_source(
    tmp_path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    source_ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert source_ref is not None
    source_path = repository._result_snapshot_path("case-a", source_ref["snapshot_id"])
    source_bytes = source_path.read_bytes()

    monkeypatch.setattr(
        flow_repository_module,
        "project_flow_projection_layout_sync",
        lambda **_kwargs: {
            "layout_by_id": {"acct-a": {"x": 12.5, "y": -3.0}},
            "node_updates": [
                {
                    "index": 0,
                    "id": "acct-a",
                    "x": 12.5,
                    "y": -3.0,
                    "cluster_node": False,
                }
            ],
            "layout_index_summary": {
                "signature": "layout-signature-a",
                "viewport": {"zoom": 1.0},
            },
            "signature": "layout-signature-a",
        },
    )

    summary = repository.update_result_snapshot_projection_layout(
        case_id="case-a",
        snapshot_ref=source_ref,
        layout_index={"positions": {"acct-a": {"x": 12.5, "y": -3.0}}},
    )

    target_ref = summary["snapshot_ref"]
    assert target_ref["snapshot_id"] != source_ref["snapshot_id"]
    assert source_path.read_bytes() == source_bytes
    assert repository._read_valid_result_snapshot_payload(
        case_id="case-a",
        snapshot_ref=source_ref,
    ) is not None
    assert repository._read_valid_result_snapshot_payload(
        case_id="case-a",
        snapshot_ref=target_ref,
    ) is not None
    assert "x" not in repository.get_result_snapshot(case_id="case-a", snapshot_ref=source_ref)["runtime_graph"]["nodes"][0]
    updated = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert updated["runtime_graph"]["nodes"][0]["x"] == 12.5
    assert updated["runtime_graph"]["nodes"][0]["y"] == -3.0

    stored_snapshot_count = len(repository._iter_result_snapshot_paths("case-a"))
    replay = repository.update_result_snapshot_projection_layout(
        case_id="case-a",
        snapshot_ref=target_ref,
        layout_index={"positions": {"acct-a": {"x": 12.5, "y": -3.0}}},
    )
    assert replay["layout_sync_skipped"] is True
    assert replay["snapshot_ref"]["snapshot_id"] == target_ref["snapshot_id"]
    assert len(repository._iter_result_snapshot_paths("case-a")) == stored_snapshot_count


def test_delta_snapshot_round_trips_only_with_exact_base_and_patch_contract(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None
    target_graph = _runtime_graph(amount=1.0)
    target_graph["nodes"][0]["x"] = 42.5
    target_ref = repository.persist_result_snapshot(
        case_id="case-a",
        base_snapshot_ref=base_ref,
        result=_attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
            },
            coverage,
        ),
    )

    assert target_ref is not None
    assert target_ref["graph_storage_mode"] == "delta"
    validated = repository._read_valid_result_snapshot_payload(case_id="case-a", snapshot_ref=target_ref)
    assert validated is not None
    _, payload = validated
    assert set(payload["graph_base_snapshot_ref"]) == {"snapshot_id", "graph_hash"}
    assert payload["graph_storage_chain_depth"] == 1
    assert payload["graph_patch"]["summary"]["target_nodes"] == payload["node_count"] == 2
    assert payload["graph_patch"]["summary"]["target_edges"] == payload["edge_count"] == 1
    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert loaded["runtime_graph"]["nodes"][0]["x"] == 42.5


def test_delta_publication_and_gc_share_one_case_lease(tmp_path, monkeypatch) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))

    def persist_graph(graph: dict) -> dict:
        ref = repository.persist_result_snapshot(
            case_id="case-a",
            result=_attach_candidate_amount_coverage(
                {
                    "nodes": graph["nodes"],
                    "edges": graph["edges"],
                    "stats": {},
                    "runtime_graph": graph,
                },
                coverage,
            ),
        )
        assert ref is not None
        return ref

    base_graph = _runtime_graph(amount=1.0)
    base_ref = persist_graph(base_graph)
    unrelated_ref = persist_graph(_runtime_graph(amount=2.0))
    target_graph = _runtime_graph(amount=1.0)
    target_graph["nodes"][0]["x"] = 42.5

    original_create = flow_repository_module.atomic_create_private_text
    publisher_entered = threading.Event()
    release_publisher = threading.Event()
    publisher_done = threading.Event()
    gc_done = threading.Event()
    published: list[dict | None] = []
    failures: list[BaseException] = []
    existing_names = {
        f'{base_ref["snapshot_id"]}.json',
        f'{unrelated_ref["snapshot_id"]}.json',
    }

    def blocking_create(path: Path, content: str) -> bool:
        if path.parent == repository._result_snapshots_dir("case-a") and path.name not in existing_names:
            publisher_entered.set()
            if not release_publisher.wait(timeout=5):
                raise RuntimeError("test_delta_publication_release_timeout")
        return original_create(path, content)

    monkeypatch.setattr(flow_repository_module, "atomic_create_private_text", blocking_create)

    def publish_delta() -> None:
        try:
            published.append(
                repository.persist_result_snapshot(
                    case_id="case-a",
                    base_snapshot_ref=base_ref,
                    result=_attach_candidate_amount_coverage(
                        {
                            "nodes": target_graph["nodes"],
                            "edges": target_graph["edges"],
                            "stats": {},
                            "runtime_graph": target_graph,
                        },
                        coverage,
                    ),
                )
            )
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            publisher_done.set()

    def collect_garbage() -> None:
        try:
            repository._gc_case_result_snapshots(
                "case-a",
                soft_limit=1,
                keep_latest=1,
                ttl_seconds=0,
            )
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            gc_done.set()

    publisher = threading.Thread(target=publish_delta, daemon=True)
    publisher.start()
    assert publisher_entered.wait(timeout=5)
    collector = threading.Thread(target=collect_garbage, daemon=True)
    collector.start()
    assert not gc_done.wait(timeout=0.1)
    release_publisher.set()
    assert publisher_done.wait(timeout=5)
    assert gc_done.wait(timeout=5)
    publisher.join(timeout=1)
    collector.join(timeout=1)

    assert failures == []
    assert published and published[0] is not None
    target_ref = published[0]
    assert target_ref["graph_storage_mode"] == "delta"
    assert repository._result_snapshot_path("case-a", base_ref["snapshot_id"]).exists()
    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert loaded["runtime_graph"]["nodes"][0]["x"] == 42.5


def test_dangling_delta_blocks_manifest_metrics_and_gc(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None
    target_graph = _runtime_graph(amount=1.0)
    target_graph["nodes"][0]["x"] = 42.5
    target_ref = repository.persist_result_snapshot(
        case_id="case-a",
        base_snapshot_ref=base_ref,
        result=_attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
            },
            coverage,
        ),
    )
    assert target_ref is not None and target_ref["graph_storage_mode"] == "delta"
    job_id = str(uuid4())
    repository.persist_job_result_manifest(case_id="case-a", job_id=job_id, snapshot_ref=target_ref)
    repository._result_snapshot_path("case-a", base_ref["snapshot_id"]).unlink()

    assert repository.get_job_result_manifest(case_id="case-a", job_id=job_id) is None
    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")
    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=100,
        keep_latest=100,
        ttl_seconds=3600,
    )

    assert metrics["inventory_status"] == "unknown"
    assert summary["inventory_status"] == "unknown"
    assert summary["deleted_count"] == 0
    assert repository._result_snapshot_path("case-a", target_ref["snapshot_id"]).exists()


def test_case_purge_lease_rejects_late_snapshot_publication(tmp_path, monkeypatch) -> None:
    coverage = _complete_coverage()
    storage = object.__new__(CaseStorage)
    storage.app_dir = tmp_path
    storage.cases_dir = tmp_path / "cases"
    storage.cases_dir.mkdir(mode=0o700)
    storage.registry_path = tmp_path / "case_registry.json"
    storage._registry = {
        "case-a": {
            "name": "case-a",
            "created_at": "2026-07-20T00:00:00+00:00",
            "updated_at": "2026-07-20T00:00:00+00:00",
            "status": "进行中",
            "deleted_at": "",
            "lifecycle_generation": 1,
        }
    }
    storage._save_registry()
    storage._case_engine_open_locks_guard = threading.RLock()
    storage._case_engine_open_locks = {}
    storage._audit = lambda *_args, **_kwargs: None
    (storage.cases_dir / "case-a").mkdir(mode=0o700)

    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = storage
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None

    purge_entered = threading.Event()
    release_purge = threading.Event()
    purge_done = threading.Event()
    publisher_done = threading.Event()
    published: list[dict | None] = []
    failures: list[BaseException] = []
    original_remove = storage_module.remove_path_no_follow_under

    def blocking_remove(root: Path, path: Path) -> None:
        if not purge_entered.is_set():
            purge_entered.set()
            if not release_purge.wait(timeout=5):
                raise RuntimeError("test_case_purge_release_timeout")
        original_remove(root, path)

    monkeypatch.setattr(storage_module, "remove_path_no_follow_under", blocking_remove)

    def purge_case() -> None:
        try:
            storage.purge_case("case-a")
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            purge_done.set()

    def publish_late_delta() -> None:
        target_graph = _runtime_graph(amount=1.0)
        target_graph["nodes"][0]["x"] = 99.0
        try:
            published.append(
                repository.persist_result_snapshot(
                    case_id="case-a",
                    base_snapshot_ref=base_ref,
                    result=_attach_candidate_amount_coverage(
                        {
                            "nodes": target_graph["nodes"],
                            "edges": target_graph["edges"],
                            "stats": {},
                            "runtime_graph": target_graph,
                        },
                        coverage,
                    ),
                )
            )
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            publisher_done.set()

    purge_thread = threading.Thread(target=purge_case, daemon=True)
    purge_thread.start()
    assert purge_entered.wait(timeout=5)
    publisher_thread = threading.Thread(target=publish_late_delta, daemon=True)
    publisher_thread.start()
    assert not publisher_done.wait(timeout=0.1)
    release_purge.set()
    assert purge_done.wait(timeout=5)
    assert publisher_done.wait(timeout=5)
    purge_thread.join(timeout=1)
    publisher_thread.join(timeout=1)

    assert failures == []
    assert published == [None]
    assert storage.get_case("case-a") is None
    assert not repository._result_snapshots_dir("case-a").exists()


def test_case_delete_restore_aba_rejects_late_snapshot_publication(tmp_path, monkeypatch) -> None:
    coverage = _complete_coverage()
    storage = object.__new__(CaseStorage)
    storage.app_dir = tmp_path
    storage.cases_dir = tmp_path / "cases"
    storage.cases_dir.mkdir(mode=0o700)
    storage.registry_path = tmp_path / "case_registry.json"
    storage._registry = {
        "case-a": {
            "name": "case-a",
            "created_at": "2026-07-20T00:00:00+00:00",
            "updated_at": "2026-07-20T00:00:00+00:00",
            "status": "进行中",
            "deleted_at": "",
            "lifecycle_generation": 1,
        }
    }
    storage._save_registry()
    storage._case_engine_open_locks_guard = threading.RLock()
    storage._case_engine_open_locks = {}
    storage._audit = lambda *_args, **_kwargs: None
    (storage.cases_dir / "case-a").mkdir(mode=0o700)

    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = storage
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None

    publisher_entered = threading.Event()
    release_publisher = threading.Event()
    publisher_done = threading.Event()
    published: list[dict | None] = []
    failures: list[BaseException] = []
    original_dumps = flow_repository_module.dumps_canonical_json
    should_block = True

    def blocking_dumps(*args, **kwargs):
        nonlocal should_block
        if should_block:
            should_block = False
            publisher_entered.set()
            if not release_publisher.wait(timeout=5):
                raise RuntimeError("test_case_aba_release_timeout")
        return original_dumps(*args, **kwargs)

    monkeypatch.setattr(flow_repository_module, "dumps_canonical_json", blocking_dumps)

    def publish_late_delta() -> None:
        target_graph = _runtime_graph(amount=1.0)
        target_graph["nodes"][0]["x"] = 101.0
        try:
            published.append(
                repository.persist_result_snapshot(
                    case_id="case-a",
                    base_snapshot_ref=base_ref,
                    result=_attach_candidate_amount_coverage(
                        {
                            "nodes": target_graph["nodes"],
                            "edges": target_graph["edges"],
                            "stats": {},
                            "runtime_graph": target_graph,
                        },
                        coverage,
                    ),
                )
            )
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            publisher_done.set()

    publisher_thread = threading.Thread(target=publish_late_delta, daemon=True)
    publisher_thread.start()
    assert publisher_entered.wait(timeout=5)

    storage.soft_delete_case("case-a")
    storage.restore_case("case-a")
    assert storage.get_case_lifecycle_generation("case-a") == 3

    release_publisher.set()
    assert publisher_done.wait(timeout=5)
    publisher_thread.join(timeout=1)

    assert failures == []
    assert published == [None]
    assert storage.get_case("case-a") is not None
    assert len(repository._iter_result_snapshot_paths("case-a")) == 1
    assert repository._result_snapshot_path("case-a", base_ref["snapshot_id"]).exists()


def test_build_graph_delete_restore_during_query_rejects_result(tmp_path) -> None:
    coverage = _complete_coverage()
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()
    (tmp_path / "cases" / "case-a").mkdir(mode=0o700)
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = storage
    query_entered = threading.Event()
    release_query = threading.Event()
    build_done = threading.Event()
    failures: list[BaseException] = []

    def blocking_query(**_kwargs):
        query_entered.set()
        if not release_query.wait(timeout=5):
            raise RuntimeError("test_build_graph_release_timeout")
        return _runtime_graph(amount=1.0)

    repository._build_runtime_graph = blocking_query

    def build() -> None:
        try:
            repository.build_graph(
                case_id="case-a",
                seeds=["acct-a"],
                depth=1,
                direction="both",
                min_amount=0,
                request_context={},
            )
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            build_done.set()

    build_thread = threading.Thread(target=build, daemon=True)
    build_thread.start()
    assert query_entered.wait(timeout=5)
    storage.soft_delete_case("case-a")
    storage.restore_case("case-a")
    release_query.set()
    assert build_done.wait(timeout=5)
    build_thread.join(timeout=1)

    assert len(failures) == 1
    assert isinstance(failures[0], FlowResultSnapshotError)
    assert str(failures[0]) == "flow_case_lifecycle_binding_stale"
    assert not repository._result_snapshots_dir("case-a").exists()


def test_async_flow_job_stale_lifecycle_generation_fails_without_manifest_or_success_event(tmp_path) -> None:
    coverage = _complete_coverage()
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()
    (tmp_path / "cases" / "case-a").mkdir(mode=0o700)
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = storage
    binding = repository.freeze_case_lifecycle_binding("case-a")
    tasks = TaskService()
    task = tasks.create_task(
        task_type=TaskType.FLOW_BUILD,
        case_id="case-a",
        metadata={
            "request": {
                "case_id": "case-a",
                "case_lifecycle_binding": binding.to_dict(),
                "seeds": ["acct-a"],
                "depth": 1,
                "direction": "both",
                "min_amount": 0,
            },
            "summary": {},
            "result": None,
        },
    )
    storage.soft_delete_case("case-a")
    storage.restore_case("case-a")
    repository._build_runtime_graph = lambda **_kwargs: pytest.fail(
        "stale queued flow job reached database computation"
    )
    service = FlowService(repository=repository, task_service=tasks)
    events: list[str] = []
    service._emit_event = lambda **kwargs: events.append(str(kwargs.get("event") or ""))

    service._run_build_job(task.task_id)

    failed = tasks.get_task(task.task_id)
    assert failed.status == TaskStatus.FAILED
    assert failed.error == "task_failed"
    assert "analysis.flow.build.completed" not in events
    assert "analysis.flow.build.failed" in events
    assert not repository._job_result_manifests_dir("case-a").exists()
    assert not repository._result_snapshots_dir("case-a").exists()


def test_async_flow_job_success_publication_linearizes_before_case_delete_restore(tmp_path) -> None:
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()
    (tmp_path / "cases" / "case-a").mkdir(mode=0o700)
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    repository._storage = storage
    binding = repository.freeze_case_lifecycle_binding("case-a")
    tasks = TaskService()
    task = tasks.create_task(
        task_type=TaskType.FLOW_BUILD,
        case_id="case-a",
        metadata={
            "request": {
                "case_id": "case-a",
                "case_lifecycle_binding": binding.to_dict(),
                "seeds": ["acct-a"],
                "depth": 1,
                "direction": "both",
                "min_amount": 0,
            },
            "summary": {},
            "result": None,
        },
    )
    service = FlowService(repository=repository, task_service=tasks)
    service.build_graph = lambda **_kwargs: {
        "nodes": [],
        "edges": [],
        "stats": {},
        "runtime_graph": {"nodes": [], "edges": []},
        "publication_status": "blocked",
        "fact_answer_allowed": False,
        "graph_tier": "small",
        "render_hints": {},
        "projection": {},
    }
    snapshot_ref = {
        "snapshot_id": "a" * 64,
        "graph_hash": "a" * 64,
        "graph_storage_mode": "full",
    }
    repository.persist_result_snapshot = lambda **_kwargs: snapshot_ref
    repository.persist_job_result_manifest = lambda **_kwargs: snapshot_ref

    success_transition_entered = threading.Event()
    release_success_transition = threading.Event()
    lifecycle_attempted = threading.Event()
    lifecycle_finished = threading.Event()
    order: list[str] = []
    original_safe_transition = service._safe_transition

    def blocking_success_transition(job_id: str, **kwargs):
        if kwargs.get("to_status") == TaskStatus.SUCCEEDED:
            success_transition_entered.set()
            assert release_success_transition.wait(timeout=5)
        updated = original_safe_transition(job_id, **kwargs)
        if kwargs.get("to_status") == TaskStatus.SUCCEEDED and updated is not None:
            order.append("succeeded")
        return updated

    service._safe_transition = blocking_success_transition

    def record_event(**kwargs) -> None:
        if kwargs.get("event") == "analysis.flow.build.completed":
            order.append("completed")

    service._emit_event = record_event

    worker = threading.Thread(target=service._run_build_job, args=(task.task_id,), daemon=True)
    worker.start()
    assert success_transition_entered.wait(timeout=5)

    def change_lifecycle() -> None:
        lifecycle_attempted.set()
        storage.soft_delete_case("case-a")
        storage.restore_case("case-a")
        order.append("lifecycle_changed")
        lifecycle_finished.set()

    lifecycle = threading.Thread(target=change_lifecycle, daemon=True)
    lifecycle.start()
    assert lifecycle_attempted.wait(timeout=5)
    assert not lifecycle_finished.wait(timeout=0.1)

    release_success_transition.set()
    worker.join(timeout=5)
    lifecycle.join(timeout=5)

    assert not worker.is_alive()
    assert not lifecycle.is_alive()
    assert tasks.get_task(task.task_id).status == TaskStatus.SUCCEEDED
    assert order == ["succeeded", "completed", "lifecycle_changed"]
    assert storage.get_case_lifecycle_generation("case-a") == 3


def test_old_generation_snapshot_manifest_sidecar_and_projection_are_rejected_after_restore(tmp_path) -> None:
    coverage = _complete_coverage()
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()
    (tmp_path / "cases" / "case-a").mkdir(mode=0o700)
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = storage
    graph = _runtime_graph(amount=1.0)
    old_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": graph["nodes"],
                "edges": graph["edges"],
                "stats": {},
                "runtime_graph": graph,
            },
            coverage,
        ),
    )
    assert old_ref is not None
    job_id = str(uuid4())
    old_manifest = repository.persist_job_result_manifest(
        case_id="case-a",
        job_id=job_id,
        snapshot_ref=old_ref,
    )
    assert old_manifest["case_lifecycle_generation"] == 1
    old_compute_path = Path(
        repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=old_ref)
    )
    assert old_compute_path.exists()

    storage.soft_delete_case("case-a")
    storage.restore_case("case-a")
    assert storage.get_case_lifecycle_generation("case-a") == 3

    assert repository.get_result_snapshot(case_id="case-a", snapshot_ref=old_ref)["runtime_graph"] == {
        "nodes": [],
        "edges": [],
    }
    assert repository.get_job_result_manifest(case_id="case-a", job_id=job_id) is None
    assert repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=old_ref) == ""

    fresh_graph = _runtime_graph(amount=2.0)
    stale_projection = _attach_candidate_amount_coverage(
        {
            "nodes": fresh_graph["nodes"],
            "edges": fresh_graph["edges"],
            "stats": {},
            "runtime_graph": fresh_graph,
            "projection": {"source_result_snapshot_ref": old_ref},
        },
        coverage,
    )
    assert repository.persist_result_snapshot(case_id="case-a", result=stale_projection) is None

    fresh_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": fresh_graph["nodes"],
                "edges": fresh_graph["edges"],
                "stats": {},
                "runtime_graph": fresh_graph,
            },
            coverage,
        ),
    )
    assert fresh_ref is not None
    validated = repository._read_valid_result_snapshot_payload(
        case_id="case-a",
        snapshot_ref=fresh_ref,
    )
    assert validated is not None
    assert validated[1]["case_lifecycle_generation"] == 3

    registry = json.loads(storage.registry_path.read_text(encoding="utf-8"))
    registry["case-a"].pop("lifecycle_generation")
    storage.registry_path.write_text(json.dumps(registry), encoding="utf-8")

    with pytest.raises(RuntimeError, match="case_registry_invalid"):
        storage.get_case_binding_state("case-a")
    assert repository.get_result_snapshot(case_id="case-a", snapshot_ref=old_ref)[
        "runtime_graph"
    ] == {"nodes": [], "edges": []}
    assert repository.get_job_result_manifest(case_id="case-a", job_id=job_id) is None


def test_registry_metadata_writer_cannot_rollback_lifecycle_generation(tmp_path, monkeypatch) -> None:
    initial = {
        "case-a": {
            "name": "before",
            "created_at": "2026-07-20T00:00:00+00:00",
            "updated_at": "2026-07-20T00:00:00+00:00",
            "deleted_at": "",
            "lifecycle_generation": 1,
        }
    }
    metadata_storage = _case_storage(tmp_path, initial)
    metadata_storage._save_registry()
    lifecycle_storage = _case_storage(tmp_path)
    (tmp_path / "cases" / "case-a").mkdir(mode=0o700)

    writer_entered = threading.Event()
    release_writer = threading.Event()
    lifecycle_done = threading.Event()
    failures: list[BaseException] = []
    original_write = metadata_storage._write_registry_file

    def blocking_write() -> None:
        writer_entered.set()
        if not release_writer.wait(timeout=5):
            raise RuntimeError("test_registry_writer_release_timeout")
        original_write()

    monkeypatch.setattr(metadata_storage, "_write_registry_file", blocking_write)

    def update_metadata() -> None:
        try:
            metadata_storage.update_case_meta("case-a", name="after")
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)

    def delete_case() -> None:
        try:
            lifecycle_storage.soft_delete_case("case-a")
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)
        finally:
            lifecycle_done.set()

    metadata_thread = threading.Thread(target=update_metadata, daemon=True)
    metadata_thread.start()
    assert writer_entered.wait(timeout=5)
    lifecycle_thread = threading.Thread(target=delete_case, daemon=True)
    lifecycle_thread.start()
    assert not lifecycle_done.wait(timeout=0.1)
    release_writer.set()
    metadata_thread.join(timeout=5)
    lifecycle_thread.join(timeout=5)

    final_registry = lifecycle_storage._load_registry()
    assert failures == []
    assert final_registry["case-a"]["name"] == "after"
    assert final_registry["case-a"]["deleted_at"]
    assert final_registry["case-a"]["lifecycle_generation"] == 2


def test_concurrent_different_case_lifecycle_updates_are_not_lost(tmp_path) -> None:
    initial = {
        case_id: {
            "name": case_id,
            "created_at": "2026-07-20T00:00:00+00:00",
            "updated_at": "2026-07-20T00:00:00+00:00",
            "deleted_at": "",
            "lifecycle_generation": 1,
        }
        for case_id in ("case-a", "case-b")
    }
    first = _case_storage(tmp_path, initial)
    first._save_registry()
    second = _case_storage(tmp_path)
    for case_id in initial:
        (tmp_path / "cases" / case_id).mkdir(mode=0o700)
    start = threading.Barrier(3)
    failures: list[BaseException] = []

    def delete(storage: CaseStorage, case_id: str) -> None:
        try:
            start.wait(timeout=5)
            storage.soft_delete_case(case_id)
        except BaseException as exc:  # pragma: no cover - asserted below
            failures.append(exc)

    first_thread = threading.Thread(target=delete, args=(first, "case-a"), daemon=True)
    second_thread = threading.Thread(target=delete, args=(second, "case-b"), daemon=True)
    first_thread.start()
    second_thread.start()
    start.wait(timeout=5)
    first_thread.join(timeout=5)
    second_thread.join(timeout=5)

    final_registry = first._load_registry()
    assert failures == []
    assert final_registry["case-a"]["deleted_at"]
    assert final_registry["case-b"]["deleted_at"]
    assert final_registry["case-a"]["lifecycle_generation"] == 2
    assert final_registry["case-b"]["lifecycle_generation"] == 2


@pytest.mark.parametrize("field", ("deleted_at", "lifecycle_generation"))
def test_update_case_meta_rejects_lifecycle_authority_fields(tmp_path, field: str) -> None:
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()

    with pytest.raises(ValueError, match="case_lifecycle_field_reserved"):
        storage.update_case_meta("case-a", **{field: "forged"})

    binding = storage.get_case_binding_state("case-a")
    assert binding == {"case_id": "case-a", "deleted_at": "", "lifecycle_generation": 1}


def test_corrupt_or_symlink_registry_fails_closed_without_overwrite(tmp_path) -> None:
    storage = _case_storage(
        tmp_path,
        {
            "case-a": {
                "name": "case-a",
                "created_at": "2026-07-20T00:00:00+00:00",
                "updated_at": "2026-07-20T00:00:00+00:00",
                "deleted_at": "",
                "lifecycle_generation": 1,
            }
        },
    )
    storage._save_registry()
    storage.registry_path.write_text("{broken", encoding="utf-8")

    with pytest.raises(RuntimeError, match="case_registry_invalid"):
        storage.soft_delete_case("case-a")
    assert storage.registry_path.read_text(encoding="utf-8") == "{broken"

    external = tmp_path / "external-registry.json"
    external.write_text('{"case-a":{"lifecycle_generation":999}}', encoding="utf-8")
    storage.registry_path.unlink()
    storage.registry_path.symlink_to(external)
    with pytest.raises(RuntimeError, match="case_registry_invalid"):
        storage.get_case_binding_state("case-a")
    assert external.read_text(encoding="utf-8") == '{"case-a":{"lifecycle_generation":999}}'


@pytest.mark.parametrize(
    ("field", "value"),
    (
        ("status", None),
        ("status", ""),
        ("status", "unknown"),
        ("deleted_at", None),
        ("deleted_at", False),
        ("lifecycle_generation", None),
        ("lifecycle_generation", 0),
        ("lifecycle_generation", True),
    ),
)
def test_case_registry_lifecycle_authority_fields_are_required_and_strict(
    tmp_path,
    field: str,
    value: object,
) -> None:
    storage = _case_storage(tmp_path)
    metadata = {
        "name": "case-a",
        "created_at": "2026-07-20T00:00:00+00:00",
        "updated_at": "2026-07-20T00:00:00+00:00",
        "status": "进行中",
        "deleted_at": "",
        "lifecycle_generation": 1,
    }
    metadata[field] = value
    storage.registry_path.write_text(
        json.dumps({"case-a": metadata}, ensure_ascii=False),
        encoding="utf-8",
    )

    with pytest.raises(RuntimeError, match="case_registry_invalid"):
        storage.get_case_binding_state("case-a")


@pytest.mark.parametrize("field", ("status", "deleted_at", "lifecycle_generation"))
def test_case_registry_lifecycle_authority_fields_cannot_be_absent(tmp_path, field: str) -> None:
    storage = _case_storage(tmp_path)
    metadata = {
        "name": "case-a",
        "created_at": "2026-07-20T00:00:00+00:00",
        "updated_at": "2026-07-20T00:00:00+00:00",
        "status": "进行中",
        "deleted_at": "",
        "lifecycle_generation": 1,
    }
    metadata.pop(field)
    storage.registry_path.write_text(
        json.dumps({"case-a": metadata}, ensure_ascii=False),
        encoding="utf-8",
    )

    with pytest.raises(RuntimeError, match="case_registry_invalid"):
        storage.get_case_binding_state("case-a")


@pytest.mark.parametrize("duplicate_kind", ("node", "edge"))
def test_delta_snapshot_with_duplicate_identity_falls_back_to_lossless_full(
    tmp_path,
    duplicate_kind: str,
) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None
    target_graph = _runtime_graph(amount=1.0)
    if duplicate_kind == "node":
        target_graph["nodes"].append({**target_graph["nodes"][0], "x": 42.5})
    else:
        target_graph["edges"].append({**target_graph["edges"][0], "label": "duplicate"})
    target_ref = repository.persist_result_snapshot(
        case_id="case-a",
        base_snapshot_ref=base_ref,
        result=_attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
            },
            coverage,
        ),
    )

    assert target_ref is not None
    assert target_ref["graph_storage_mode"] == "full"
    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert len(loaded["runtime_graph"][f"{duplicate_kind}s"]) == len(target_graph[f"{duplicate_kind}s"])


def test_delta_snapshot_preserves_target_runtime_revision(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    base_graph = _runtime_graph(amount=1.0)
    base_graph["runtime_revision"] = 1
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None
    target_graph = _runtime_graph(amount=1.0)
    target_graph["runtime_revision"] = 2
    target_graph["nodes"][0]["x"] = 42.5
    target_ref = repository.persist_result_snapshot(
        case_id="case-a",
        base_snapshot_ref=base_ref,
        result=_attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
            },
            coverage,
        ),
    )

    assert target_ref is not None and target_ref["graph_storage_mode"] == "delta"
    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert loaded["runtime_graph"]["runtime_revision"] == 2


def test_hostile_precomputed_delta_patch_cannot_replace_recomputed_target(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    base_graph = _runtime_graph(amount=1.0)
    base_ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": base_graph["nodes"],
                "edges": base_graph["edges"],
                "stats": {},
                "runtime_graph": base_graph,
            },
            coverage,
        ),
    )
    assert base_ref is not None
    target_graph = _runtime_graph(amount=1.0)
    target_graph["nodes"][0]["x"] = 42.5
    hostile_patch = {
        "remove_node_ids": [],
        "upsert_nodes": [],
        "remove_edge_ids": [],
        "upsert_edges": [],
        "summary": {
            "base_nodes": 2,
            "base_edges": 1,
            "target_nodes": 2,
            "target_edges": 1,
            "op_count": 0,
            "target_entity_count": 3,
        },
    }
    target_ref = repository._persist_result_snapshot(
        "case-a",
        _attach_candidate_amount_coverage(
            {
                "nodes": target_graph["nodes"],
                "edges": target_graph["edges"],
                "stats": {},
                "runtime_graph": target_graph,
            },
            coverage,
        ),
        base_snapshot_ref=base_ref,
        precomputed_graph_patch=hostile_patch,
    )

    assert target_ref is not None
    assert target_ref["graph_storage_mode"] == "full"
    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=target_ref)
    assert loaded["runtime_graph"]["nodes"][0]["x"] == 42.5


def test_projection_layout_rejects_non_finite_coordinates_without_new_snapshot(
    tmp_path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    source_ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert source_ref is not None
    monkeypatch.setattr(
        flow_repository_module,
        "project_flow_projection_layout_sync",
        lambda **_kwargs: {
            "layout_by_id": {"acct-a": {"x": float("inf"), "y": 1.0}},
            "node_updates": [{"index": 0, "id": "acct-a", "x": float("inf"), "y": 1.0}],
            "layout_index_summary": {},
            "signature": "hostile-layout",
        },
    )
    before = len(repository._iter_result_snapshot_paths("case-a"))

    summary = repository.update_result_snapshot_projection_layout(
        case_id="case-a",
        snapshot_ref=source_ref,
        layout_index={},
    )

    assert summary["layout_sync_skipped"] is True
    assert summary["snapshot_ref"]["snapshot_id"] == source_ref["snapshot_id"]
    assert len(repository._iter_result_snapshot_paths("case-a")) == before


def test_result_snapshot_gc_symlink_only_inventory_blocks_without_mutation(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    valid_ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert valid_ref is not None
    snapshot_dir = repository._result_snapshots_dir("case-a")
    external = tmp_path / "external-fact.json"
    external.write_text('{"amount":999}', encoding="utf-8")
    symlink = snapshot_dir / ("b" * 64 + ".json")
    symlink.symlink_to(external)
    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")
    gc_summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=0,
        keep_latest=0,
        ttl_seconds=0,
    )

    assert metrics["inventory_status"] == "unknown"
    assert metrics["snapshot_count"] is None
    assert gc_summary["deleted_count"] == 0
    assert gc_summary["blocked_reason"] == "invalid_snapshot_inventory"
    assert repository._result_snapshot_path("case-a", valid_ref["snapshot_id"]).exists()
    assert external.read_text(encoding="utf-8") == '{"amount":999}'
    assert symlink.is_symlink()


@pytest.mark.parametrize("inventory_kind", ("hardlink", "insecure_mode"))
def test_result_snapshot_gc_rejects_non_private_or_multilink_inventory(tmp_path, inventory_kind: str) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        ref = repository.persist_result_snapshot(
            case_id="case-a",
            result=_attach_candidate_amount_coverage(
                {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
                coverage,
            ),
        )
        assert ref is not None
        refs.append(ref)
    first_path = repository._result_snapshot_path("case-a", refs[0]["snapshot_id"])
    if inventory_kind == "hardlink":
        os.link(first_path, first_path.with_name("b" * 64 + ".json"))
    else:
        first_path.chmod(0o644)

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["deleted_count"] == 0
    assert summary["blocked_reason"] == "invalid_snapshot_inventory"
    assert all(repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists() for ref in refs)


def test_mismatched_snapshot_graph_hash_is_rejected(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert ref is not None
    mismatched = {**ref, "graph_hash": "f" * 64}

    loaded = repository.get_result_snapshot(case_id="case-a", snapshot_ref=mismatched)

    assert loaded["nodes"] == []
    assert loaded["edges"] == []
    assert loaded["runtime_graph"] == {"nodes": [], "edges": []}


def test_flow_result_gc_keeps_every_snapshot_rooted_by_valid_job_manifest(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        result = _attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        )
        ref = repository.persist_result_snapshot(case_id="case-a", result=result)
        assert ref is not None
        refs.append(ref)

    repository.persist_job_result_manifest(
        case_id="case-a",
        job_id=str(uuid4()),
        snapshot_ref=refs[0],
    )
    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["deleted_count"] == 0
    assert repository._result_snapshot_path("case-a", refs[0]["snapshot_id"]).exists()
    assert repository._result_snapshot_path("case-a", refs[1]["snapshot_id"]).exists()


def test_flow_result_gc_manifest_corruption_blocks_all_deletion(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        result = _attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        )
        ref = repository.persist_result_snapshot(case_id="case-a", result=result)
        assert ref is not None
        refs.append(ref)
    job_id = str(uuid4())
    repository.persist_job_result_manifest(case_id="case-a", job_id=job_id, snapshot_ref=refs[0])
    repository._job_result_manifest_path("case-a", job_id).write_text("{}", encoding="utf-8")

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["deleted_count"] == 0
    assert summary["blocked_reason"] == "invalid_job_result_manifest_inventory"
    assert all(repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists() for ref in refs)


def test_flow_result_gc_validates_manifest_before_soft_limit_early_return(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        ),
    )
    assert ref is not None
    invalid_manifest = repository._job_result_manifests_dir("case-a") / f"{uuid4()}.json"
    invalid_manifest.parent.mkdir(parents=True, mode=0o700)
    invalid_manifest.write_text("{}", encoding="utf-8")
    invalid_manifest.chmod(0o600)

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=100,
        keep_latest=100,
        ttl_seconds=3600,
    )
    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")

    assert summary["inventory_status"] == "unknown"
    assert summary["blocked_reason"] == "invalid_job_result_manifest_inventory"
    assert summary["deleted_count"] == 0
    assert metrics["inventory_status"] == "unknown"
    assert "invalid_job_result_manifest_inventory" in metrics["blocked_reasons"]
    assert repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists()


@pytest.mark.parametrize("location", ("snapshot", "manifest", "compute"))
def test_flow_result_inventory_rejects_every_unknown_case_entry(tmp_path, location: str) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        ),
    )
    assert ref is not None
    if location == "snapshot":
        unknown = repository._result_snapshots_dir("case-a") / "unknown.bin"
    elif location == "manifest":
        unknown = repository._job_result_manifests_dir("case-a") / "unknown.bin"
    else:
        unknown = repository._result_snapshots_dir("case-a") / "_compute" / "unknown.bin"
    unknown.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    unknown.write_text("untrusted", encoding="utf-8")
    unknown.chmod(0o600)

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=100,
        keep_latest=100,
        ttl_seconds=3600,
    )
    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")

    assert summary["inventory_status"] == "unknown"
    assert summary["deleted_count"] == 0
    assert metrics["inventory_status"] == "unknown"
    assert unknown.exists()
    assert repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists()


def test_result_snapshot_inventory_rejects_intermediate_directory_symlink(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    external = tmp_path / "external-snapshots"
    external_case = external / repository._result_snapshots_dir("case-a").name
    external_case.mkdir(parents=True)
    hostile = external_case / ("a" * 64 + ".json")
    hostile.write_text('{"amount":999}', encoding="utf-8")
    hostile.chmod(0o600)
    (tmp_path / "flow_result_snapshots").symlink_to(external, target_is_directory=True)

    with pytest.raises(OSError, match="private_artifact_list_invalid"):
        repository._iter_result_snapshot_paths("case-a")
    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")
    assert metrics["inventory_status"] == "unknown"
    assert metrics["snapshot_count"] is None
    gc_summary = repository._gc_case_result_snapshots("case-a", soft_limit=0, keep_latest=0, ttl_seconds=0)
    assert gc_summary["blocked_reason"] == "invalid_snapshot_inventory"
    assert gc_summary["before_count"] is None
    assert hostile.read_text(encoding="utf-8") == '{"amount":999}'


def test_result_snapshot_rejects_duplicate_keys_unknown_fields_and_nonfinite_values(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )
    ref = repository.persist_result_snapshot(case_id="case-a", result=result)
    assert ref is not None
    path = repository._result_snapshot_path("case-a", ref["snapshot_id"])
    original = path.read_text(encoding="utf-8")

    duplicate = original.replace(
        f'"snapshot_id":"{ref["snapshot_id"]}"',
        f'"snapshot_id":"6222020202020202020","snapshot_id":"{ref["snapshot_id"]}"',
        1,
    )
    path.write_text(duplicate, encoding="utf-8")
    path.chmod(0o600)
    assert repository._read_valid_result_snapshot_payload(case_id="case-a", snapshot_ref=ref) is None

    payload = json.loads(original)
    payload["unknown_case_fact"] = "6222020202020202020"
    path.write_text(dumps_canonical_json(payload), encoding="utf-8")
    path.chmod(0o600)
    assert repository._read_valid_result_snapshot_payload(case_id="case-a", snapshot_ref=ref) is None

    nonfinite = original.replace('"amount":1.0', '"amount":NaN', 1)
    assert nonfinite != original
    path.write_text(nonfinite, encoding="utf-8")
    path.chmod(0o600)
    assert repository._read_valid_result_snapshot_payload(case_id="case-a", snapshot_ref=ref) is None


def test_result_snapshot_never_writes_nonfinite_json(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    graph["nodes"][0]["x"] = float("nan")
    result = _attach_candidate_amount_coverage(
        {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
        coverage,
    )

    assert repository.persist_result_snapshot(case_id="case-a", result=result) is None
    assert not repository._result_snapshots_dir("case-a").exists()


def test_snapshot_stored_at_tamper_cannot_change_gc_authority(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        ref = repository.persist_result_snapshot(
            case_id="case-a",
            result=_attach_candidate_amount_coverage(
                {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
                coverage,
            ),
        )
        assert ref is not None
        refs.append(ref)

    older_path = repository._result_snapshot_path("case-a", refs[0]["snapshot_id"])
    newer_path = repository._result_snapshot_path("case-a", refs[1]["snapshot_id"])
    payload = json.loads(older_path.read_text(encoding="utf-8"))
    payload["stored_at"] = "2999-01-01T00:00:00+00:00"
    payload["envelope_sha256"] = _result_snapshot_envelope_digest(payload)
    older_path.write_text(dumps_canonical_json(payload), encoding="utf-8")
    older_path.chmod(0o600)
    now = time.time()
    os.utime(older_path, (now - 300, now - 300))
    os.utime(newer_path, (now - 100, now - 100))
    assert repository._read_valid_result_snapshot_payload(case_id="case-a", snapshot_ref=refs[0]) is not None

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["deleted_count"] == 1
    assert not older_path.exists()
    assert newer_path.exists()


def test_job_manifest_symlink_only_inventory_blocks_snapshot_gc(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        ref = repository.persist_result_snapshot(
            case_id="case-a",
            result=_attach_candidate_amount_coverage(
                {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
                coverage,
            ),
        )
        assert ref is not None
        refs.append(ref)
    external = tmp_path / "external-manifest.json"
    external.write_text('{"snapshot_id":"untrusted"}', encoding="utf-8")
    manifest_path = repository._job_result_manifests_dir("case-a") / f"{uuid4()}.json"
    manifest_path.parent.mkdir(parents=True)
    manifest_path.symlink_to(external)

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["deleted_count"] == 0
    assert summary["blocked_reason"] == "invalid_job_result_manifest_inventory"
    assert all(repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists() for ref in refs)


def test_snapshot_catalog_failure_and_case_unknown_never_aggregate_to_zero(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))

    metrics = repository.collect_result_snapshot_metrics()
    gc_all = repository.gc_result_snapshots()

    assert metrics["inventory_status"] == "unknown"
    assert metrics["case_count"] is None
    assert metrics["snapshot_count"] is None
    assert gc_all["inventory_status"] == "unknown"
    assert gc_all["case_count"] is None
    assert gc_all["before_count"] is None
    assert gc_all["after_count"] is None


def test_explicit_unknown_case_metrics_and_gc_are_unknown_not_zero(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))

    metrics = repository.collect_result_snapshot_metrics(case_id="missing-case")
    gc_summary = repository.gc_result_snapshots(case_id="missing-case")

    assert metrics["inventory_status"] == "unknown"
    assert metrics["snapshot_count"] is None
    assert metrics["total_bytes"] is None
    assert metrics["cases"][0]["blocked_reason"] == "snapshot_case_binding_unavailable"
    assert gc_summary["inventory_status"] == "unknown"
    assert gc_summary["before_count"] is None
    assert gc_summary["after_count"] is None
    assert gc_summary["deleted_count"] is None
    assert gc_summary["cases"][0]["blocked_reason"] == "snapshot_case_binding_unavailable"


def test_dependency_inventory_uses_bounded_iterative_walk(tmp_path, monkeypatch) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    monkeypatch.setattr(
        repository,
        "_load_result_snapshot",
        lambda *_args, **_kwargs: {"runtime_graph": {"nodes": [{"id": "node"}], "edges": []}},
    )
    snapshot_ids = [hashlib.sha256(f"snapshot-{index}".encode()).hexdigest() for index in range(400)]
    metas = [
        {
            "snapshot_id": snapshot_id,
            "source_snapshot_id": snapshot_ids[index - 1] if index else "",
            "graph_base_snapshot_id": "",
            "graph_storage_mode": "full",
            "graph_storage_chain_depth": 0,
            "node_count": 1,
            "edge_count": 0,
        }
        for index, snapshot_id in enumerate(snapshot_ids)
    ]

    valid, depths = repository._validate_result_snapshot_dependency_inventory("case-a", metas)

    assert valid is True
    assert depths[snapshot_ids[-1]] == len(snapshot_ids) - 1


def test_excessive_projection_dependency_chain_fails_closed(tmp_path, monkeypatch) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    monkeypatch.setattr(
        repository,
        "_load_result_snapshot",
        lambda *_args, **_kwargs: pytest.fail("excessive dependency inventory reached graph loading"),
    )
    snapshot_ids = [hashlib.sha256(f"snapshot-{index}".encode()).hexdigest() for index in range(514)]
    metas = [
        {
            "snapshot_id": snapshot_id,
            "source_snapshot_id": snapshot_ids[index - 1] if index else "",
            "graph_base_snapshot_id": "",
            "graph_storage_mode": "full",
            "graph_storage_chain_depth": 0,
            "node_count": 1,
            "edge_count": 0,
        }
        for index, snapshot_id in enumerate(snapshot_ids)
    ]

    assert repository._validate_result_snapshot_dependency_inventory("case-a", metas) == (False, {})


def test_dependency_cycle_returns_unknown_without_recursion_error(tmp_path, monkeypatch) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    monkeypatch.setattr(
        repository,
        "_load_result_snapshot",
        lambda *_args, **_kwargs: pytest.fail("cyclic dependency inventory reached graph loading"),
    )
    first = hashlib.sha256(b"first").hexdigest()
    second = hashlib.sha256(b"second").hexdigest()
    metas = [
        {
            "snapshot_id": first,
            "source_snapshot_id": second,
            "graph_base_snapshot_id": "",
            "graph_storage_mode": "full",
            "graph_storage_chain_depth": 0,
            "node_count": 1,
            "edge_count": 0,
        },
        {
            "snapshot_id": second,
            "source_snapshot_id": first,
            "graph_base_snapshot_id": "",
            "graph_storage_mode": "full",
            "graph_storage_chain_depth": 0,
            "node_count": 1,
            "edge_count": 0,
        },
    ]

    assert repository._validate_result_snapshot_dependency_inventory("case-a", metas) == (False, {})


@pytest.mark.parametrize("root_name", ("flow_result_snapshots", "flow_job_results"))
def test_global_snapshot_inventory_rejects_orphan_case_directories(tmp_path, root_name: str) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = _StorageWithCases(tmp_path)
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": graph["nodes"],
                "edges": graph["edges"],
                "stats": {},
                "runtime_graph": graph,
            },
            coverage,
        ),
    )
    assert ref is not None
    orphan = tmp_path / root_name / ("case-" + "f" * 64)
    orphan.mkdir(parents=True, mode=0o700)
    orphan.chmod(0o700)

    metrics = repository.collect_result_snapshot_metrics()
    gc_all = repository.gc_result_snapshots()

    assert metrics["inventory_status"] == "unknown"
    assert metrics["case_count"] is None
    assert metrics["snapshot_count"] is None
    assert gc_all["inventory_status"] == "unknown"
    assert gc_all["case_count"] is None
    assert gc_all["before_count"] is None
    assert repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists()


@pytest.mark.parametrize("operation", ("metrics", "gc"))
def test_global_inventory_insert_between_root_scans_returns_unknown_without_deletion(
    tmp_path,
    monkeypatch,
    operation: str,
) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._storage = _StorageWithCases(tmp_path)
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": graph["nodes"],
                "edges": graph["edges"],
                "stats": {},
                "runtime_graph": graph,
            },
            coverage,
        ),
    )
    assert ref is not None
    snapshot_path = repository._result_snapshot_path("case-a", ref["snapshot_id"])
    original_list = flow_repository_module.list_private_directories
    calls = 0

    def insert_between_scans(root: Path):
        nonlocal calls
        calls += 1
        if calls == 2:
            orphan = tmp_path / "flow_result_snapshots" / "orphan-unregistered-case"
            orphan.mkdir(parents=True, mode=0o700)
            orphan.chmod(0o700)
        return original_list(root)

    monkeypatch.setattr(flow_repository_module, "list_private_directories", insert_between_scans)

    summary = (
        repository.collect_result_snapshot_metrics()
        if operation == "metrics"
        else repository.gc_result_snapshots()
    )

    assert summary["inventory_status"] == "unknown"
    assert summary["case_count"] is None
    assert snapshot_path.exists()
    if operation == "gc":
        assert summary["deleted_count"] is None


def test_case_gc_unknown_inventory_keeps_aggregate_totals_unknown(tmp_path) -> None:
    repository = _repository(tmp_path, _CompleteDailyAggregate(_complete_coverage()))
    snapshot_dir = repository._result_snapshots_dir("case-a")
    snapshot_dir.mkdir(parents=True)
    external = tmp_path / "external.json"
    external.write_text("{}", encoding="utf-8")
    (snapshot_dir / ("a" * 64 + ".json")).symlink_to(external)

    summary = repository.gc_result_snapshots(case_id="case-a")

    assert summary["inventory_status"] == "unknown"
    assert summary["before_count"] is None
    assert summary["after_count"] is None
    assert summary["deleted_count"] is None


def test_snapshot_inventory_rejects_insecure_case_directory(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {
                "nodes": graph["nodes"],
                "edges": graph["edges"],
                "stats": {},
                "runtime_graph": graph,
            },
            coverage,
        ),
    )
    assert ref is not None
    snapshot_dir = repository._result_snapshots_dir("case-a")
    snapshot_dir.chmod(0o777)

    metrics = repository.collect_result_snapshot_metrics(case_id="case-a")
    gc_summary = repository.gc_result_snapshots(case_id="case-a")

    assert metrics["inventory_status"] == "unknown"
    assert metrics["snapshot_count"] is None
    assert gc_summary["inventory_status"] == "unknown"
    assert gc_summary["before_count"] is None
    assert repository._result_snapshot_path("case-a", ref["snapshot_id"]).exists()


def test_gc_retains_compute_sidecar_for_retained_snapshot(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    refs: list[dict] = []
    sidecars: list[Path] = []
    for amount in (1.0, 2.0):
        graph = _runtime_graph(amount=amount)
        ref = repository.persist_result_snapshot(
            case_id="case-a",
            result=_attach_candidate_amount_coverage(
                {
                    "nodes": graph["nodes"],
                    "edges": graph["edges"],
                    "stats": {},
                    "runtime_graph": graph,
                },
                coverage,
            ),
        )
        assert ref is not None
        refs.append(ref)
        sidecar = Path(repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=ref))
        assert sidecar.name.endswith(".network-graph.json")
        sidecars.append(sidecar)
    now = time.time()
    os.utime(repository._result_snapshot_path("case-a", refs[0]["snapshot_id"]), (now - 300, now - 300))
    os.utime(repository._result_snapshot_path("case-a", refs[1]["snapshot_id"]), (now - 100, now - 100))

    summary = repository._gc_case_result_snapshots(
        "case-a",
        soft_limit=1,
        keep_latest=1,
        ttl_seconds=0,
    )

    assert summary["inventory_status"] == "valid"
    assert summary["deleted_count"] == 1
    assert not sidecars[0].exists()
    assert sidecars[1].exists()


def test_compute_sidecar_lease_failure_returns_no_path(tmp_path, monkeypatch) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        ),
    )
    assert ref is not None

    def unavailable_lock(*_args, **_kwargs):
        raise OSError("private_artifact_lock_timeout")

    monkeypatch.setattr(flow_repository_module, "private_exclusive_file_lock", unavailable_lock)

    assert repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=ref) == ""


def test_compute_sidecar_conflict_never_falls_back_to_snapshot_path(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    graph = _runtime_graph(amount=1.0)
    ref = repository.persist_result_snapshot(
        case_id="case-a",
        result=_attach_candidate_amount_coverage(
            {"nodes": graph["nodes"], "edges": graph["edges"], "stats": {}, "runtime_graph": graph},
            coverage,
        ),
    )
    assert ref is not None
    compute_path = repository._result_snapshot_compute_graph_path("case-a", ref["snapshot_id"])
    compute_path.parent.mkdir(parents=True, mode=0o700)
    compute_path.write_text('{"version":2,"snapshot_id":"hostile"}', encoding="utf-8")
    compute_path.chmod(0o600)

    observed = repository.get_result_snapshot_compute_path(case_id="case-a", snapshot_ref=ref)

    assert observed == ""
    assert observed != str(repository._result_snapshot_path("case-a", ref["snapshot_id"]))


def test_missing_runtime_edge_amount_cannot_become_real_zero(tmp_path) -> None:
    coverage = _complete_coverage()
    repository = _repository(tmp_path, _CompleteDailyAggregate(coverage))
    repository._build_runtime_graph = lambda **_kwargs: _runtime_graph(amount=...)

    with pytest.raises(TxnAmountCoverageIncompleteError, match="^transaction_amount_coverage_incomplete$"):
        repository.build_graph(
            case_id="case-a",
            seeds=["acct-a"],
            depth=1,
            direction="both",
            min_amount=0,
            request_context={},
        )
