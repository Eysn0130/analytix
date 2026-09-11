from __future__ import annotations

import multiprocessing
import os
import threading
from pathlib import Path

import pytest

from app.utils.fs import private_exclusive_file_lock


def _hold_private_lock_in_spawned_process(lock_path: str, acquired, release) -> None:
    with private_exclusive_file_lock(Path(lock_path)):
        acquired.set()
        release.wait(timeout=5)


def test_private_exclusive_file_lock_has_bounded_fail_closed_wait(tmp_path: Path) -> None:
    lock_path = tmp_path / "leases" / "case.lock"
    finished = threading.Event()
    failures: list[str] = []

    def contender() -> None:
        try:
            with private_exclusive_file_lock(lock_path, timeout_seconds=0.05):
                failures.append("unexpected_lock_acquisition")
        except OSError as exc:
            failures.append(str(exc))
        finally:
            finished.set()

    with private_exclusive_file_lock(lock_path):
        thread = threading.Thread(target=contender, daemon=True)
        thread.start()
        assert finished.wait(timeout=1)
    thread.join(timeout=1)

    assert failures == ["private_artifact_lock_timeout"]


@pytest.mark.parametrize("timeout_seconds", (True, 0, -1, 61, float("nan"), float("inf"), float("-inf")))
def test_private_exclusive_file_lock_rejects_invalid_deadlines(
    tmp_path: Path,
    timeout_seconds: object,
) -> None:
    with pytest.raises(OSError, match="^private_artifact_lock_invalid$"):
        with private_exclusive_file_lock(
            tmp_path / "case.lock",
            timeout_seconds=timeout_seconds,  # type: ignore[arg-type]
        ):
            pytest.fail("invalid deadline acquired publication authority")


@pytest.mark.skipif(os.name == "nt", reason="POSIX flock authority is intentionally unavailable on Windows")
def test_private_exclusive_file_lock_serializes_across_processes(tmp_path: Path) -> None:
    context = multiprocessing.get_context("spawn")
    acquired = context.Event()
    release = context.Event()
    lock_path = tmp_path / "leases" / "case.lock"
    holder = context.Process(
        target=_hold_private_lock_in_spawned_process,
        args=(str(lock_path), acquired, release),
    )
    holder.start()
    try:
        assert acquired.wait(timeout=5), "spawned holder did not acquire the lease"
        with pytest.raises(OSError, match="^private_artifact_lock_timeout$"):
            with private_exclusive_file_lock(lock_path, timeout_seconds=0.05):
                pytest.fail("cross-process contender acquired the held lease")
    finally:
        release.set()
        holder.join(timeout=5)
        if holder.is_alive():
            holder.terminate()
            holder.join(timeout=5)
    assert holder.exitcode == 0
