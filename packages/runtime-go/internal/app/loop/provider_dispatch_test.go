package loop

import (
	"errors"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestProviderDispatchGuardRejectsUnpairedAndFailedDurableStages(t *testing.T) {
	for _, fault := range []string{"missing", "pre", "post", "double-pre", "post-without-pre"} {
		t.Run(fault, func(t *testing.T) {
			write := func(stage domainmodel.PipelineStage) error {
				if stage.Stage == fault+"_send" {
					return errors.New("durable write failed")
				}
				return nil
			}
			g := providerDispatchGuard{pre: write, post: write, rejected: write, other: write}
			if err := g.before(1); err != nil {
				t.Fatal(err)
			}
			var stageErr error
			switch fault {
			case "pre", "post", "double-pre":
				stageErr = g.stage(domainmodel.PipelineStage{Stage: "pre_send"})
				if fault == "post" {
					stageErr = g.stage(domainmodel.PipelineStage{Stage: "post_send"})
				}
				if fault == "double-pre" {
					stageErr = g.stage(domainmodel.PipelineStage{Stage: "pre_send"})
				}
			case "post-without-pre":
				stageErr = g.stage(domainmodel.PipelineStage{Stage: "post_send"})
			}
			if fault != "missing" && stageErr == nil {
				t.Fatal("invalid pipeline accepted")
			}
			if err := g.after(1, stageErr); err == nil {
				t.Fatal("incomplete dispatch admitted result")
			}
		})
	}
}
