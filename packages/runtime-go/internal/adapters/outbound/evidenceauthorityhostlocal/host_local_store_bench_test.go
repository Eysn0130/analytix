package evidenceauthorityhostlocal

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainhost "analytix.local/runtime-go/internal/domain/hostcurrentness"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func BenchmarkHostLocalStoreCurrent(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("heads_%d", count), func(b *testing.B) {
			ctx := context.Background()
			root := b.TempDir()
			access, err := privatecastest.NewAccessAuthority(root)
			if err != nil {
				b.Fatal(err)
			}
			authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "key.json"), false)
			if err != nil {
				b.Fatal(err)
			}
			store, err := OpenHostLocalStore(ctx, filepath.Join(root, "heads"), filepath.Join(root, "selector"), access,
				domainsecurity.SHA256Hex([]byte("benchmark installation")), domainsecurity.SHA256Hex([]byte("benchmark root")), authority)
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			input := domainhost.HeadInputV1{
				InstallationID: store.installationID, RootBindingDigest: store.rootBindingDigest,
				MutationID:                  domainsecurity.SHA256Hex([]byte("benchmark mode")),
				DatasetSnapshotIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
				EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
				PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
				AuthorityKeyID:              authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
			}
			sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
			var last domainhost.HeadV1
			for index := 0; index < count; index++ {
				input.Generation = uint64(index)
				if index != 0 {
					input.PreviousHeadDigest = last.RecordDigest
					input.MutationID = domainsecurity.SHA256Hex([]byte(fmt.Sprintf("mutation %d", index)))
					input.DatasetSnapshotIndexDigest = domainsecurity.SHA256Hex([]byte(fmt.Sprintf("dataset %d", index)))
					input.DatasetSnapshotCount = uint64(index)
				}
				next, err := domainhost.NewHeadV1(input, sign)
				if err != nil {
					b.Fatal(err)
				}
				body, err := domainhost.HeadBytesV1(next)
				if err != nil || store.history.PutIfAbsent(ctx, next.RecordDigest, body) != nil {
					b.Fatalf("prepare benchmark chain: %v", err)
				}
				last = next
			}
			lastBody, err := domainhost.HeadBytesV1(last)
			if err != nil {
				b.Fatal(err)
			}
			if result, err := store.selector.ReplaceExact(ctx, "", lastBody); err != nil || result.State != finalauthorityadapter.SecureMutableProjectionCommitted {
				b.Fatalf("prepare benchmark selector: state=%v err=%v", result.State, err)
			}
			b.ReportMetric(float64(count), "heads")
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				current, found, err := store.Current(ctx)
				if err != nil || !found || current != last {
					b.Fatalf("benchmark currentness failed: found=%v err=%v", found, err)
				}
			}
		})
	}
}
