package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestRuntimePublicationTraceMeasuresFinalDependencyWithoutCandidateContent(t *testing.T) {
	start := time.Now()
	result := apploop.RuntimeAgentLoopResult{AssistantText: "private candidate sentinel", LastDependencyAt: start.Add(30 * time.Millisecond), LastResult: domainmodel.Result{HostTiming: domainmodel.ProviderTiming{StartedAt: start, FirstContentAt: start.Add(time.Millisecond), LastContentAt: start.Add(10 * time.Millisecond), FinishedAt: start.Add(20 * time.Millisecond)}}}
	body := runtimePublicationTrace("thread-private-id", "turn-private-id", result, start.Add(35*time.Millisecond), appturn.PublicationTiming{ProjectionReadyAt: start.Add(40 * time.Millisecond), CommittedAt: start.Add(50 * time.Millisecond), DeliverableAt: start.Add(55 * time.Millisecond)})
	if strings.Contains(string(body), "private candidate sentinel") || strings.Contains(string(body), "thread-private-id") || strings.Contains(string(body), "turn-private-id") {
		t.Fatal("trace disclosed identity or candidate bytes")
	}
	var record struct {
		Data map[string]float64 `json:"data"`
	}
	if json.Unmarshal(body, &record) != nil || record.Data["publicationGapMs"] != 20 {
		t.Fatal("gap included model or previous tool wait", string(body))
	}
	if record.Data["sseDeliverableMs"] <= record.Data["publicationCommittedMs"] || record.Data["publicationCommittedMs"] <= record.Data["publicationProjectionReadyMs"] {
		t.Fatal("publication stages lost their order")
	}
	result.LastDependencyAt = time.Time{}
	body = runtimePublicationTrace("t", "u", result, start, appturn.PublicationTiming{CommittedAt: start})
	if strings.Contains(string(body), "publicationGapMs") {
		t.Fatal("missing dependency clock became zero duration")
	}
}
