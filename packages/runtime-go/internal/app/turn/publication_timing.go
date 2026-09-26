package turn

import "time"

// PublicationTiming is an observation of successful mutations in this process.
// It is not part of any sealed authority, persisted record, or replay contract.
// A zero commit time means no successful CAS was observed by this invocation.
type PublicationTiming struct {
	ProjectionReadyAt time.Time
	CommittedAt       time.Time
	DeliverableAt     time.Time
}
