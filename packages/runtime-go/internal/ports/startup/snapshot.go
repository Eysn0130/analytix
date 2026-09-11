package startup

import (
	"context"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

type SnapshotReader interface {
	CaptureManagedSnapshotV1(context.Context) (domainstartup.ManagedSnapshotV1, error)
}

type PersistenceRootsV1 struct {
	DataDir    string
	DurableDir string
}

type SemanticSimulationV1 func(context.Context, PersistenceRootsV1) error

type PreparedSemanticPlanV1 interface {
	Plan() domainstartup.SemanticStartupPlanV1
	Apply(context.Context) error
	Close() error
}

type SemanticPlanBuilderV1 interface {
	// Recover accepts only the already validated exact current-run security
	// configuration digest; a journal from any other configuration fails closed.
	Recover(context.Context, string) error
	Prepare(
		context.Context,
		domainstartup.ReadOnlyStartupBaselineV1,
		string,
		SemanticSimulationV1,
	) (PreparedSemanticPlanV1, error)
}
