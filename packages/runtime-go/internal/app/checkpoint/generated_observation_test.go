package checkpoint

import (
	"context"
	"testing"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointfileport "analytix.local/runtime-go/internal/ports/checkpointfile"
)

type generatedObserverFixture struct {
	checkpointfileport.Observer
	calls []string
}

func (o *generatedObserverFixture) Observe(context.Context, string, checkpointfileport.PathAuthority, string) domaincheckpoint.ObservedOperationPathV2 {
	o.calls = append(o.calls, "text")
	return domaincheckpoint.ObservedOperationPathV2{ObservationStatus: "exact"}
}
func (o *generatedObserverFixture) ObserveRelative(context.Context, string, checkpointfileport.PathAuthority) domaincheckpoint.ObservedOperationPathV2 {
	o.calls = append(o.calls, "text-relative")
	return domaincheckpoint.ObservedOperationPathV2{ObservationStatus: "exact"}
}
func (o *generatedObserverFixture) ObserveGenerated(context.Context, string, checkpointfileport.PathAuthority, string) domaincheckpoint.ObservedOperationPathV2 {
	o.calls = append(o.calls, "generated")
	return domaincheckpoint.ObservedOperationPathV2{ObservationStatus: "exact"}
}
func (o *generatedObserverFixture) ObserveGeneratedRelative(context.Context, string, checkpointfileport.PathAuthority) domaincheckpoint.ObservedOperationPathV2 {
	o.calls = append(o.calls, "generated-relative")
	return domaincheckpoint.ObservedOperationPathV2{ObservationStatus: "exact"}
}

func TestGeneratedObservationUsesSameBoundedLaneForLiveAndRestart(t *testing.T) {
	o := &generatedObserverFixture{}
	service := OperationService{Observer: o}
	intent := domaincheckpoint.OperationGroupIntentV2{ToolName: "generate_office_document", Paths: []domaincheckpoint.OperationPathV2{{BeforeExisted: false, ExpectedAfterExisted: true}}}
	for _, relative := range []bool{false, true} {
		service.observeOperationPath(intent, relative, context.Background(), "/synthetic", checkpointfileport.PathAuthority{}, "/synthetic/report.docx")
	}
	if len(o.calls) != 2 || o.calls[0] != "generated" || o.calls[1] != "generated-relative" {
		t.Fatal("live/restart observation mismatch")
	}
	intent.Paths[0].BeforeExisted = true
	if got := service.observeOperationPath(intent, false, context.Background(), "/synthetic", checkpointfileport.PathAuthority{}, "/synthetic/report.docx"); got.ObservationStatus != "unavailable" || len(o.calls) != 2 {
		t.Fatal("overwrite acquired binary creation lane")
	}
	intent.ToolName = "write_file"
	service.observeOperationPath(intent, false, context.Background(), "/synthetic", checkpointfileport.PathAuthority{}, "/synthetic/report.docx")
	if o.calls[2] != "text" {
		t.Fatal("text observer policy changed")
	}
}
