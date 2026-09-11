package contextepoch

import (
	"context"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	contextsourceport "analytix.local/runtime-go/internal/ports/contextsource"
)

func RestoreThreadState(ctx context.Context, thread map[string]any, threadID string, reader contextsourceport.Reader, at time.Time) (domaincontextepoch.State, bool, error) {
	state, hasState, err := StateFromThread(thread)
	if err != nil {
		return domaincontextepoch.State{}, false, err
	}
	if !hasState {
		baseEpoch := uint64(1)
		if securityContext, securityErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"]); securityErr == nil {
			baseEpoch = securityContext.ContextEpoch
		}
		state, err = DefaultState(threadID, baseEpoch, at)
		return state, true, err
	}
	recovered, err := Recover(ctx, state, reader, at)
	if err != nil {
		return domaincontextepoch.State{}, false, err
	}
	return recovered.State, recovered.Changed, nil
}
