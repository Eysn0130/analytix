package toolcall

import (
	"bytes"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestToolCallItemIDNeverEmbedsTurnOrProviderBytes(t *testing.T) {
	hostID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x61}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	const turnID = "turn_6222020202020202020"
	itemID := ToolCallItemIDV1(turnID, hostID)
	if !IsToolCallItemIDV1(itemID) || strings.Contains(itemID, turnID) || strings.Contains(itemID, "6222020202020202020") {
		t.Fatalf("tool-call item identity leaked input bytes: %q", itemID)
	}
	if ToolCallItemIDV1(turnID, "provider_call_6222020202020202020") != "" {
		t.Fatal("tool-call item identity accepted a provider-originated identity")
	}
}
