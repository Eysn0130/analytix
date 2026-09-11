from __future__ import annotations

import os
from pathlib import Path
from typing import Any, Dict, List, NoReturn, Optional, Sequence, Tuple

from app.core.execution_authority_health import (
    ExecutionAuthorityHealth,
    execution_authority_health,
    unadmitted_execution_capability_health,
)


_CLEANING_CAPABILITY_UNAVAILABLE = "managed_process_capability_admission_unavailable"


def _raise_cleaning_capability_unavailable() -> NoReturn:
    """Reject before reading case scope, database paths, argv, or env paths."""

    _cleaning_capability_health()
    raise RuntimeError(_CLEANING_CAPABILITY_UNAVAILABLE)


def _cleaning_capability_health() -> ExecutionAuthorityHealth:
    return unadmitted_execution_capability_health(execution_authority_health())


def repo_root() -> Path:
    return Path(__file__).resolve().parents[3]


def native_cleaning_bin_name() -> str:
    return "analytix-cleaning-ops.exe" if os.name == "nt" else "analytix-cleaning-ops"


def native_cleaning_binary_candidates(root: Path) -> Tuple[Path, ...]:
    """Executable discovery remains closed until capability admission exists."""

    return ()


def find_native_cleaning_binary(root: Optional[Path] = None) -> Optional[Path]:
    return None


def native_cleaning_binary_health(root: Optional[Path] = None) -> Dict[str, Any]:
    del root
    capability = _cleaning_capability_health()
    return {
        "native_cleaning_available": False,
        "native_cleaning_reason": capability.reason_code,
        "native_cleaning_bin": "",
        "native_cleaning_bin_source": "",
        "legacy_python_cleaning_allowed": False,
        "message": "Rust cleaning runtime is unavailable",
    }


def build_native_cleaning_command(
    *,
    case_id: str,
    db_path: Path,
    command: str,
    txn_file_ids: Sequence[str],
    acc_file_ids: Sequence[str] = (),
    root: Optional[Path] = None,
) -> List[str]:
    """Return no executable plan without reading case-scoped arguments."""

    return []


def warm_native_cleaning_worker(root: Optional[Path] = None) -> Optional[Dict[str, Any]]:
    return None


def run_native_cleaning_command(
    *,
    case_id: str,
    db_path: Path,
    command: str,
    txn_file_ids: Sequence[str],
    acc_file_ids: Sequence[str] = (),
    argv: Optional[Sequence[str]] = None,
    root: Optional[Path] = None,
) -> Optional[Dict[str, Any]]:
    _raise_cleaning_capability_unavailable()


__all__ = [
    "build_native_cleaning_command",
    "find_native_cleaning_binary",
    "native_cleaning_binary_candidates",
    "native_cleaning_binary_health",
    "native_cleaning_bin_name",
    "repo_root",
    "run_native_cleaning_command",
    "warm_native_cleaning_worker",
]
