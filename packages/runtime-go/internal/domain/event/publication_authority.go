package event

var acceptedFinalPublicationAuthorityFields = map[string]bool{
	"acceptedFinal":            true,
	"acceptedFinalView":        true,
	"acceptedFinalDigest":      true,
	"publicationCommitId":      true,
	"publicationEventId":       true,
	"publicationSlot":          true,
	"publicationPayloadDigest": true,
}

var strongPrivateAcceptedFinalAuthorityFields = map[string]bool{
	"acceptedFinal":                  true,
	"factFinalWitnessAdmission":      true,
	"publicationSnapshotProof":       true,
	"publicationSnapshotProofDigest": true,
}

var generalTerminalPublicationAuthorityFields = map[string]bool{
	"generalTerminalCASBinding":         true,
	"generalTerminalCASBindingDigest":   true,
	"generalTerminalPublication":        true,
	"generalTerminalPublicationArchive": true,
	"generalTerminalCommitId":           true,
	"generalTerminalEventId":            true,
	"generalTerminalSlot":               true,
	"generalTerminalPayloadDigest":      true,
	"generalTerminalAuthorityKind":      true,
	"generalTerminalAuthorityDigest":    true,
}

// ContainsAcceptedFinalPublicationAuthority detects fields reserved for the
// host's accepted-final CAS and atomic publication manifest. Ordinary and
// generic event paths must reject these fields at any nesting depth.
func ContainsAcceptedFinalPublicationAuthority(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if acceptedFinalPublicationAuthorityFields[key] || ContainsAcceptedFinalPublicationAuthority(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ContainsAcceptedFinalPublicationAuthority(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if ContainsAcceptedFinalPublicationAuthority(child) {
				return true
			}
		}
	}
	return false
}

// ContainsPrivateAcceptedFinalAuthority detects host-private accepted-final
// records and their detached authority fragments. A complete signed record is
// still private: signatures and compact witness fields do not authorize it for
// generic events, HTTP/SSE, or ordinary history. A bare securityContext key is
// not sufficient to classify an object as an accepted-final record because
// ordinary internal lifecycle events also carry it; their closed projector
// removes that field before publication.
func ContainsPrivateAcceptedFinalAuthority(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if historicalAcceptedFinalRecordV5HasExactKeys(typed) ||
			completePrivateAcceptedFinalRecordV5Structure(typed) {
			return true
		}
		for key, child := range typed {
			if strongPrivateAcceptedFinalAuthorityFields[key] || ContainsPrivateAcceptedFinalAuthority(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ContainsPrivateAcceptedFinalAuthority(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if ContainsPrivateAcceptedFinalAuthority(child) {
				return true
			}
		}
	}
	return false
}

func completePrivateAcceptedFinalRecordV5Structure(record map[string]any) bool {
	version, versionOK := numericPublicationAuthorityVersionV1(record["schemaVersion"])
	acceptedFinal, acceptedFinalOK := record["acceptedFinal"].(map[string]any)
	if !versionOK || version != 5 || !acceptedFinalOK ||
		!historicalAcceptedFinalRecordV5HasExactKeys(acceptedFinal) {
		return false
	}
	required := []string{
		"schemaVersion", "securityContext", "envelope", "renderedText", "registryHead",
		"publicationIntent", "acceptedFinal", "privateRecordDigest", "storeDigest",
	}
	for _, key := range required {
		if _, present := record[key]; !present {
			return false
		}
	}
	if len(record) == len(required) {
		return true
	}
	_, proofPresent := record["publicationSnapshotProof"]
	return proofPresent && len(record) == len(required)+1
}

func numericPublicationAuthorityVersionV1(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	}
	return 0, false
}

// ContainsGeneralTerminalPublicationAuthority detects both the thread CAS
// binding/outbox and the six-field durable event marker group. Presence is
// authoritative even when the value is null, so malformed current records
// cannot be reclassified as legacy data by a migration.
func ContainsGeneralTerminalPublicationAuthority(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if generalTerminalPublicationAuthorityFields[key] || ContainsGeneralTerminalPublicationAuthority(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if ContainsGeneralTerminalPublicationAuthority(child) {
				return true
			}
		}
	case []map[string]any:
		for _, child := range typed {
			if ContainsGeneralTerminalPublicationAuthority(child) {
				return true
			}
		}
	}
	return false
}

func ContainsTerminalPublicationAuthority(value any) bool {
	return ContainsAcceptedFinalPublicationAuthority(value) || ContainsGeneralTerminalPublicationAuthority(value)
}
