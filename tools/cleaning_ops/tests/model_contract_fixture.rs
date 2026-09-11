use analytix_cleaning_ops::export_realtime::{
    apply_export_realtime_state, get_export_realtime_terminal_action,
};
use analytix_cleaning_ops::step_impact::project_cleaning_step_impact;
use serde_json::Value;

mod model_contract;

use model_contract::{
    array_at, at, bool_at, export_realtime_projection_at, export_realtime_projection_from_event,
    export_state_at, export_terminal_action_at, int_at, read_contract_fixture,
    step_impact_summaries, text_at,
};

#[test]
fn reads_cleaning_model_contract_fixture() {
    let fixture = read_contract_fixture();
    assert_eq!(int_at(&fixture, &["version"]), 1);

    assert_cleaning_realtime_contract(at(&fixture, &["cleaningRealtime"]));
    assert_export_realtime_contract(at(&fixture, &["exportRealtime"]));
    assert_step_impact_contract(at(&fixture, &["stepImpact"]));
}

#[test]
fn projects_step_impact_from_shared_contract_fixture() {
    let fixture = read_contract_fixture();
    let section = at(&fixture, &["stepImpact"]);
    let impact = project_cleaning_step_impact(&step_impact_summaries(section));
    let expected = at(section, &["expectedImpact"]);

    assert_eq!(
        impact.txn_step_affected,
        int_at(expected, &["txnStepAffected"])
    );
    assert_eq!(
        impact.account_step_affected,
        int_at(expected, &["accountStepAffected"])
    );
    assert_eq!(impact.step_hits, int_at(expected, &["stepHits"]));
}

#[test]
fn reduces_export_realtime_from_shared_contract_fixture() {
    let fixture = read_contract_fixture();
    let section = at(&fixture, &["exportRealtime"]);
    let initial_state = export_state_at(section, &["initialState"]);

    let progress = export_realtime_projection_from_event(at(section, &["progressEvent"]));
    assert_eq!(
        progress,
        export_realtime_projection_at(section, &["expectedProgressProjection"])
    );
    assert_eq!(
        apply_export_realtime_state(&initial_state, &progress),
        export_state_at(section, &["expectedProgressState"])
    );
    let mismatched = analytix_cleaning_ops::export_realtime::ExportState {
        job_id: "other-export-job".to_string(),
        ..initial_state.clone()
    };
    assert_eq!(
        apply_export_realtime_state(&mismatched, &progress),
        mismatched
    );

    let completed = export_realtime_projection_from_event(at(section, &["completedEvent"]));
    assert_eq!(
        apply_export_realtime_state(&initial_state, &completed),
        export_state_at(section, &["expectedCompletedState"])
    );
    assert_eq!(
        get_export_realtime_terminal_action(&completed),
        Some(export_terminal_action_at(
            section,
            &["expectedCompletedTerminalAction"]
        ))
    );

    let failed = export_realtime_projection_from_event(at(section, &["failedEvent"]));
    assert_eq!(
        apply_export_realtime_state(&initial_state, &failed),
        export_state_at(section, &["expectedFailedState"])
    );
    assert_eq!(
        get_export_realtime_terminal_action(&failed),
        Some(export_terminal_action_at(
            section,
            &["expectedFailedTerminalAction"]
        ))
    );

    let canceled = export_realtime_projection_from_event(at(section, &["canceledEvent"]));
    assert_eq!(
        apply_export_realtime_state(
            &export_state_at(section, &["stateWithPreviousError"]),
            &canceled
        ),
        export_state_at(section, &["expectedCanceledState"])
    );
    assert_eq!(
        get_export_realtime_terminal_action(&canceled),
        Some(export_terminal_action_at(
            section,
            &["expectedCanceledTerminalAction"]
        ))
    );

    let completed_without_path =
        export_realtime_projection_from_event(at(section, &["completedWithoutPathEvent"]));
    assert_eq!(
        get_export_realtime_terminal_action(&completed_without_path),
        Some(export_terminal_action_at(
            section,
            &["expectedSyncTerminalAction"]
        ))
    );

    let failed_without_message =
        export_realtime_projection_from_event(at(section, &["failedWithoutMessageEvent"]));
    assert_eq!(
        get_export_realtime_terminal_action(&failed_without_message),
        Some(export_terminal_action_at(
            section,
            &["expectedSyncTerminalAction"]
        ))
    );
}

fn assert_cleaning_realtime_contract(section: &Value) {
    assert_eq!(text_at(section, &["activeCaseId"]), "case-1");
    assert_eq!(text_at(section, &["otherCaseId"]), "case-2");

    let progress_event = at(section, &["progressEvent"]);
    let progress_projection = at(section, &["expectedProgressProjection"]);
    assert_eq!(
        text_at(progress_projection, &["eventName"]),
        text_at(progress_event, &["event"])
    );
    assert_eq!(
        text_at(progress_projection, &["jobId"]),
        text_at(progress_event, &["job_id"])
    );
    assert_eq!(
        int_at(progress_projection, &["liveEvent", "sequence"]),
        int_at(progress_event, &["sequence"])
    );
    assert_eq!(
        text_at(progress_projection, &["liveEvent", "level"]),
        text_at(progress_event, &["type"])
    );
    assert_eq!(
        text_at(progress_projection, &["liveEvent", "message"]),
        text_at(progress_event, &["payload", "message"])
    );
    assert_eq!(
        int_at(progress_projection, &["liveEvent", "progress"]),
        int_at(progress_event, &["payload", "progress"])
    );
    assert_eq!(
        int_at(progress_projection, &["liveEvent", "step"]),
        int_at(progress_event, &["payload", "step"])
    );
    assert_eq!(bool_at(progress_projection, &["shouldRefreshJobs"]), false);
    assert_eq!(
        bool_at(progress_projection, &["shouldRefreshContext"]),
        false
    );

    let completed_event = at(section, &["completedEvent"]);
    let completed_projection = at(section, &["expectedCompletedProjection"]);
    assert_eq!(
        int_at(completed_projection, &["completedCleanedRows"]),
        int_at(completed_event, &["payload", "counters", "cleaned_rows"])
    );
    assert_eq!(bool_at(completed_projection, &["shouldRefreshJobs"]), true);
    assert_eq!(
        bool_at(completed_projection, &["shouldRefreshContext"]),
        true
    );

    let previous_jobs = array_at(section, &["previousJobs"]);
    let completed_jobs = array_at(section, &["expectedCompletedJobs"]);
    assert_eq!(
        text_at(&completed_jobs[0], &["job_id"]),
        text_at(&previous_jobs[0], &["job_id"])
    );
    assert_eq!(text_at(&completed_jobs[0], &["status"]), "succeeded");
    assert_eq!(
        int_at(&completed_jobs[0], &["progress"]),
        int_at(completed_event, &["payload", "progress"])
    );
    assert_eq!(
        int_at(&completed_jobs[0], &["cleaned_rows"]),
        int_at(completed_projection, &["completedCleanedRows"])
    );
    assert_eq!(
        text_at(&completed_jobs[0], &["updated_at"]),
        text_at(completed_event, &["timestamp"])
    );
    assert_eq!(completed_jobs[1], previous_jobs[1]);

    let canceled_event = at(section, &["canceledEvent"]);
    let canceled_jobs = array_at(section, &["expectedCanceledJobs"]);
    assert_eq!(
        text_at(canceled_event, &["payload", "code"]),
        "JOB_CANCELED"
    );
    assert_eq!(text_at(&canceled_jobs[0], &["status"]), "canceled");
    assert_eq!(
        int_at(&canceled_jobs[0], &["progress"]),
        int_at(&previous_jobs[0], &["progress"])
    );
    assert_eq!(
        int_at(&canceled_jobs[0], &["cleaned_rows"]),
        int_at(&previous_jobs[0], &["cleaned_rows"])
    );
    assert_eq!(
        text_at(&canceled_jobs[0], &["updated_at"]),
        text_at(canceled_event, &["timestamp"])
    );
}

fn assert_export_realtime_contract(section: &Value) {
    assert_eq!(text_at(section, &["activeCaseId"]), "case-1");
    assert_eq!(text_at(section, &["otherCaseId"]), "case-2");

    let progress_event = at(section, &["progressEvent"]);
    let progress_projection = at(section, &["expectedProgressProjection"]);
    let initial_state = at(section, &["initialState"]);
    let progress_state = at(section, &["expectedProgressState"]);
    assert_eq!(
        text_at(progress_projection, &["eventName"]),
        text_at(progress_event, &["event"])
    );
    assert_eq!(
        text_at(progress_projection, &["jobId"]),
        text_at(progress_event, &["job_id"])
    );
    assert_eq!(
        int_at(progress_projection, &["progress"]),
        int_at(progress_event, &["payload", "progress"])
    );
    assert_eq!(text_at(progress_state, &["status"]), "running");
    assert_eq!(
        int_at(progress_state, &["progress"]),
        int_at(progress_projection, &["progress"])
    );
    assert_eq!(
        text_at(progress_state, &["message"]),
        text_at(progress_projection, &["payloadMessage"])
    );
    assert_eq!(
        text_at(progress_state, &["error"]),
        text_at(initial_state, &["error"])
    );

    let completed_event = at(section, &["completedEvent"]);
    let completed_state = at(section, &["expectedCompletedState"]);
    let completed_action = at(section, &["expectedCompletedTerminalAction"]);
    assert_eq!(text_at(completed_state, &["status"]), "done");
    assert_eq!(int_at(completed_state, &["progress"]), 100);
    assert_eq!(
        text_at(completed_state, &["outputPath"]),
        text_at(completed_event, &["payload", "output_path"])
    );
    assert_eq!(text_at(completed_action, &["type"]), "notify");
    assert_eq!(text_at(completed_action, &["status"]), "done");
    assert_eq!(
        text_at(completed_action, &["outputPath"]),
        text_at(completed_state, &["outputPath"])
    );

    let failed_event = at(section, &["failedEvent"]);
    let failed_state = at(section, &["expectedFailedState"]);
    let failed_action = at(section, &["expectedFailedTerminalAction"]);
    assert_eq!(text_at(failed_event, &["payload", "code"]), "WRITE_FAILED");
    assert_eq!(text_at(failed_state, &["status"]), "failed");
    assert_eq!(
        text_at(failed_state, &["error"]),
        text_at(failed_event, &["payload", "message"])
    );
    assert_eq!(
        text_at(failed_action, &["errorText"]),
        text_at(failed_state, &["error"])
    );

    let canceled_event = at(section, &["canceledEvent"]);
    let state_with_previous_error = at(section, &["stateWithPreviousError"]);
    let canceled_state = at(section, &["expectedCanceledState"]);
    let canceled_action = at(section, &["expectedCanceledTerminalAction"]);
    assert_eq!(
        text_at(canceled_event, &["payload", "code"]),
        "JOB_CANCELED"
    );
    assert_eq!(text_at(canceled_state, &["status"]), "canceled");
    assert_eq!(text_at(canceled_state, &["message"]), "导出已取消");
    assert_eq!(
        text_at(canceled_state, &["error"]),
        text_at(state_with_previous_error, &["error"])
    );
    assert_eq!(text_at(canceled_action, &["status"]), "canceled");

    assert_eq!(
        text_at(at(section, &["expectedSyncTerminalAction"]), &["type"]),
        "sync"
    );
}

fn assert_step_impact_contract(section: &Value) {
    let summaries = array_at(section, &["summaries"]);
    let positive_steps = array_at(section, &["expectedPositiveSteps"]);
    let impact = at(section, &["expectedImpact"]);

    let projected = project_cleaning_step_impact(&step_impact_summaries(section));
    assert_eq!(
        projected.txn_step_affected,
        int_at(impact, &["txnStepAffected"])
    );
    assert_eq!(
        projected.account_step_affected,
        int_at(impact, &["accountStepAffected"])
    );
    assert_eq!(projected.step_hits, int_at(impact, &["stepHits"]));
    assert_eq!(positive_steps.len() as i64, projected.step_hits);

    for (summary, positive_step) in summaries.iter().zip(positive_steps) {
        assert_eq!(int_at(positive_step, &["step"]), int_at(summary, &["step"]));
        assert_eq!(
            text_at(positive_step, &["description"]),
            text_at(summary, &["description"])
        );
        assert_eq!(
            int_at(positive_step, &["affectedRows"]),
            int_at(summary, &["affected_rows"])
        );
    }
}
