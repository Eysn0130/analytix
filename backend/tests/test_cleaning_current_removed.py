from __future__ import annotations

from app.domain.ordinary_diagnostic_projection import project_cleaning_summary_diagnostic
from app.repositories import cleaning_native_plan, cleaning_repository, cleaning_run_setup


class _Connection:
    def __init__(self) -> None:
        self.closed = False

    def close(self) -> None:
        self.closed = True


def test_full_cleaning_run_never_reuses_previous_factual_summary(monkeypatch) -> None:
    connection = _Connection()
    setup = cleaning_run_setup.CleaningRunSetup(
        con=connection,
        cur=object(),
        import_marker="2026-07-20T00:00:00Z",
        scope_rows=7,
        scope_ids=["file-a"],
        active_steps=list(range(1, 11)),
        native_plan=cleaning_native_plan.NativeCleaningPlan(
            active_steps=tuple(range(1, 11)),
            clean_all=("native-clean-all",),
        ),
        active_step_set=set(range(1, 11)),
        step_total=10,
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_run_setup,
        "prepare_cleaning_run",
        lambda _executor, *, progress: setup,
    )

    pipeline_calls: list[str] = []

    class _NativeRunner:
        def __init__(self, **kwargs) -> None:
            self.con = kwargs["con"]
            self.cur = kwargs["cur"]

    monkeypatch.setattr(cleaning_repository.cleaning_native_runner, "CleaningNativeRunner", _NativeRunner)
    monkeypatch.setattr(
        cleaning_repository.cleaning_pipeline,
        "execute_cleaning_pipeline",
        lambda **_kwargs: pipeline_calls.append("executed"),
    )
    monkeypatch.setattr(
        cleaning_repository.cleaning_completion,
        "complete_cleaning_run",
        lambda _executor, **_kwargs: {"execution": "fresh"},
    )

    executor = cleaning_repository.CleaningExecutor(object(), "case-alpha")
    assert executor.run() == {"execution": "fresh"}
    assert pipeline_calls == ["executed"]
    assert connection.closed is True


def test_legacy_cleaning_current_marker_is_not_projected_as_current() -> None:
    projected = project_cleaning_summary_diagnostic(
        {
            "cleaning_short_circuit": True,
            "cleaning_short_circuit_reason": "cleaning_current",
        }
    )

    assert "cleaning_short_circuit" not in projected
    assert "cleaning_short_circuit_reason" not in projected
    assert projected["restricted_details_withheld"] is True
    assert projected["fact_answer_allowed"] is False
