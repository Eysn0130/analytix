package loop

import (
	"errors"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

// Both foreground and auxiliary producers require the same durable physical
// send pairs. The recorder functions retain each producer's existing event lane.
type providerDispatchGuard struct {
	pre, post, rejected, other func(domainmodel.PipelineStage) error
	pending                    bool
	pairs, pairStart           int
	started, closed            bool
}

func (g *providerDispatchGuard) stage(stage domainmodel.PipelineStage) error {
	switch stage.Stage {
	case "provider_admission_rejected":
		return g.rejected(stage)
	case "pre_send":
		if g.pending {
			return errors.New("provider pre-send pipeline stage is already pending")
		}
		if err := g.pre(stage); err != nil {
			return err
		}
		g.pending = true
		return nil
	case "post_send":
		if !g.pending {
			return errors.New("provider send pipeline stage pair is invalid")
		}
		if err := g.post(stage); err != nil {
			return err
		}
		g.pending = false
		g.pairs++
		return nil
	default:
		return g.other(stage)
	}
}

func (g *providerDispatchGuard) before(int) error {
	if g.pending {
		return providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}
	}
	if g.started && !g.closed {
		return providerPipelineContractError{reason: "provider retry followed an unclosed durable dispatch attempt"}
	}
	g.started, g.closed, g.pairStart = true, false, g.pairs
	return nil
}

func (g *providerDispatchGuard) after(_ int, providerErr error) error {
	if !g.started {
		return providerPipelineContractError{reason: "provider dispatch attempt was not opened"}
	}
	g.closed = true
	if g.pending {
		return providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}
	}
	if g.pairs != g.pairStart {
		return nil
	}
	if state, known := providerDispatchStateFromErrorV1(providerErr); known && state == domaincache.ProviderDispatchStateNotSent {
		return nil
	}
	return providerPipelineContractError{reason: "provider attempt lacks a durable send pipeline pair or trusted not-sent disposition"}
}

func (g *providerDispatchGuard) finish(stream ProviderStreamOutput, err error) (ProviderStreamOutput, error) {
	var contractErr error
	if g.pending {
		contractErr = providerPipelineContractError{reason: "provider durable send pipeline stage pair is incomplete"}
	} else if (g.started && !g.closed) || (err == nil && !g.started) {
		contractErr = providerPipelineContractError{reason: "provider completed without a closed durable dispatch attempt"}
	}
	if contractErr != nil {
		return sealFailedProviderStreamOutput(stream), errors.Join(err, ProviderStreamCallbackError{Err: contractErr})
	}
	return stream, err
}
