package toolresult

import (
	"bytes"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestToolResultItemIDIsOpaqueStableAndLengthDelimited(t *testing.T) {
	const account = "6222020202020202020"
	hostID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x62}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	first := ToolResultItemIDV1("turn-a", hostID)
	if first == "" || !IsToolResultItemIDV1(first) || first != ToolResultItemIDV1("turn-a", hostID) {
		t.Fatalf("tool-result item identity is not stable: %q", first)
	}
	if strings.Contains(first, account) || strings.Contains(first, "turn-a") {
		t.Fatalf("tool-result item identity leaked source bytes: %q", first)
	}
	otherHostID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x63}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	if ToolResultItemIDV1("turn-a_b", hostID) == ToolResultItemIDV1("turn-a", otherHostID) {
		t.Fatal("tool-result item identity has a delimiter collision")
	}
	if ToolResultItemIDV1("", hostID) != "" || ToolResultItemIDV1("turn-a", "call_host_"+account) != "" || IsToolResultItemIDV1("item_result_not-a-digest") {
		t.Fatal("tool-result item identity accepted an invalid value")
	}
}
