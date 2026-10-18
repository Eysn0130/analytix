package scheduleadmission

import (
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
	"encoding/json"
	"strings"
	"testing"
)

func fixtureV1() RequestV1 {
	return RequestV1{1, "submit", "task-1", OriginV1{"thread-1", "turn-1", toolidentity.MustHostToolCallIDV1("schedule-origin"), "grant-1"}, "a8b6850e-10e3-4a54-a80c-140899708b67", "2026-10-12T01:00:00Z", "exact frozen prompt", ExecutionV1{}}
}
func TestScheduleAdmissionClosedWireAndIdentityV1(t *testing.T) {
	r := fixtureV1()
	b, _ := json.Marshal(r)
	parsed, e := DecodeRequestV1(b)
	if e != nil || InputDigestV1(parsed) != InputDigestV1(r) {
		t.Fatal(e)
	}
	q := r
	q.Kind = "query"
	if InputDigestV1(q) != InputDigestV1(r) {
		t.Fatal("query changed frozen identity")
	}
	model := "other"
	q.Execution.Model = &model
	if InputDigestV1(q) == InputDigestV1(r) {
		t.Fatal("explicit selection not bound")
	}
	for name, body := range map[string]string{
		"duplicate":      strings.Replace(string(b), `"kind":"submit"`, `"kind":"submit","kind":"query"`, 1),
		"alias":          strings.Replace(string(b), `"taskId"`, `"TaskId"`, 1),
		"null-execution": strings.Replace(string(b), `"execution":{}`, `"execution":null`, 1),
		"null-selection": strings.Replace(string(b), `"execution":{}`, `"execution":{"model":null}`, 1),
		"proof":          strings.Replace(string(b), `"grantId":"grant-1"`, `"grantId":"grant-1","proof":"opaque"`, 1),
		"origin-kind":    strings.Replace(string(b), `"kind":"submit"`, `"kind":"origin"`, 1),
		"oversize":       strings.Replace(string(b), r.Prompt, strings.Repeat("x", MaximumPromptBytesV1+1), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := DecodeRequestV1([]byte(body)); e == nil {
				t.Fatal("ambiguous request accepted")
			}
		})
	}
	m := NewMarkerV1(r, "turn-2", "item_turn-2_user", "start", "2026-10-12T01:00:01Z", strings.Repeat("a", 64), r.Prompt)
	if _, e := DecodeMarkerV1(m.RecordV1()); e != nil {
		t.Fatal(e)
	}
	record := m.RecordV1()
	record["proof"] = "removed"
	if _, e := DecodeMarkerV1(record); e == nil {
		t.Fatal("unclosed marker accepted")
	}
}
