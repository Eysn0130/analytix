package pluginmaterializationauthority

import (
	"bytes"
	"path/filepath"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

func TestPluginMaterializationAuthorityIsIndependentFromFinalEvidenceAuthority(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	final, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(root, "private", "authority", "final-answer-ed25519-v1.json"), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := OpenOrCreateV1(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if plugin.KeyID() == final.KeyID() || bytes.Equal(plugin.PublicKey(), final.PublicKey()) ||
		KeyPathV1(root) == filepath.Join(root, "private", "authority", "final-answer-ed25519-v1.json") {
		t.Fatal("plugin materialization reused the Final Evidence authority")
	}
	if _, err := OpenOrCreateV1(filepath.Join(root, "missing-installation"), true); err == nil {
		t.Fatal("missing plugin authority was recreated while materialization state existed")
	}
}
