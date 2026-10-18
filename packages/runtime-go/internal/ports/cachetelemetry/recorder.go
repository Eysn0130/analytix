package cachetelemetry

import (
	"context"
	"time"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// AttemptRegistrationInputV1 contains process-local raw bytes only long enough
// for the durable service to derive installation-keyed HMACs. Implementations
// must not persist or expose these byte slices. PublicAttribution is a separate
// host-approved value whose explicit public labels may be signed into the intent.
type AttemptRegistrationInputV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	// OrdinaryEffect selects the permanent ordinary Agent admission rule for
	// this provider send. False preserves strict case execution validation.
	// This process-local bit is not persisted or exposed.
	OrdinaryEffect    bool
	UsageSource       domaincachetelemetry.ProviderUsageSourceV1
	ChildRunID        []byte
	Channel           domaincachetelemetry.ProviderChannelV1
	LogicalSequence   uint64
	OuterAttempt      uint32
	LaneCallSequence  uint32
	PhysicalAttempt   uint32
	ProviderFamily    domaincachetelemetry.ProviderFamilyV1
	EndpointFormat    domaincachetelemetry.EndpointFormatV1
	Model             []byte
	Endpoint          []byte
	WireBody          []byte
	WireHeaders       []byte
	CredentialScope   []byte
	ProviderConfig    []byte
	PublicAttribution domaincachetelemetry.ProviderPublicAttributionV1
	StepStartedAt     time.Time
	StartedAt         time.Time
}

type AttemptHandleV1 struct {
	Intent domaincachetelemetry.ProviderAttemptIntentV1
}

type AttemptSettlementInputV1 struct {
	Handle         AttemptHandleV1
	DispatchState  domaincachetelemetry.ProviderDispatchStateV1
	Status         domaincachetelemetry.ProviderCallStatusV1
	Usage          domaincachetelemetry.ProviderUsageV1
	SafeReasonCode string
	FirstTokenAt   time.Time
	SettledAt      time.Time
}

type Recorder interface {
	BeginAttempt(context.Context, AttemptRegistrationInputV1) (AttemptHandleV1, error)
	SettleAttempt(context.Context, AttemptSettlementInputV1) error
}
