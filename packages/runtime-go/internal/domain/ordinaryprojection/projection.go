package ordinaryprojection

import (
	"reflect"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecret "analytix.local/runtime-go/internal/domain/secretprojection"
)

const maxProjectionPassesV1 = 8

// ProjectTextV1 composes credential and restricted-PII projection to a fixed
// point. Each projector can shorten text and expose a token boundary that the
// other projector could not see in the previous representation, so a single
// secret-then-privacy pass is not a safe ordinary-output boundary.
func ProjectTextV1(text string, explicitSecrets ...string) string {
	current := text
	for pass := 0; pass < maxProjectionPassesV1; pass++ {
		credentialSafe := domainsecret.ProjectTextV1(current, explicitSecrets...)
		next := domainprivacy.ProjectText(credentialSafe).Text
		if next == current {
			return current
		}
		current = next
	}
	return domainsecret.RedactedV1
}

// ProjectValueV1 applies the same fixed-point rule to JSON-like ordinary
// records. Projection-limit or non-convergent values collapse to the closed
// scalar sentinel, causing callers that require a structured record to reject
// the entire record rather than persist a partially projected value.
func ProjectValueV1(value any) any {
	current := value
	for pass := 0; pass < maxProjectionPassesV1; pass++ {
		credentialSafe := domainsecret.ProjectValueV1(current)
		next, _ := domainprivacy.ProjectPublicValue(credentialSafe)
		if reflect.DeepEqual(next, current) {
			return current
		}
		current = next
	}
	return domainsecret.RedactedV1
}
