package toolresultsnapshot

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	snapshotport "analytix.local/runtime-go/internal/ports/toolresultsnapshot"
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
)

const (
	BodyLimitV1       = 32 << 10
	EnvelopeLimitV1   = 256 << 10
	RootByteLimitV1   = 128 << 20
	RootRecordLimitV1 = 4096
)

type Store struct {
	cas    *finalauthority.SecurePrivateCAS
	mu     sync.Mutex
	sizes  map[string]int
	bytes  int64
	paused bool
}

func NewStore(ctx context.Context, root string, access finalauthority.SecurePrivateCASAccessAuthority) (*Store, error) {
	cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(ctx, root, EnvelopeLimitV1, access)
	if err != nil {
		return nil, err
	}
	store := &Store{cas: cas}
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
			return snapshotport.ErrUnavailable
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

func (s *Store) Put(ctx context.Context, snapshot domaintoolresult.ProtectedSnapshotV1) (domaintoolresult.ProtectedSnapshotBindingV1, error) {
	binding, err := domaintoolresult.ProtectedSnapshotBindingForV1(snapshot)
	if err != nil || s == nil || s.cas == nil {
		return domaintoolresult.ProtectedSnapshotBindingV1{}, snapshotport.ErrUnavailable
	}
	body, err := domaintoolresult.ProtectedSnapshotBytesV1(snapshot)
	if err != nil {
		return domaintoolresult.ProtectedSnapshotBindingV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Exact existing content wins before reservation/capacity. A metadata-only
	// quota refusal cannot change the tool's real outcome or trigger execution.
	if current, readErr := s.cas.Read(ctx, binding.SnapshotDigest); readErr == nil {
		if !bytes.Equal(current, body) {
			return domaintoolresult.ProtectedSnapshotBindingV1{}, snapshotport.ErrUnavailable
		}
		if _, err := parse(binding.SnapshotDigest, current); err != nil {
			return domaintoolresult.ProtectedSnapshotBindingV1{}, err
		}
		if _, known := s.sizes[binding.SnapshotDigest]; !known {
			s.sizes[binding.SnapshotDigest] = len(current)
			s.bytes += int64(len(current))
		}
		return binding, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return domaintoolresult.ProtectedSnapshotBindingV1{}, snapshotport.ErrUnavailable
	}
	if s.paused || len(s.sizes) >= RootRecordLimitV1 || s.bytes+int64(len(body)) > RootByteLimitV1 {
		return domaintoolresult.ProtectedSnapshotBindingV1{}, snapshotport.ErrCapacity
	}
	s.sizes[binding.SnapshotDigest] = len(body)
	s.bytes += int64(len(body))
	putErr := s.cas.PutIfAbsent(ctx, binding.SnapshotDigest, body)
	current, readErr := s.cas.Read(ctx, binding.SnapshotDigest)
	if readErr == nil && bytes.Equal(current, body) {
		if _, err := parse(binding.SnapshotDigest, current); err == nil {
			return binding, nil
		}
	}
	// Never refund an uncertain commit. New capture pauses until startup's
	// existing recovery and a fresh complete inventory; immutable reads remain.
	s.paused = true
	return domaintoolresult.ProtectedSnapshotBindingV1{}, errors.Join(snapshotport.ErrUnavailable, putErr, readErr)
}

func (s *Store) Read(ctx context.Context, binding domaintoolresult.ProtectedSnapshotBindingV1) (domaintoolresult.ProtectedSnapshotV1, error) {
	if s == nil || s.cas == nil {
		return domaintoolresult.ProtectedSnapshotV1{}, snapshotport.ErrUnavailable
	}
	if _, err := domaintoolresult.ParseProtectedSnapshotBindingV1(binding); err != nil {
		return domaintoolresult.ProtectedSnapshotV1{}, snapshotport.ErrUnavailable
	}
	body, err := s.cas.Read(ctx, binding.SnapshotDigest)
	if err != nil {
		return domaintoolresult.ProtectedSnapshotV1{}, snapshotport.ErrUnavailable
	}
	snapshot, err := parse(binding.SnapshotDigest, body)
	if err != nil || domainsecurity.SHA256Hex([]byte(snapshot.Capture.Body)) != binding.BodyDigest {
		return domaintoolresult.ProtectedSnapshotV1{}, snapshotport.ErrUnavailable
	}
	return snapshot, nil
}

func parse(digest string, body []byte) (domaintoolresult.ProtectedSnapshotV1, error) {
	if domainsecurity.SHA256Hex(body) != digest {
		return domaintoolresult.ProtectedSnapshotV1{}, snapshotport.ErrUnavailable
	}
	return domaintoolresult.ParseProtectedSnapshotV1(body)
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
		return snapshotport.ErrUnavailable
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
		return snapshotport.ErrUnavailable
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
