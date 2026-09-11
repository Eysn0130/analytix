package casethreadauthority

import (
	"context"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type Store interface {
	PutIfAbsent(context.Context, domainsecurity.CaseThreadAuthorityRecord) error
	List(context.Context) ([]domainsecurity.CaseThreadAuthorityRecord, error)
	HasRecords(context.Context) (bool, error)
}
