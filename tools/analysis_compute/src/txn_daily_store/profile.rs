use std::collections::BTreeMap;
use std::time::Instant;

#[derive(Default)]
pub(super) struct PhaseProfile {
    phases_s: BTreeMap<String, f64>,
}

impl PhaseProfile {
    pub(super) fn record_since(&mut self, phase: &str, started_at: Instant) {
        let elapsed = round_seconds(started_at.elapsed().as_secs_f64());
        if elapsed <= 0.0 {
            return;
        }
        *self.phases_s.entry(phase.to_string()).or_insert(0.0) += elapsed;
    }

    pub(super) fn extend(&mut self, other: PhaseProfile) {
        for (phase, elapsed) in other.phases_s {
            *self.phases_s.entry(phase).or_insert(0.0) += elapsed;
        }
    }

    pub(super) fn sum(&self, phases: &[&str]) -> f64 {
        phases
            .iter()
            .map(|phase| self.phases_s.get(*phase).copied().unwrap_or(0.0))
            .sum()
    }

    pub(super) fn total(&self) -> f64 {
        self.phases_s.values().sum()
    }

    pub(super) fn into_phases(self) -> BTreeMap<String, f64> {
        self.phases_s
    }
}

pub(super) fn round_seconds(value: f64) -> f64 {
    (value * 1_000_000.0).round() / 1_000_000.0
}
