//go:build darwin && analytix_native_build_probe && !analytix_prod

package nativecomponentsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	nativecomponentregistry "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentregistry"
	securegeneration "analytix.local/runtime-go/internal/adapters/outbound/securegeneration"

	"golang.org/x/sys/unix"
)

func TestSnapshotFromDescriptorsPublishesExactImmutableSourceGeneration(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()

	request := createRequestV1()
	response, err := CreateFromDescriptorsV1(context.Background(), request, manifest, repositoryFile, parent)
	if err != nil || response.Status != "committed" || response.Kind != ResponseKindV1 ||
		response.Operation != OperationCreateV1 || response.RequestNonce != request.RequestNonce ||
		len(response.Components) != 4 || response.GenerationID == "" || response.FileCount != 15 {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	current := filepath.Join(parentPath, RootNameV1, PublicNameV1)
	for _, path := range []string{
		ContextFileNameV1,
		ManifestSnapshotPathV1,
		"rust-toolchain.toml",
		"tools/import_accelerator/Cargo.toml",
		"tools/import_accelerator/Cargo.lock",
		"tools/import_accelerator/src/main.rs",
		"inventory.v1.json",
		"receipt.v1.json",
	} {
		if _, err := os.Stat(filepath.Join(current, filepath.FromSlash(path))); err != nil {
			t.Fatalf("missing snapshot path %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(current, "tools", "import_accelerator", "target")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("excluded target copied: %v", err)
	}
	observation, err := securegeneration.ObserveUnder(
		context.Background(), parent, RootNameV1, PublicNameV1,
		securegeneration.Limits{
			MaxFiles: maxSourceFilesV1, MaxFileBytes: maxSourceFileBytesV1,
			MaxTotalBytes: maxSourceTotalBytesV1, MaxDepth: maxSourceDepthV1,
		},
	)
	if err != nil || !observation.Installed || observation.Current.GenerationID != response.GenerationID {
		t.Fatalf("observation=%#v err=%v", observation, err)
	}
	verifyRequest := verifyRequestForResponseV1(response)
	verified, err := VerifyFromDescriptorsV1(context.Background(), verifyRequest, manifest, repositoryFile, parent)
	if err != nil || verified.Status != "verified" || verified.Operation != OperationVerifyV1 ||
		verified.RequestNonce != verifyRequest.RequestNonce || verified.GenerationID != response.GenerationID ||
		verified.InventorySHA256 != response.InventorySHA256 ||
		verified.GenerationReceiptSHA256 != response.GenerationReceiptSHA256 {
		t.Fatalf("verified=%#v err=%v", verified, err)
	}
	for _, component := range response.Components {
		if component.FileCount != 3 || component.TotalBytes <= 0 || component.SourceDigest == "" || component.CargoLockSHA256 == "" {
			t.Fatalf("component receipt=%#v", component)
		}
	}
	discarded, err := DiscardFromDescriptorsV1(context.Background(), discardRequestV1(response), parent)
	if err != nil || discarded.Status != "discarded" || discarded.GenerationID != response.GenerationID ||
		discarded.InventorySHA256 != response.InventorySHA256 ||
		discarded.GenerationReceiptSHA256 != response.GenerationReceiptSHA256 {
		t.Fatalf("discarded=%#v err=%v", discarded, err)
	}
	assertDirectoryNamesV1(t, parentPath)
}

func TestSnapshotDiscardDoesNotDependOnLiveSourceAndBindsExactReceipt(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	created, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repository, "tools", "data_engine", "src", "main.rs")); err != nil {
		t.Fatal(err)
	}
	mismatch := discardRequestV1(created)
	mismatch.ExpectedInventorySHA256 = nativecomponentregistry.FrozenManifestSHA256V4
	if _, err := DiscardFromDescriptorsV1(context.Background(), mismatch, parent); !errors.Is(err, ErrGeneration) {
		t.Fatalf("mismatched discard error=%v, want generation mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(parentPath, RootNameV1, PublicNameV1)); err != nil {
		t.Fatalf("mismatched discard removed generation: %v", err)
	}
	discarded, err := DiscardFromDescriptorsV1(context.Background(), discardRequestV1(created), parent)
	if err != nil || discarded.Status != "discarded" || discarded.GenerationID != created.GenerationID {
		t.Fatalf("discarded=%#v err=%v", discarded, err)
	}
	assertDirectoryNamesV1(t, parentPath)
}

func TestSnapshotReconcileDiscardHandlesLostCreateResponseAndEmptySession(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	created, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := ReconcileDiscardFromDescriptorsV1(context.Background(), reconcileDiscardRequestV1(), parent)
	if err != nil || discarded.Status != "discarded" || discarded.GenerationID != created.GenerationID ||
		discarded.InventorySHA256 != created.InventorySHA256 ||
		discarded.GenerationReceiptSHA256 != created.GenerationReceiptSHA256 {
		t.Fatalf("discard current=%#v err=%v", discarded, err)
	}
	absent, err := ReconcileDiscardFromDescriptorsV1(context.Background(), reconcileDiscardRequestV1(), parent)
	if err != nil || absent.Status != "discarded" ||
		absent.Disposition != DispositionEmptySessionDiscardedV1 ||
		absent.GenerationID != "" || absent.InventorySHA256 != "" {
		t.Fatalf("discard empty=%#v err=%v", absent, err)
	}
}

func TestSnapshotFromDescriptorsRequiresManifestRepositoryIdentity(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	externalManifest := filepath.Join(t.TempDir(), "native-components.json")
	mustWriteV1(t, externalManifest, manifestBody, 0o644)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, externalManifest)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()

	response, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if response.Status != "" || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	assertDirectoryNamesV1(t, parentPath)
}

func TestSourceScannerRevalidationRejectsPathReplacement(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	body, manifestIdentity, err := readPinnedManifestV1(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := nativecomponentregistry.ParseFrozenManifestV4(body)
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := newScannerV1(context.Background(), repositoryFile)
	if err != nil {
		t.Fatal(err)
	}
	defer scanner.close()
	if err := scanner.scanRequiredRepositoryFile(ManifestSnapshotPathV1, manifestIdentity); err != nil {
		t.Fatal(err)
	}
	for _, component := range parsed.Components {
		if err := scanner.scanComponent(component); err != nil {
			t.Fatal(err)
		}
	}
	original := filepath.Join(repository, "tools", "import_accelerator", "src", "main.rs")
	moved := original + ".moved"
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	mustWriteV1(t, original, []byte("fn main() { panic!(\"replacement\") }\n"), 0o644)
	if err := scanner.revalidate(); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("replacement accepted: %v", err)
	}
}

func TestSnapshotFromDescriptorsRejectsLinksAndNonEmptyParentWithoutPublication(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, repository, parent string)
	}{
		{
			name: "source symlink",
			setup: func(t *testing.T, repository, _ string) {
				t.Helper()
				target := filepath.Join(repository, "tools", "import_accelerator", "target")
				if err := os.RemoveAll(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), target); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "hard linked source",
			setup: func(t *testing.T, repository, _ string) {
				t.Helper()
				source := filepath.Join(repository, "tools", "cleaning_ops", "src", "main.rs")
				if err := os.Link(source, source+".alias"); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non-empty parent",
			setup: func(t *testing.T, _, parent string) {
				t.Helper()
				mustWriteV1(t, filepath.Join(parent, "unexpected"), []byte("blocked"), 0o600)
			},
		},
		{
			name: "source xattr",
			setup: func(t *testing.T, repository, _ string) {
				t.Helper()
				path := filepath.Join(repository, "tools", "cleaning_ops", "src", "main.rs")
				file, err := os.OpenFile(path, os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				if err := unix.Fsetxattr(int(file.Fd()), "com.analytix.untrusted", []byte("unsafe"), 0); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, manifestPath := sourceRepositoryFixtureV1(t)
			parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
			mustMkdirV1(t, parentPath, 0o700)
			test.setup(t, repository, parentPath)
			manifest := mustOpenV1(t, manifestPath)
			defer manifest.Close()
			repositoryFile := mustOpenV1(t, repository)
			defer repositoryFile.Close()
			parent := mustOpenV1(t, parentPath)
			defer parent.Close()
			response, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
			if response.Status != "" || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("response=%#v err=%v", response, err)
			}
			if _, statErr := os.Stat(filepath.Join(parentPath, RootNameV1)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("rejected snapshot published source root: %v", statErr)
			}
		})
	}
}

func TestSnapshotBindsOmittedEmptyDirectoryTopology(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	emptyPath := filepath.Join(repository, "tools", "data_engine", "src", "empty")
	mustMkdirV1(t, emptyPath, 0o755)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	created, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := os.ReadFile(filepath.Join(parentPath, RootNameV1, PublicNameV1, ContextFileNameV1))
	if err != nil || !bytes.Contains(contextBody, []byte(`"omittedEmptyDirectories":["tools/data_engine/src/empty"]`)) {
		t.Fatalf("context=%s err=%v", contextBody, err)
	}
	if _, err := os.Stat(filepath.Join(parentPath, RootNameV1, PublicNameV1, "tools", "data_engine", "src", "empty")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty source directory became an unreceipted payload: %v", err)
	}
	if err := os.Remove(emptyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFromDescriptorsV1(
		context.Background(), verifyRequestForResponseV1(created), manifest, repositoryFile, parent,
	); !errors.Is(err, ErrGeneration) {
		t.Fatalf("changed empty-directory topology accepted: %v", err)
	}
}

func TestVerifyFromDescriptorsRejectsStaleGenerationAndMutation(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	created, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFromDescriptorsV1(
		context.Background(), verifyRequestV1(nativecomponentregistry.FrozenManifestSHA256V4), manifest, repositoryFile, parent,
	); !errors.Is(err, ErrGeneration) {
		t.Fatalf("stale generation accepted: %v", err)
	}
	payload := filepath.Join(parentPath, RootNameV1, PublicNameV1, "tools", "data_engine", "src", "main.rs")
	if err := os.WriteFile(payload, []byte("fn main() { panic!(\"tampered\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFromDescriptorsV1(
		context.Background(), verifyRequestForResponseV1(created), manifest, repositoryFile, parent,
	); !errors.Is(err, ErrGeneration) {
		t.Fatalf("mutated generation accepted: %v", err)
	}
}

func TestDecodeRequestV1RejectsAmbiguousOrInvalidFrames(t *testing.T) {
	valid, err := json.Marshal(createRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeRequestV1(valid); err != nil || decoded.Operation != OperationCreateV1 {
		t.Fatalf("decoded=%#v err=%v", decoded, err)
	}
	for name, raw := range map[string][]byte{
		"unknown":   bytes.Replace(valid, []byte("}"), []byte(",\"unknown\":true}"), 1),
		"duplicate": bytes.Replace(valid, []byte("\"kind\":"), []byte("\"kind\":\"duplicate\",\"kind\":"), 1),
		"trailing":  append(append([]byte(nil), valid...), []byte(" true")...),
		"bad nonce": bytes.Replace(valid, []byte(nativecomponentregistry.FrozenManifestSHA256V4), []byte("not-a-digest"), 1),
		"create id": bytes.Replace(valid, []byte("\"expected_generation_id\":\"\""), []byte("\"expected_generation_id\":\""+nativecomponentregistry.FrozenManifestSHA256V4+"\""), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRequestV1(raw); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("invalid request accepted: %s", raw)
			}
		})
	}
}

func TestReadRequestFrameV1RequiresOneLFThenEOF(t *testing.T) {
	valid, err := json.Marshal(createRequestV1())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"valid":          append(append([]byte(nil), valid...), '\n'),
		"missing lf":     append([]byte(nil), valid...),
		"crlf":           append(append([]byte(nil), valid...), '\r', '\n'),
		"nul":            append(append(append([]byte(nil), valid...), 0), '\n'),
		"oversized":      append(bytes.Repeat([]byte{'x'}, requestLimitV1+1), '\n'),
		"second frame":   append(append(append([]byte(nil), valid...), '\n'), []byte("{}\n")...),
		"trailing bytes": append(append(append([]byte(nil), valid...), '\n'), 'x'),
	} {
		t.Run(name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				_, _ = writer.Write(body)
				_ = writer.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			frame, readErr := readRequestFrameV1(ctx, reader)
			_ = reader.Close()
			if name == "valid" {
				if readErr != nil || !bytes.Equal(frame, valid) {
					t.Fatalf("frame=%q err=%v", frame, readErr)
				}
				return
			}
			if !errors.Is(readErr, ErrInvalidInput) {
				t.Fatalf("invalid frame accepted: frame=%q err=%v", frame, readErr)
			}
		})
	}
}

func TestReadRequestFrameV1HonorsDeadlineAndCommandPipeType(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := readRequestFrameV1(ctx, reader); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blocked frame did not fail closed: %v", err)
	}
	_ = reader.Close()
	_ = writer.Close()

	regular, err := os.CreateTemp(t.TempDir(), "regular-output")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if file, err := openCommandPipeV1(int(regular.Fd()), "regular"); !errors.Is(err, ErrInvalidInput) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("regular command channel accepted: %v", err)
	}
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipeReader.Close()
	defer pipeWriter.Close()
	opened, err := openCommandPipeV1(int(pipeReader.Fd()), "pipe")
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotIgnoresMetadataOnlyOnExcludedTargetDirectory(t *testing.T) {
	repository, manifestPath := sourceRepositoryFixtureV1(t)
	targetPath := filepath.Join(repository, "tools", "import_accelerator", "target")
	target, err := os.Open(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Fsetxattr(int(target.Fd()), "com.analytix.ignored-target-metadata", []byte("ignored"), 0); err != nil {
		_ = target.Close()
		t.Fatal(err)
	}
	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(t.TempDir(), "snapshot-parent")
	mustMkdirV1(t, parentPath, 0o700)
	manifest := mustOpenV1(t, manifestPath)
	defer manifest.Close()
	repositoryFile := mustOpenV1(t, repository)
	defer repositoryFile.Close()
	parent := mustOpenV1(t, parentPath)
	defer parent.Close()
	created, err := CreateFromDescriptorsV1(context.Background(), createRequestV1(), manifest, repositoryFile, parent)
	if err != nil || created.Status != "committed" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
}

func createRequestV1() RequestV1 {
	return RequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1, Operation: OperationCreateV1,
		RequestNonce: nativecomponentregistry.FrozenManifestSHA256V4,
		SessionName:  sessionNamePrefixV1 + "test01",
	}
}

func verifyRequestV1(generationID string) RequestV1 {
	return RequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1, Operation: OperationVerifyV1,
		RequestNonce: nativecomponentregistry.FrozenManifestSHA256V4, ExpectedGenerationID: generationID,
		ExpectedInventorySHA256:         nativecomponentregistry.FrozenManifestSHA256V4,
		ExpectedGenerationReceiptSHA256: nativecomponentregistry.FrozenManifestSHA256V4,
		SessionName:                     sessionNamePrefixV1 + "test01",
	}
}

func verifyRequestForResponseV1(response ResponseV1) RequestV1 {
	return RequestV1{
		Kind: RequestKindV1, SchemaVersion: SchemaVersionV1, Operation: OperationVerifyV1,
		RequestNonce:         nativecomponentregistry.FrozenManifestSHA256V4,
		ExpectedGenerationID: response.GenerationID, ExpectedInventorySHA256: response.InventorySHA256,
		ExpectedGenerationReceiptSHA256: response.GenerationReceiptSHA256,
		SessionName:                     sessionNamePrefixV1 + "test01",
	}
}

func discardRequestV1(response ResponseV1) RequestV1 {
	request := verifyRequestForResponseV1(response)
	request.Operation = OperationDiscardV1
	return request
}

func reconcileDiscardRequestV1() RequestV1 {
	request := createRequestV1()
	request.Operation = OperationReconcileDiscardV1
	return request
}

func sourceRepositoryFixtureV1(t *testing.T) (string, string) {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	mustMkdirV1(t, repository, 0o700)
	manifestBody, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "scripts", "native-components.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repository, filepath.FromSlash(ManifestSnapshotPathV1))
	mustWriteV1(t, manifestPath, manifestBody, 0o644)
	manifest, err := nativecomponentregistry.ParseFrozenManifestV4(manifestBody)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range manifest.Components {
		root := filepath.Join(repository, filepath.FromSlash(component.SourceRoot))
		mustWriteV1(t, filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \""+component.BinaryName+"\"\nversion = \"0.1.0\"\n"), 0o644)
		mustWriteV1(t, filepath.Join(root, "Cargo.lock"), []byte("version = 3\n"), 0o644)
		mustWriteV1(t, filepath.Join(root, "src", "main.rs"), []byte("fn main() {}\n"), 0o644)
		mustWriteV1(t, filepath.Join(root, "target", "ignored.bin"), []byte("ignored"), 0o644)
	}
	mustWriteV1(t, filepath.Join(repository, "rust-toolchain.toml"), []byte("[toolchain]\nchannel = \"1.88.0\"\n"), 0o644)
	return repository, manifestPath
}

func mustWriteV1(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	mustMkdirV1(t, filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirV1(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustOpenV1(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func assertDirectoryNamesV1(t *testing.T, path string, expected ...string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(expected) {
		t.Fatalf("entries=%v expected=%v", entries, expected)
	}
	for index, name := range expected {
		if entries[index].Name() != name {
			t.Fatalf("entries=%v expected=%v", entries, expected)
		}
	}
}
