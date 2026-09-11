use serde::Serialize;
use serde_json::{json, Value};
use std::time::Instant;

#[derive(Clone, Debug, Serialize)]
struct StatsQueryTiming {
    stage: String,
    duration_ms: f64,
}

#[derive(Clone, Debug, Default)]
pub(super) struct StatsQueryDiagnostics {
    stages: Vec<StatsQueryTiming>,
    total_ms: f64,
}

impl StatsQueryDiagnostics {
    pub(super) fn new() -> Self {
        Self::default()
    }

    pub(super) fn record_elapsed(&mut self, stage: &str, started_at: Instant) {
        self.stages.push(StatsQueryTiming {
            stage: stage.to_string(),
            duration_ms: elapsed_ms(started_at),
        });
    }

    pub(super) fn record_duration_ms(&mut self, stage: &str, duration_ms: f64) {
        self.stages.push(StatsQueryTiming {
            stage: stage.to_string(),
            duration_ms: round_ms(duration_ms),
        });
    }

    pub(super) fn finish_total(&mut self, started_at: Instant) {
        self.total_ms = elapsed_ms(started_at);
    }

    pub(super) fn to_json(&self) -> Value {
        json!({
            "total_ms": round_ms(self.total_ms),
            "stages": self.stages,
        })
    }
}

pub(super) fn elapsed_ms(started_at: Instant) -> f64 {
    round_ms(started_at.elapsed().as_secs_f64() * 1000.0)
}

fn round_ms(value: f64) -> f64 {
    (value * 1000.0).round() / 1000.0
}
