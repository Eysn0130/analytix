package job

import (
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainordinary "analytix.local/runtime-go/internal/domain/ordinaryprojection"
)

// ProjectPersistableUntrustedOutputV1 is the only projection for process,
// background-job, and unbound child output that may enter ordinary durable
// job state. Provider reasoning markup or serialized reasoning fields cause
// the whole value to be withheld; otherwise restricted PII is masked.
func ProjectPersistableUntrustedOutputV1(value string) string {
	public, err := domainevent.FilterPublicText(value)
	if err != nil {
		return ""
	}
	return domainordinary.ProjectTextV1(public)
}
