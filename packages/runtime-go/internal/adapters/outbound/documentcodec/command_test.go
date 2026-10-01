package documentcodec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
)

func TestCodecCommandDoesNotInheritHostSecretsOrOptions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture")
	}
	t.Setenv("ANALYTIX_CODEC_TEST_SECRET", "synthetic-only")
	t.Setenv("NODE_OPTIONS", "synthetic-not-an-option")
	entry := filepath.Join(t.TempDir(), "fixture.sh")
	if err := os.WriteFile(entry, []byte("[ -z \"$ANALYTIX_CODEC_TEST_SECRET\" ] && [ -z \"$NODE_OPTIONS\" ] && [ \"$ELECTRON_RUN_AS_NODE\" = 1 ] || exit 4\nprintf 'synthetic-codec-bytes'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := New("/bin/sh", entry).Encode(context.Background(), codecport.Input{SchemaVersion: 1, Kind: "docx", Markdown: "synthetic"})
	if err != nil || string(out) != "synthetic-codec-bytes" {
		t.Fatal("codec process boundary failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New("/bin/sh", entry).Encode(ctx, codecport.Input{}); err == nil {
		t.Fatal("cancelled codec accepted")
	}
}

func TestCodecOutputRejectsOverflowWithoutRetainingExtraBytes(t *testing.T) {
	out := &boundedOutput{limit: 4}
	if _, err := out.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte("5")); err == nil || out.Len() != 4 {
		t.Fatal("codec output was not bounded")
	}
	if New("node", "/entry.js") != nil || New("/node", "entry.js") != nil {
		t.Fatal("ambient executable resolution accepted")
	}
}
