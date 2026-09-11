from __future__ import annotations

import json
import os
from types import SimpleNamespace

import pytest

import app.repositories.flow_repository as flow_repository_module
from app.repositories.flow_repository import FlowRepository, FlowViewMetadataError


CASE_ID = "case-flow-metadata"
PRIVATE_SENTINEL = "6222020202020202020 /private/case.csv 987654321.23"


def _repository(tmp_path) -> FlowRepository:
    repository = object.__new__(FlowRepository)
    repository._storage = SimpleNamespace(app_dir=tmp_path)
    return repository


def test_saved_view_persists_only_private_case_bound_metadata(tmp_path) -> None:
    repository = _repository(tmp_path)

    created = repository.create_view(case_id=CASE_ID)
    path = repository._case_views_path(CASE_ID)
    raw = path.read_text(encoding="utf-8")
    payload = json.loads(raw)

    assert os.stat(path).st_mode & 0o077 == 0
    assert set(payload) == {"version", "case_id", "views"}
    assert payload["version"] == 2
    assert payload["case_id"] == CASE_ID
    assert payload["views"] == [{"case_id": CASE_ID, "view_id": created["view_id"]}]
    assert PRIVATE_SENTINEL not in raw
    assert not (tmp_path / "flow_view_snapshots").exists()
    assert not (tmp_path / "flow_views").exists()

    before = path.stat().st_mtime_ns
    listed, total = repository.list_views(case_id=CASE_ID, page=1, page_size=50)
    found = repository.get_view(case_id=CASE_ID, view_id=created["view_id"])
    assert path.stat().st_mtime_ns == before
    assert total == 1
    assert listed == [found]
    assert found["graph_query"] == {}
    assert found["view_state"]["graph"] == {"nodes": [], "edges": []}
    assert "snapshot_ref" not in json.dumps(found, sort_keys=True)


def test_saved_view_metadata_rejects_unknown_fact_fields_without_migration(tmp_path) -> None:
    repository = _repository(tmp_path)
    created = repository.create_view(case_id=CASE_ID)
    path = repository._case_views_path(CASE_ID)
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["views"][0]["graph"] = {"nodes": [{"account": PRIVATE_SENTINEL}]}
    path.write_text(json.dumps(payload), encoding="utf-8")

    with pytest.raises(FlowViewMetadataError, match="flow_view_metadata_invalid"):
        repository.list_views(case_id=CASE_ID, page=1, page_size=50)

    assert PRIVATE_SENTINEL in path.read_text(encoding="utf-8")


def test_saved_view_metadata_rejects_duplicate_keys_with_hidden_pii(tmp_path) -> None:
    repository = _repository(tmp_path)
    created = repository.create_view(case_id=CASE_ID)
    path = repository._case_views_path(CASE_ID)
    raw = path.read_text(encoding="utf-8")
    hostile = raw.replace(
        f'"view_id":"{created["view_id"]}"',
        f'"view_id":"{PRIVATE_SENTINEL}","view_id":"{created["view_id"]}"',
        1,
    )
    path.write_text(hostile, encoding="utf-8")
    path.chmod(0o600)

    with pytest.raises(FlowViewMetadataError, match="flow_view_metadata_invalid"):
        repository.list_views(case_id=CASE_ID, page=1, page_size=50)

    assert PRIVATE_SENTINEL in path.read_text(encoding="utf-8")


def test_saved_view_metadata_rejects_nonfinite_json_constants(tmp_path) -> None:
    repository = _repository(tmp_path)
    repository.create_view(case_id=CASE_ID)
    path = repository._case_views_path(CASE_ID)
    payload = json.loads(path.read_text(encoding="utf-8"))
    payload["case_id"] = float("nan")
    path.write_text(json.dumps(payload, allow_nan=True, separators=(",", ":")), encoding="utf-8")
    path.chmod(0o600)

    with pytest.raises(FlowViewMetadataError, match="flow_view_metadata_invalid"):
        repository.list_views(case_id=CASE_ID, page=1, page_size=50)


def test_legacy_saved_view_artifacts_are_purged_without_following_symlinks(tmp_path) -> None:
    repository = _repository(tmp_path)
    legacy_views = tmp_path / "flow_views"
    legacy_views.mkdir()
    (legacy_views / "case.json").write_text(PRIVATE_SENTINEL, encoding="utf-8")
    external = tmp_path / "external"
    external.mkdir()
    external_sentinel = external / "keep.json"
    external_sentinel.write_text(PRIVATE_SENTINEL, encoding="utf-8")
    legacy_snapshots = tmp_path / "flow_view_snapshots"
    legacy_snapshots.symlink_to(external, target_is_directory=True)

    repository._purge_legacy_flow_view_artifacts()

    assert not legacy_views.exists()
    assert not legacy_snapshots.exists()
    assert external_sentinel.read_text(encoding="utf-8") == PRIVATE_SENTINEL


def test_legacy_saved_view_purge_failure_blocks_repository_readiness(tmp_path, monkeypatch) -> None:
    repository = _repository(tmp_path)

    def fail(_root, _path) -> None:
        raise OSError("simulated purge failure")

    monkeypatch.setattr(flow_repository_module, "remove_path_no_follow_under", fail)
    with pytest.raises(FlowViewMetadataError, match="legacy_flow_view_purge_failed"):
        repository._purge_legacy_flow_view_artifacts()
