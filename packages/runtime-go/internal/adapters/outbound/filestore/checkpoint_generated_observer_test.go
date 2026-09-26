package filestore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

func TestGeneratedCheckpointObservesRawBytesAcrossReopenWithoutChangingTextPolicy(t *testing.T) {
	ctx, workspace := context.Background(), t.TempDir()
	target := filepath.Join(workspace, "report.docx")
	observer := CheckpointOperationObserver{}
	before, err := observer.CaptureBefore(ctx, workspace, target)
	if err != nil || before.Existed {
		t.Fatal("absent capture failed", err)
	}
	data := bytes.Repeat([]byte{0, 255, 1, 3}, 200000)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := observer.ObserveGenerated(ctx, workspace, before.PathAuthority, target); got.ObservationStatus != "exact" || got.Hash != checkpointapp.HashBytes(data) {
		t.Fatal("binary observation failed")
	}
	if got := (CheckpointOperationObserver{}).ObserveGeneratedRelative(ctx, workspace, before.PathAuthority); got.ObservationStatus != "exact" || got.Hash != checkpointapp.HashBytes(data) {
		t.Fatal("reopened observation failed")
	}
	if got := observer.Observe(ctx, workspace, before.PathAuthority, target); got.ObservationStatus == "exact" {
		t.Fatal("ordinary text policy widened")
	}
	alias := filepath.Join(workspace, "alias.docx")
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}
	if got := observer.ObserveGeneratedRelative(ctx, workspace, before.PathAuthority); got.ObservationStatus == "exact" {
		t.Fatal("hard-link alias accepted")
	}
}

func TestGeneratedCheckpointRejectsOversizedFile(t *testing.T) {
	ctx, workspace := context.Background(), t.TempDir()
	target := filepath.Join(workspace, "large.docx")
	observer := CheckpointOperationObserver{}
	before, err := observer.CaptureBefore(ctx, workspace, target)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(codecport.MaxDocumentBytes + 1)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got := observer.ObserveGeneratedRelative(ctx, workspace, before.PathAuthority); got.ObservationStatus == "exact" {
		t.Fatal("oversized artifact accepted")
	}
}
