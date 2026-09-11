from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.api.v1 import stats as stats_api
from app.core import analysis_compute
from app.core import data_engine_client
from app.core import analysis_compute_runner
from app.core import analysis_compute_worker
from app.core import import_accelerator
from app.core.execution_authority_health import ExecutionAuthorityHealth
from app.domain import privacy_projection_service
from app.domain.cleaning_service import CleaningService
from app.domain.privacy_projection_service import PrivacyProjectionService, PrivacyProjectionUnavailableError
from app.repositories.import_repository import ImportRepository
from app.repositories import cleaning_native_runtime


_EXECUTION_CAPABILITY_BLOCKER = "managed_process_capability_admission_unavailable"
_DATA_ENGINE_ANALYSIS_COMMANDS = (
    "query-stats-rows",
    "query-stats-tree",
    "query-stats-date-range",
    "query-stats-txn-rows",
    "query-chart-dashboard",
    "query-chart-detail-rows",
    "query-chart-counterparties",
    "query-chart-distribution",
    "query-chart-flow",
    "query-chart-heatmap",
    "query-chart-summary",
    "query-chart-trend",
    "query-flow-focus-rows",
    "query-flow-focus-graph",
    "materialize-txn-daily",
    "materialize-rule-txn-index",
    "materialize-rule-pattern-index",
)


def _phase2_analysis_args(command: str, db_path: Path) -> list[str]:
    return [command, "--case-id", "case-1", "--db-path", str(db_path)]


def test_privacy_projection_without_bundled_binary_reports_unavailable(monkeypatch) -> None:
    service = PrivacyProjectionService()
    monkeypatch.delenv("ANALYTIX_PRIVACY_PROJECTION_BIN", raising=False)

    health = service.get_runtime_health()

    assert health["privacy_projection_native_available"] is False
    assert health["privacy_projection_native_bin"] == ""
    assert health["privacy_projection_native_bin_source"] == ""
    assert health["privacy_projection_native_reason"] == "managed_process_execution_authority_unavailable"
    with pytest.raises(PrivacyProjectionUnavailableError, match="^managed_process_capability_admission_unavailable$"):
        service.start_build("case-1")


def test_privacy_projection_env_binary_cannot_bypass_capability_admission(tmp_path, monkeypatch) -> None:
    class PoisonRepository:
        def get_db_path(self, _case_id: str):
            raise AssertionError("case database was read before capability admission")

    service = PrivacyProjectionService(repository=PoisonRepository())
    binary = tmp_path / "analytix-privacy-projection.exe"
    marker = tmp_path / "must-not-exist"
    binary.write_text(f"marker={marker}", encoding="utf-8")
    monkeypatch.setenv("ANALYTIX_PRIVACY_PROJECTION_BIN", str(binary))

    with pytest.raises(PrivacyProjectionUnavailableError, match="^managed_process_capability_admission_unavailable$"):
        service.start_build("case-1")
    with pytest.raises(PrivacyProjectionUnavailableError, match="^managed_process_capability_admission_unavailable$"):
        service._run_cli("case-1", "disable")

    assert not marker.exists()


def test_privacy_projection_rejects_before_case_scope_is_inspected() -> None:
    inspected: list[str] = []

    class PoisonCase:
        def __str__(self) -> str:
            inspected.append("case")
            raise AssertionError("case scope was inspected before capability admission")

    service = PrivacyProjectionService()
    for operation in (
        lambda: service.toggle(PoisonCase()),
        lambda: service.start_build(PoisonCase()),
        lambda: service._run_cli(PoisonCase(), PoisonCase()),
    ):
        with pytest.raises(
            PrivacyProjectionUnavailableError,
            match="^managed_process_capability_admission_unavailable$",
        ):
            operation()

    assert inspected == []


def test_global_execution_authority_is_consulted_but_never_admits_native_consumers(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    consulted: list[str] = []

    def available(name: str):
        def status() -> ExecutionAuthorityHealth:
            consulted.append(name)
            return ExecutionAuthorityHealth(available=True, reason_code="")

        return status

    for module, name in (
        (analysis_compute_runner, "analysis-compute"),
        (data_engine_client, "data-engine"),
        (import_accelerator, "import-accelerator"),
        (cleaning_native_runtime, "cleaning"),
        (privacy_projection_service, "privacy-projection"),
    ):
        monkeypatch.setattr(module, "execution_authority_health", available(name))

    assert analysis_compute_runner.analysis_compute_binary_health()["analysis_compute_reason"] == (
        "managed_process_capability_admission_unavailable"
    )
    assert data_engine_client.data_engine_binary_health()["data_engine_reason"] == (
        "managed_process_capability_admission_unavailable"
    )
    assert cleaning_native_runtime.native_cleaning_binary_health()["native_cleaning_reason"] == (
        "managed_process_capability_admission_unavailable"
    )
    assert PrivacyProjectionService().get_runtime_health()["privacy_projection_native_reason"] == (
        "managed_process_capability_admission_unavailable"
    )
    with pytest.raises(import_accelerator.ImportAcceleratorUnavailableError):
        import_accelerator.detect_encoding(tmp_path / "case.csv")

    assert consulted == [
        "analysis-compute",
        "data-engine",
        "cleaning",
        "privacy-projection",
        "import-accelerator",
    ]


def test_import_repository_refresh_case_stats_propagates_failure(monkeypatch) -> None:
    repository = ImportRepository()

    class FailingStorage:
        def compute_case_stats(self, case_id: str, *, engine=None) -> None:
            raise RuntimeError(f"stats failed: {case_id}")

    monkeypatch.setattr(repository, "_storage", FailingStorage())

    with pytest.raises(RuntimeError, match="stats failed: case-1"):
        repository.refresh_case_stats("case-1")


def test_import_repository_refresh_analysis_aggregates_requires_materialized_success(monkeypatch) -> None:
    repository = ImportRepository()

    class FailingDailyAggregate:
        def ensure_materialized(self, case_id: str, *, force: bool, engine=None, profile_cb=None) -> bool:
            if profile_cb is not None:
                profile_cb({"ok": False, "native_used": False})
            return False

    monkeypatch.setattr(repository, "_daily_agg", FailingDailyAggregate())

    with pytest.raises(RuntimeError, match="analysis materialization refresh failed"):
        repository.refresh_analysis_aggregates("case-1")


def test_analysis_compute_health_required_commands_cover_chart_flow_and_rule() -> None:
    required = set(analysis_compute_runner._REQUIRED_ANALYSIS_COMPUTE_COMMANDS)

    assert "query-chart-dashboard" in required
    assert "query-chart-flow" in required
    assert "query-flow-focus-graph" in required
    assert "project-flow-graph-render-plan" in required
    assert "materialize-rule-txn-index" in required
    assert "materialize-rule-pattern-index" in required


class _PoisonCaseValue:
    def __init__(self, marker=None) -> None:
        self._marker = marker

    def _mark_read(self) -> None:
        if self._marker is not None:
            self._marker.write_text("read", encoding="utf-8")

    def __str__(self) -> str:
        self._mark_read()
        raise AssertionError("case-scoped value was read before capability admission")

    def __iter__(self):
        self._mark_read()
        raise AssertionError("case-scoped collection was read before capability admission")


def test_stats_worker_rejects_before_reading_case_scope() -> None:
    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute_worker.query_stats_date_range_worker(
            case_id=_PoisonCaseValue(),
            db_path=_PoisonCaseValue(),
        )


@pytest.mark.parametrize("command", _DATA_ENGINE_ANALYSIS_COMMANDS)
def test_phase2_analysis_commands_have_no_direct_or_development_fallback(
    monkeypatch,
    tmp_path,
    command: str,
) -> None:
    marker = tmp_path / "must-not-exist"
    binary = tmp_path / "analytix-analysis-compute"
    binary.write_text(f"marker={marker}", encoding="utf-8")
    monkeypatch.setenv("ANALYTIX_ANALYSIS_COMPUTE_BIN", str(binary))
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)

    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute_runner.run_analysis_compute(
            _phase2_analysis_args(command, tmp_path / "case.duckdb")
        )

    assert not marker.exists()


def test_analysis_compute_rejects_before_iterating_arguments() -> None:
    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute_runner.run_analysis_compute(_PoisonCaseValue())


def test_analysis_compute_payload_and_snapshot_helpers_gate_before_temp_or_source_effects() -> None:
    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute.project_flow_skeleton_clusters(
            nodes=_PoisonCaseValue(),
            edges=_PoisonCaseValue(),
            request_context=_PoisonCaseValue(),
            base_projection=None,
            tile_size=1,
        )

    with pytest.raises(
        analysis_compute_runner.AnalysisComputeUnavailableError,
        match="^managed_process_capability_admission_unavailable$",
    ):
        analysis_compute._copy_readonly_db_snapshot(
            _PoisonCaseValue(),
            _PoisonCaseValue(),
        )


def test_cleaning_runtime_has_no_direct_or_development_fallback(monkeypatch, tmp_path) -> None:
    marker = tmp_path / "must-not-exist"
    binary = tmp_path / "analytix-cleaning-ops"
    binary.write_text(f"marker={marker}", encoding="utf-8")
    monkeypatch.setenv("ANALYTIX_CLEANING_OPS_BIN", str(binary))
    monkeypatch.delenv("ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS", raising=False)

    assert cleaning_native_runtime.build_native_cleaning_command(
        case_id=_PoisonCaseValue(),
        db_path=_PoisonCaseValue(),
        command=_PoisonCaseValue(),
        txn_file_ids=_PoisonCaseValue(),
        acc_file_ids=_PoisonCaseValue(),
        root=_PoisonCaseValue(),
    ) == []
    with pytest.raises(RuntimeError, match="^managed_process_capability_admission_unavailable$"):
        cleaning_native_runtime.run_native_cleaning_command(
            case_id=_PoisonCaseValue(),
            db_path=_PoisonCaseValue(),
            command=_PoisonCaseValue(),
            txn_file_ids=_PoisonCaseValue(),
            acc_file_ids=_PoisonCaseValue(),
            argv=_PoisonCaseValue(),
            root=_PoisonCaseValue(),
        )

    assert cleaning_native_runtime.warm_native_cleaning_worker(_PoisonCaseValue()) is None
    assert not marker.exists()


@pytest.mark.parametrize(
    ("response_code", "expected_code"),
    (
        ("data_engine_owner_conflict", "data_engine_owner_conflict"),
        ("external_process_holds_duckdb", "external_process_holds_duckdb"),
        ("secret-case.duckdb", "data_engine_error"),
    ),
)
def test_data_engine_client_preserves_only_allowlisted_engine_error_code(
    response_code: str,
    expected_code: str,
) -> None:
    response = {
        "request_id": "1",
        "ok": False,
        "error": {"code": response_code, "message": f"{response_code} raw"},
        "diagnostics": {"db_path": "case.duckdb", "owner_pid": 123},
    }
    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client._parse_data_engine_response(json.dumps(response), "1")

    assert raised.value.code == expected_code
    assert raised.value.message == data_engine_client._FIXED_ERROR_MESSAGES[expected_code]
    assert raised.value.diagnostics == {}
    assert raised.value.raw_error is None
    assert "case.duckdb" not in str(raised.value)
    assert raised.value.__cause__ is None
    assert raised.value.__context__ is None


@pytest.mark.parametrize(
    ("response_line", "expected_code"),
    (
        ("", "data_engine_empty_response"),
        ("{not json", "data_engine_invalid_response_json"),
        (json.dumps({"request_id": "wrong", "ok": True, "data": {}}), "data_engine_response_id_mismatch"),
    ),
)
def test_data_engine_client_protocol_errors_are_stable(response_line: str, expected_code: str) -> None:
    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        data_engine_client._parse_data_engine_response(response_line, "1")

    assert raised.value.code == expected_code
    if response_line:
        assert response_line not in str(raised.value)
    assert raised.value.__cause__ is None
    assert raised.value.__context__ is None


def test_data_engine_private_request_rejects_before_payload_or_process(tmp_path) -> None:
    marker = tmp_path / "private-request-inspected"
    poison = _PoisonCaseValue(marker)
    client = data_engine_client._DataEngineClient(poison)

    with pytest.raises(data_engine_client.DataEngineUnavailableError) as raised:
        client.request(
            command=poison,
            case_id=poison,
            db_path=poison,
            payload=poison,
            timeout=poison,
        )

    assert raised.value.code == _EXECUTION_CAPABILITY_BLOCKER
    assert not marker.exists()


@pytest.mark.parametrize(
    ("engine_code", "product_code", "message_fragment"),
    (
        ("data_engine_owner_conflict", "DATA_ENGINE_OWNER_CONFLICT", "另一个 Analytix 数据引擎"),
        ("external_process_holds_duckdb", "EXTERNAL_PROCESS_HOLDS_DUCKDB", "外部进程占用"),
    ),
)
def test_data_engine_product_error_maps_known_codes(
    engine_code: str,
    product_code: str,
    message_fragment: str,
) -> None:
    error = data_engine_client.DataEngineUnavailableError(
        "raw data engine error",
        code=engine_code,
        diagnostics={"db_path": "case.duckdb", "owner_pid": 123},
        raw_error={"code": engine_code, "message": "raw data engine error"},
    )

    mapped = data_engine_client.data_engine_product_error(error)

    assert mapped is not None
    assert mapped["status_code"] == 409
    assert mapped["code"] == product_code
    assert message_fragment in mapped["message"]
    assert mapped["details"] == {"data_engine_code": engine_code}


@pytest.mark.parametrize(
    ("engine_code", "product_code", "message_fragment"),
    (
        ("data_engine_owner_conflict", "DATA_ENGINE_OWNER_CONFLICT", "另一个 Analytix 数据引擎"),
        ("external_process_holds_duckdb", "EXTERNAL_PROCESS_HOLDS_DUCKDB", "外部进程占用"),
    ),
)
def test_stats_exception_error_maps_data_engine_errors(
    engine_code: str,
    product_code: str,
    message_fragment: str,
) -> None:
    engine_error = data_engine_client.DataEngineUnavailableError(
        "raw data engine error",
        code=engine_code,
        diagnostics={"db_path": "case.duckdb"},
        raw_error={"code": engine_code, "message": "raw data engine error"},
    )
    wrapped = RuntimeError("stats data engine failed")
    wrapped.__cause__ = engine_error

    response = stats_api._exception_error(wrapped)
    payload = json.loads(response.body.decode("utf-8"))

    assert response.status_code == 409
    assert payload["error"]["code"] == product_code
    assert payload["error"]["code"] != "INTERNAL_ERROR"
    assert message_fragment in payload["error"]["message"]
    assert payload["error"]["details"]["data_engine_code"] == engine_code


@pytest.mark.parametrize(
    ("engine_code", "product_code", "message_fragment"),
    (
        ("data_engine_owner_conflict", "DATA_ENGINE_OWNER_CONFLICT", "另一个 Analytix 数据引擎"),
        ("external_process_holds_duckdb", "EXTERNAL_PROCESS_HOLDS_DUCKDB", "外部进程占用"),
    ),
)
def test_cleaning_service_maps_data_engine_failure_payload(
    engine_code: str,
    product_code: str,
    message_fragment: str,
) -> None:
    engine_error = data_engine_client.DataEngineUnavailableError(
        "raw data engine error",
        code=engine_code,
        diagnostics={"db_path": "case.duckdb"},
        raw_error={"code": engine_code, "message": "raw data engine error"},
    )
    wrapped = RuntimeError("native cleaning clean-all data engine failed")
    wrapped.__cause__ = engine_error

    payload = CleaningService._job_error_payload(wrapped)

    assert payload["code"] == product_code
    assert payload["code"] != "INTERNAL_ERROR"
    assert message_fragment in payload["message"]
    assert payload["details"]["data_engine_code"] == engine_code


def test_product_direct_duckdb_helper_paths_are_guarded_or_allowlisted() -> None:
    app_root = Path(__file__).resolve().parents[1] / "app"
    closed_helper_paths = (
        Path("core/analysis_compute_runner.py"),
        Path("core/analysis_compute_worker.py"),
        Path("repositories/cleaning_native_runtime.py"),
    )
    duckdb_allowlist = {
        Path("core/db_engine.py"): "central DuckDBEngine implementation proxies product active DB sessions through data engine and guards direct Python opens",
    }
    helper_markers = (
        "analysis_compute_binary(",
        "ANALYTIX_ANALYSIS_COMPUTE_BIN",
        "analytix-analysis-compute",
        "ANALYTIX_CLEANING_OPS_BIN",
        "analytix-cleaning-ops",
        "native_cleaning_binary",
    )
    duckdb_offenders = []

    for path in app_root.rglob("*.py"):
        rel = path.relative_to(app_root)
        text = path.read_text(encoding="utf-8")
        if "duckdb.connect(" in text and rel not in duckdb_allowlist:
            duckdb_offenders.append(str(rel))

    assert duckdb_offenders == []
    for rel in closed_helper_paths:
        text = (app_root / rel).read_text(encoding="utf-8")
        assert "subprocess" not in text
        assert "ANALYTIX_ANALYSIS_COMPUTE_BIN" not in text
        assert "ANALYTIX_CLEANING_OPS_BIN" not in text
        assert "managed_process_capability_admission_unavailable" in text
        assert not any(
            marker in text
            for marker in helper_markers
            if marker.startswith("ANALYTIX_")
        )
    for reason in duckdb_allowlist.values():
        assert reason
