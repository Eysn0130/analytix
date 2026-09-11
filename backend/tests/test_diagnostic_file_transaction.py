from __future__ import annotations

import os

import pytest

from app.core.diagnostic_file_transaction import (
    DiagnosticFileTransactionError,
    recover_diagnostic_file_transaction,
    replace_diagnostic_file_transactionally,
)


_OLD = b'{"version":1,"diagnostic":"TRACEBACK 6222020202020202020 /private/case.csv"}'
_NEW = b'{"tasks":[],"version":2}'
_POINTS = (
    "stage_fsynced",
    "intent_fsynced",
    "previous_moved",
    "target_visible",
    "visible_fsynced",
    "settled",
)


class _Crash(BaseException):
    pass


@pytest.mark.parametrize("point", _POINTS)
def test_every_crash_cut_resumes_to_one_verified_generation(tmp_path, point: str) -> None:
    target = tmp_path / "task-store.json"
    target.write_bytes(_OLD)

    def crash(observed: str) -> None:
        if observed == point:
            raise _Crash(point)

    with pytest.raises(_Crash):
        replace_diagnostic_file_transactionally(
            target,
            _NEW,
            surface="task_store",
            max_bytes=4096,
            fault_hook=crash,
        )

    recover_diagnostic_file_transaction(
        target,
        surface="task_store",
        max_bytes=4096,
    )
    assert target.read_bytes() in {_OLD, _NEW}
    replace_diagnostic_file_transactionally(
        target,
        _NEW,
        surface="task_store",
        max_bytes=4096,
    )
    assert target.read_bytes() == _NEW
    assert b"6222020202020202020" not in target.read_bytes()
    assert not list(tmp_path.glob(".task-store.json.analytix-diagnostic-v1.stage"))
    assert not list(tmp_path.glob(".task-store.json.analytix-diagnostic-v1.previous"))
    assert not list(tmp_path.glob(".task-store.json.analytix-diagnostic-v1.journal"))

    # Recovery is idempotent and cannot recreate the rejected generation.
    recover_diagnostic_file_transaction(
        target,
        surface="task_store",
        max_bytes=4096,
    )
    assert target.read_bytes() == _NEW


def test_tampered_staged_generation_rolls_back_exact_previous_bytes(tmp_path) -> None:
    target = tmp_path / "task-store.json"
    target.write_bytes(_OLD)

    def crash(point: str) -> None:
        if point == "previous_moved":
            raise _Crash(point)

    with pytest.raises(_Crash):
        replace_diagnostic_file_transactionally(
            target,
            _NEW,
            surface="task_store",
            max_bytes=4096,
            fault_hook=crash,
        )

    stage = tmp_path / ".task-store.json.analytix-diagnostic-v1.stage"
    stage.write_bytes(b"tampered")
    recover_diagnostic_file_transaction(
        target,
        surface="task_store",
        max_bytes=4096,
    )
    assert target.read_bytes() == _OLD
    assert not stage.exists()


def test_symlink_hardlink_and_corrupt_journal_fail_closed(tmp_path) -> None:
    real = tmp_path / "real.json"
    real.write_bytes(_OLD)
    symlink = tmp_path / "task-store.json"
    symlink.symlink_to(real)
    with pytest.raises(DiagnosticFileTransactionError):
        recover_diagnostic_file_transaction(
            symlink,
            surface="task_store",
            max_bytes=4096,
        )

    symlink.unlink()
    os.link(real, symlink)
    with pytest.raises(DiagnosticFileTransactionError):
        recover_diagnostic_file_transaction(
            symlink,
            surface="task_store",
            max_bytes=4096,
        )

    symlink.unlink()
    symlink.write_bytes(_OLD)
    journal = tmp_path / ".task-store.json.analytix-diagnostic-v1.journal"
    journal.write_bytes(b'{"phase":"visible","duplicate":1,"duplicate":2}')
    with pytest.raises(DiagnosticFileTransactionError):
        recover_diagnostic_file_transaction(
            symlink,
            surface="task_store",
            max_bytes=4096,
        )
    assert symlink.read_bytes() == _OLD
