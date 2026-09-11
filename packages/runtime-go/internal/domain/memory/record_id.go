package memory

import (
	"strconv"
	"strings"
)

const RecordIDPrefix = "mem_go_"

// IsCanonicalRecordID accepts only host-issued memory record identities.
// Filesystem sanitization is never an identity or aliasing mechanism.
func IsCanonicalRecordID(value string) bool {
	if !strings.HasPrefix(value, RecordIDPrefix) {
		return false
	}
	suffix := strings.TrimPrefix(value, RecordIDPrefix)
	sequence, err := strconv.ParseUint(suffix, 10, 63)
	return err == nil && sequence > 0 && strconv.FormatUint(sequence, 10) == suffix
}
