#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExportStateStatus {
    Idle,
    Running,
    Done,
    Failed,
    Canceled,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExportState {
    pub job_id: String,
    pub kind: String,
    pub status: ExportStateStatus,
    pub progress: i64,
    pub output_path: String,
    pub error: String,
    pub message: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExportRealtimeProjection {
    pub event_name: String,
    pub job_id: String,
    pub payload_message: String,
    pub payload_code: String,
    pub payload_output_path: String,
    pub progress: Option<i64>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ExportRealtimeTerminalAction {
    Notify {
        status: ExportStateStatus,
        output_path: String,
        error_text: String,
    },
    Sync,
}

pub fn apply_export_realtime_state(
    previous: &ExportState,
    update: &ExportRealtimeProjection,
) -> ExportState {
    if previous.job_id != update.job_id {
        return previous.clone();
    }

    match update.event_name.as_str() {
        "export.job.progress" => ExportState {
            status: ExportStateStatus::Running,
            progress: update.progress.unwrap_or(previous.progress),
            message: first_non_empty(&update.payload_message, &previous.message, "导出进行中"),
            ..previous.clone()
        },
        "export.job.completed" => ExportState {
            status: ExportStateStatus::Done,
            progress: 100,
            output_path: first_non_empty(&update.payload_output_path, &previous.output_path, ""),
            message: first_non_empty(&update.payload_message, "", "已完成导出"),
            ..previous.clone()
        },
        "export.job.failed" if update.payload_code == "JOB_CANCELED" => ExportState {
            status: ExportStateStatus::Canceled,
            message: "导出已取消".to_string(),
            ..previous.clone()
        },
        "export.job.failed" => ExportState {
            status: ExportStateStatus::Failed,
            message: first_non_empty(&update.payload_message, "", "导出失败"),
            error: first_non_empty(&update.payload_message, &previous.error, ""),
            ..previous.clone()
        },
        _ => previous.clone(),
    }
}

pub fn get_export_realtime_terminal_action(
    update: &ExportRealtimeProjection,
) -> Option<ExportRealtimeTerminalAction> {
    match update.event_name.as_str() {
        "export.job.completed" if !update.payload_output_path.is_empty() => {
            Some(ExportRealtimeTerminalAction::Notify {
                status: ExportStateStatus::Done,
                output_path: update.payload_output_path.clone(),
                error_text: String::new(),
            })
        }
        "export.job.completed" => Some(ExportRealtimeTerminalAction::Sync),
        "export.job.failed" if update.payload_code == "JOB_CANCELED" => {
            Some(ExportRealtimeTerminalAction::Notify {
                status: ExportStateStatus::Canceled,
                output_path: String::new(),
                error_text: String::new(),
            })
        }
        "export.job.failed" if !update.payload_message.is_empty() => {
            Some(ExportRealtimeTerminalAction::Notify {
                status: ExportStateStatus::Failed,
                output_path: String::new(),
                error_text: update.payload_message.clone(),
            })
        }
        "export.job.failed" => Some(ExportRealtimeTerminalAction::Sync),
        _ => None,
    }
}

fn first_non_empty(primary: &str, secondary: &str, fallback: &str) -> String {
    if !primary.is_empty() {
        primary.to_string()
    } else if !secondary.is_empty() {
        secondary.to_string()
    } else {
        fallback.to_string()
    }
}
