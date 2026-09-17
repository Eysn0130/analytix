package pluginmaterializationfs

import (
	"context"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
)

func TestDevelopmentIntentResumesInterruptedTransactionAcrossReopen(t *testing.T) {
	ctx := context.Background()
	home, source := realTempDir(t), writeDevelopmentSourceV1(t, "analytix-documents")
	authority := newTestAuthority()
	binding := developmentBindingV1(t, source)
	first, err := binding.NewIntentV1(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPackageStoreV1(home, "analytix-documents", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareDevelopmentIntentV1(ctx, first); err != nil {
		t.Fatal(err)
	}
	later, _ := binding.NewIntentV1(time.Now().UTC().Add(time.Minute))
	reopened, err := OpenExistingPackageStoreV1(home, "analytix-documents")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := reopened.PrepareDevelopmentIntentV1(ctx, later)
	if err != nil || resumed != first {
		t.Fatal("restart changed pending intent", err)
	}
	service, err := pluginapp.NewDevelopmentSourceServiceV1(reopened, authority, binding, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Materialize(ctx, resumed)
	if err != nil || result.Receipt.IntentID != first.IntentID {
		t.Fatal("retained intent failed", err)
	}
}
