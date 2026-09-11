package responses

import (
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestBodyCarriesHostOutputTokenBudget(t *testing.T) {
	body := Body(domainmodel.Request{Model: "bounded", MaxOutputTokens: 321})
	if body["max_output_tokens"] != 321 {
		t.Fatalf("responses output token budget missing: %#v", body)
	}
}
