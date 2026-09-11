package memory

import (
	"errors"
	"regexp"
	"strings"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
)

var (
	ErrContentNotAdmissibleV1  = errors.New("memory content is not admissible")
	ErrMutationNotAdmissibleV1 = errors.New("memory mutation is invalid")

	memoryCaseAliasCandidateV1 = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_:])(?:acct|card):[a-z0-9]+(?:$|[^a-z0-9_:])`)
	memoryUnixSourcePathV1     = regexp.MustCompile(`(?:^|[[:space:]"'=(])/(?:Users|Volumes|private|var|tmp|home|cases?)/[^[:space:]"']+`)
	memoryWindowsSourcePathV1  = regexp.MustCompile(`(?i)(?:^|[[:space:]"'=(])[a-z]:\\[^[:space:]"']+`)
)

// ValidateManualContentV1 is the final free-form Memory owner admission. It
// complements producer- and case-channel typing; it is not used as a detector
// that grants case authority or attempts to classify every possible PII shape.
func ValidateManualContentV1(content string) error {
	if content == "" || content != strings.TrimSpace(content) || len(content) > 64*1024 ||
		domaincaseentity.ContainsReferenceCandidateV1(content) ||
		domaincontrolledaccount.ContainsCompleteFinancialAccountCandidateV2(content) ||
		memoryCaseAliasCandidateV1.MatchString(content) ||
		memoryUnixSourcePathV1.MatchString(content) || memoryWindowsSourcePathV1.MatchString(content) ||
		containsRawToolBodyMarkerV1(content) {
		return ErrContentNotAdmissibleV1
	}
	return nil
}

func containsRawToolBodyMarkerV1(content string) bool {
	lower := strings.ToLower(content)
	if !strings.ContainsAny(content, "{}[]") {
		return false
	}
	for _, marker := range []string{
		`"structuredcontent"`, `"rawresult"`, `"hostonly"`, `"toolcallid"`,
		`"sourcefileid"`, `"sourcerownumber"`, `"subjectref"`,
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
