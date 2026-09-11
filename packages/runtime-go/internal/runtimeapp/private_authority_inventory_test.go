package runtimeapp

import "testing"

func TestPrivateAuthorityInventoryV1RequiresExistingAuthorityForV2RecordsAlone(t *testing.T) {
	if (privateAuthorityInventoryV1{}).Required() {
		t.Fatal("empty private inventory unexpectedly required an existing final authority")
	}
	if !(privateAuthorityInventoryV1{ControlledAccessV2: true}).Required() {
		t.Fatal("V2 controlled-access records did not require the existing installation authority")
	}
	if !(privateAuthorityInventoryV1{EvidenceAuthority: true}).Required() {
		t.Fatal("shared evidence authority records did not require the existing installation authority")
	}
	if !(privateAuthorityInventoryV1{CaseEntity: true}).Required() {
		t.Fatal("case entity records did not require the existing installation authority")
	}
}
