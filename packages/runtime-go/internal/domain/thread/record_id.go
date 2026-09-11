package thread

import "regexp"

var canonicalRecordIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

// IsCanonicalRecordID accepts only exact host-addressable thread/session IDs.
// Storage adapters must reject aliases instead of normalizing untrusted route
// values into another record's directory.
func IsCanonicalRecordID(value string) bool {
	return canonicalRecordIDPattern.MatchString(value) && !containsDotDot(value)
}

func containsDotDot(value string) bool {
	for index := 1; index < len(value); index++ {
		if value[index-1] == '.' && value[index] == '.' {
			return true
		}
	}
	return false
}
