from __future__ import annotations

from functools import lru_cache
from pathlib import Path
from typing import NoReturn, Optional

from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)


class AnalysisComputeUnavailableError(RuntimeError):
    pass


_ANALYSIS_COMPUTE_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"
_REQUIRED_ANALYSIS_COMPUTE_COMMANDS = (
    "materialize-txn-daily",
    "query-stats-rows",
    "query-stats-tree",
    "query-stats-date-range",
    "query-stats-txn-rows",
    "stats-query-worker",
    "query-chart-dashboard",
    "query-chart-detail-rows",
    "query-chart-flow",
    "query-flow-focus-graph",
    "project-flow-layout-network-plan",
    "project-flow-layout-network-community-quality",
    "project-flow-graph-render-plan",
    "materialize-rule-txn-index",
    "materialize-rule-pattern-index",
)


def _raise_analysis_compute_capability_unavailable() -> NoReturn:
    """Reject before inspecting case arguments or executable configuration.

    The shared managed-process authority does not yet issue a versioned,
    analysis-compute-specific capability admission. A global authority status
    or a configured path therefore cannot authorize this consumer, and there
    is no direct-process development fallback.
    """

    _analysis_compute_capability_health()
    raise AnalysisComputeUnavailableError(_ANALYSIS_COMPUTE_CAPABILITY_UNAVAILABLE)


def _analysis_compute_capability_health() -> ExecutionAuthorityHealth:
    return unadmitted_execution_capability_health(execution_authority_health())


def require_analysis_compute_capability() -> None:
    """Admit a consumer before it serializes case data or creates temp files."""

    _raise_analysis_compute_capability_unavailable()


def run_analysis_compute(
    args: list[str],
    *,
    include_diagnostics: bool = False,
) -> Optional[dict]:
    _raise_analysis_compute_capability_unavailable()


def analysis_compute_binary_health() -> dict:
    capability = _analysis_compute_capability_health()
    return {
        "analysis_compute_available": False,
        "analysis_compute_reason": capability.reason_code,
        "analysis_compute_bin": "",
        "analysis_compute_required_commands": list(_REQUIRED_ANALYSIS_COMPUTE_COMMANDS),
    }


@lru_cache(maxsize=1)
def analysis_compute_binary() -> Optional[Path]:
    """Compatibility projection; executable discovery requires admission."""

    return None


@lru_cache(maxsize=1)
def repo_root() -> Path:
    return Path(__file__).resolve().parents[3]
