package processdisplay

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	display "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	snapshotport "analytix.local/runtime-go/internal/ports/processdisplay"
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
)

// Limits are purpose-specific admission, never borrowed automatically from the
// tool root. Existing immutable reads remain possible when writes reach quota.
type LimitsV1 struct {
	Bytes   int64
	Records int
}

const EnvelopeLimitV1 = display.SettledHistoryEnvelopeLimitV1
const RootByteLimitV1 int64 = 128 << 20
const RootRecordLimitV1 = 4096

func DefaultLimitsV1() LimitsV1 {
	return LimitsV1{Bytes: RootByteLimitV1, Records: RootRecordLimitV1}
}

type Store struct {
	cas    *finalauthority.SecurePrivateCAS
	mu     sync.Mutex
	sizes  map[string]int
	bytes  int64
	paused bool
	limits LimitsV1
}

func NewStore(ctx context.Context, root string, access finalauthority.SecurePrivateCASAccessAuthority, limits LimitsV1) (*Store, error) {
	if limits.Bytes <= 0 || limits.Records <= 0 || limits.Bytes > RootByteLimitV1 || limits.Records > RootRecordLimitV1 {
		return nil, snapshotport.ErrUnavailableV1
	}
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, root, EnvelopeLimitV1, access)
	if err != nil {
		return nil, err
	}
	store := &Store{cas: cas, limits: limits}
	if err := store.scan(ctx); err != nil {
		_ = cas.Close()
		return nil, err
	}
	return store, nil
}

// Adopt only a complete strict inventory, including every orphan blob. No
// Visit callback reenters CAS, and capacity alone does not disable old reads.
func (s *Store) scan(ctx context.Context) error {
	sizes := map[string]int{}
	total := int64(0)
	err := s.cas.Visit(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		if _, exists := sizes[file.Digest]; exists {
			return snapshotport.ErrUnavailableV1
		}
		if _, err := parse(file.Digest, file.Body); err != nil {
			return err
		}
		sizes[file.Digest] = len(file.Body)
		total += int64(len(file.Body))
		return nil
	})
	if err != nil {
		return err
	}
	s.sizes = sizes
	s.bytes = total
	s.paused = false
	return nil
}

func (s *Store) Put(ctx context.Context, snapshot display.SettledHistoryV1) (display.SettledHistoryAnchorV1, error) {
	binding, err := display.SettledHistoryAnchorForV1(snapshot)
	if err != nil || s == nil || s.cas == nil {
		return display.SettledHistoryAnchorV1{}, snapshotport.ErrUnavailableV1
	}
	body, err := display.SettledHistoryBytesV1(snapshot)
	if err != nil {
		return display.SettledHistoryAnchorV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Exact existing content wins before reservation/capacity. A metadata-only
	// quota refusal cannot change the turn's real outcome or trigger execution.
	if current, readErr := s.cas.Read(ctx, binding.RecordDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return display.SettledHistoryAnchorV1{}, snapshotport.ErrUnavailableV1
		}
		if _, err := parse(binding.RecordDigest, current); err != nil {
			return display.SettledHistoryAnchorV1{}, err
		}
		if _, known := s.sizes[binding.RecordDigest]; !known {
			s.sizes[binding.RecordDigest] = len(current)
			s.bytes += int64(len(current))
		}
		return binding, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return display.SettledHistoryAnchorV1{}, snapshotport.ErrUnavailableV1
	}
	if s.paused || len(s.sizes) >= s.limits.Records || s.bytes+int64(len(body)) > s.limits.Bytes {
		return display.SettledHistoryAnchorV1{}, snapshotport.ErrCapacityV1
	}
	s.sizes[binding.RecordDigest] = len(body)
	s.bytes += int64(len(body))
	putErr := s.cas.PutIfAbsent(ctx, binding.RecordDigest, body)
	current, readErr := s.cas.Read(ctx, binding.RecordDigest)
	if readErr == nil && bytes.Equal(current, body) {
		if _, err := parse(binding.RecordDigest, current); err == nil {
			return binding, nil
		}
	}
	// Never refund an uncertain commit. New capture pauses until startup's
	// existing recovery and a fresh complete inventory; immutable reads remain.
	s.paused = true
	return display.SettledHistoryAnchorV1{}, errors.Join(snapshotport.ErrUnavailableV1, putErr, readErr)
}

func (s *Store) Read(ctx context.Context, binding display.SettledHistoryAnchorV1) (display.SettledHistoryV1, error) {
	if s == nil || s.cas == nil {
		return display.SettledHistoryV1{}, snapshotport.ErrUnavailableV1
	}
	if err := display.ValidateSettledHistoryAnchorV1(binding); err != nil || binding.Availability != "available" {
		return display.SettledHistoryV1{}, snapshotport.ErrUnavailableV1
	}
	body, err := s.cas.Read(ctx, binding.RecordDigest)
	if err != nil {
		return display.SettledHistoryV1{}, snapshotport.ErrUnavailableV1
	}
	snapshot, err := parse(binding.RecordDigest, body)
	actual, bindingErr := display.SettledHistoryAnchorForV1(snapshot)
	if err != nil || bindingErr != nil || actual != binding {
		return display.SettledHistoryV1{}, snapshotport.ErrUnavailableV1
	}
	return snapshot, nil
}

func parse(digest string, body []byte) (display.SettledHistoryV1, error) {
	if domainsecurity.SHA256Hex(body) != digest {
		return display.SettledHistoryV1{}, snapshotport.ErrUnavailableV1
	}
	return display.ParseSettledHistoryV1(body)
}
func (s *Store) Close() error {
	if s == nil || s.cas == nil {
		return nil
	}
	return s.cas.Close()
}

type PreparedRecoveryV1 struct {
	cas       *finalauthority.PreparedSecurePrivateCASRecoveryV1
	validated bool
}

func PrepareRecoveryV1(ctx context.Context, root string, access finalauthority.SecurePrivateCASRecoveryAccessAuthority) (*PreparedRecoveryV1, error) {
	cas, err := finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(ctx, root, EnvelopeLimitV1, access)
	if err != nil {
		return nil, err
	}
	return &PreparedRecoveryV1{cas: cas}, nil
}
func (p *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if p == nil || p.cas == nil {
		return snapshotport.ErrUnavailableV1
	}
	err := p.cas.VisitCommittedFiles(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		_, err := parse(file.Digest, file.Body)
		return err
	})
	if err != nil {
		return err
	}
	p.validated = true
	return nil
}
func (p *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if p == nil || p.cas == nil || !p.validated {
		return snapshotport.ErrUnavailableV1
	}
	return p.cas.Revalidate(ctx)
}
func (p *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	return nil
}
func (p *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	if p == nil || p.cas == nil {
		return nil
	}
	return []*finalauthority.PreparedSecurePrivateCASRecoveryV1{p.cas}
}
