//go:build darwin || linux || windows

package finalauthority

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrivateCASSemanticApplyAllowsOnlyScopedPreparedObservation(t *testing.T) {
	for _, operation := range []string{"prepare", "bodies", "materials"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newPrivateCASRecoveryV4Fixture(t)
			if err := os.Remove(fixture.ordinaryResidue); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(fixture.ownerRoot, "records")
			prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, fixture.access)
			if err != nil {
				t.Fatal(err)
			}
			live, err := OpenSecurePrivateCASWithAccessAuthority(root, 4096, fixture.access)
			if err != nil {
				t.Fatal(err)
			}
			defer live.Close()
			var escaped context.Context
			err = WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(context.Background(), fixture.participants(t), []string{"accepted-finals/records"}, func(ctx context.Context) error {
				escaped = context.WithoutCancel(ctx)
				bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				defer cancel()
				var err error
				count := 0
				switch operation {
				case "prepare":
					prepared, err = PrepareSecurePrivateCASRecoveryIfPresent(bounded, root, 4096, fixture.access)
					if err == nil && !prepared.Present() {
						return errors.New("scoped observation lost actual owner")
					}
				case "bodies":
					err = prepared.VisitCommittedFiles(bounded, func(file SecurePrivateCASFile) error {
						count++
						if file.Digest != privateCASRecoveryV4Digest || len(file.Body) == 0 {
							return errors.New("scoped body observation lost exact record")
						}
						return nil
					})
				case "materials":
					err = prepared.VisitCommittedMaterials(bounded, func(material SecurePrivateCASPreparedMaterialV1) error {
						count++
						if material.Digest != privateCASRecoveryV4Digest || material.ByteLength == 0 {
							return errors.New("scoped material observation lost exact record")
						}
						return nil
					})
				}
				if err != nil {
					return fmt.Errorf("read-only %s reentered live barrier: %w", operation, err)
				}
				if operation != "prepare" && count != 1 {
					return errors.New("scoped observation did not read complete original inventory")
				}
				canceled, cancelNow := context.WithCancel(ctx)
				cancelNow()
				if _, err := PrepareSecurePrivateCASRecoveryIfPresent(canceled, root, 4096, fixture.access); !errors.Is(err, context.Canceled) {
					return fmt.Errorf("scoped observation lost cancellation cause: %w", err)
				}
				outside, stopOutside := context.WithTimeout(context.Background(), 30*time.Millisecond)
				defer stopOutside()
				if _, err := PrepareSecurePrivateCASRecoveryIfPresent(outside, root, 4096, fixture.access); !errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("unscoped observation entered semantic exclusion: %w", err)
				}
				opening, stopOpening := context.WithTimeout(ctx, 30*time.Millisecond)
				defer stopOpening()
				if store, err := OpenSecurePrivateCASWithAccessAuthorityContext(opening, root, 4096, fixture.access); !errors.Is(err, context.DeadlineExceeded) {
					if store != nil {
						_ = store.Close()
					}
					return fmt.Errorf("observation context granted live opening: %w", err)
				}
				writing, stopWriting := context.WithTimeout(ctx, 30*time.Millisecond)
				defer stopWriting()
				if err := live.PutIfAbsent(writing, fmt.Sprintf("%064x", 300), []byte(`{"unexpected":"write"}`)); !errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("observation context granted preopened live write: %w", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareSecurePrivateCASRecoveryIfPresent(escaped, root, 4096, fixture.access); err == nil {
				t.Fatal("escaped callback context retained observation admission")
			}
			if err := WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(context.Background(), nil, nil, func(context.Context) error {
				bounded, cancel := context.WithTimeout(escaped, 50*time.Millisecond)
				defer cancel()
				_, err := PrepareSecurePrivateCASRecoveryIfPresent(bounded, root, 4096, fixture.access)
				if err == nil || errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("prior observation scope was readmitted or waited in later recovery: %w", err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, fixture.access); err != nil {
				t.Fatal(err)
			}
			fixture.requireCommittedRecord(t)
		})
	}
}

func TestPrivateCASSemanticObservationDrainsBeforeRecoveryRelease(t *testing.T) {
	fixture := newPrivateCASRecoveryV4Fixture(t)
	if err := os.Remove(fixture.ordinaryResidue); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(fixture.ownerRoot, "records")
	prepared, err := PrepareSecurePrivateCASRecoveryIfPresent(context.Background(), root, 4096, fixture.access)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finish := make(chan struct{})
	observerDone := make(chan error, 1)
	ownerDone := make(chan error, 1)
	callbackReturned := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		ownerDone <- WithRetiredPreparedSecurePrivateCASGenerationsForSemanticApplyV1(ctx, nil, nil, func(observation context.Context) error {
			go func() {
				observerDone <- prepared.VisitCommittedFiles(observation, func(SecurePrivateCASFile) error {
					close(entered)
					select {
					case <-finish:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				return ctx.Err()
			}
			close(callbackReturned)
			return nil
		})
	}()
	select {
	case <-callbackReturned:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	probe, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	_, err = OpenSecurePrivateCASWithAccessAuthorityContext(probe, root, 4096, fixture.access)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("callback exit released recovery before observer drained: %v", err)
	}
	select {
	case err := <-ownerDone:
		t.Fatalf("owner returned before admitted observer finished: %v", err)
	default:
	}
	close(finish)
	if err := <-observerDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("closing observation did not preserve cancellation: %v", err)
	}
	if err := <-ownerDone; err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSecurePrivateCASWithAccessAuthorityContext(ctx, root, 4096, fixture.access)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}
