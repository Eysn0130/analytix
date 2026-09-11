package server

import (
	"errors"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func emitTestDurableProviderPipelinePairV1(request domainmodel.Request) error {
	if request.OnPipelineStage == nil {
		return errors.New("test provider durable pipeline callback is unavailable")
	}
	preSendAt := time.Now().UTC()
	if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "pre_send", At: preSendAt}); err != nil {
		return err
	}
	return request.OnPipelineStage(domainmodel.PipelineStage{Stage: "post_send", At: preSendAt.Add(time.Millisecond)})
}
