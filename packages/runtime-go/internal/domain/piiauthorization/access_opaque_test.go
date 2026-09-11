package piiauthorization

import "testing"

func TestControlledAccessOpaqueDigestsAreDomainSeparatedAndStrict(t *testing.T) {
	token := "controlled-access-opaque-token-v1"
	handle, handleErr := ControlledAccessHandleDigestV1(token)
	slot, slotErr := ControlledAccessUseSlotDigestV1(token)
	principal, principalErr := ControlledAccessRendererPrincipalDigestV1(token)
	if handleErr != nil || slotErr != nil || principalErr != nil || handle == slot || handle == principal || slot == principal {
		t.Fatalf("controlled access opaque token domains collided: handle=%s slot=%s principal=%s errors=%v/%v/%v",
			handle, slot, principal, handleErr, slotErr, principalErr)
	}
	for _, invalid := range []string{
		"short", " leading-controlled-token-v1", "trailing-controlled-token-v1 ",
		"controlled/token/with/slashes", "controlled-token-with-newline\n",
	} {
		if digest, err := ControlledAccessHandleDigestV1(invalid); err == nil || digest != "" {
			t.Fatalf("invalid controlled access token acquired a digest: token=%q digest=%s err=%v", invalid, digest, err)
		}
	}
}

func TestControlledAccessOpaqueV2CannotReuseLegacyAdmissionDigests(t *testing.T) {
	token := "controlled-access-opaque-token-v2"
	v1Handle, _ := ControlledAccessHandleDigestV1(token)
	v1Slot, _ := ControlledAccessUseSlotDigestV1(token)
	v1Principal, _ := ControlledAccessRendererPrincipalDigestV1(token)
	v2Handle, handleErr := ControlledAccessHandleDigestV2(token)
	v2Slot, slotErr := ControlledAccessUseSlotDigestV2(token)
	v2Principal, principalErr := ControlledAccessRendererPrincipalDigestV2(token)
	if handleErr != nil || slotErr != nil || principalErr != nil ||
		v2Handle == v1Handle || v2Slot == v1Slot || v2Principal == v1Principal ||
		v2Handle == v2Slot || v2Handle == v2Principal || v2Slot == v2Principal {
		t.Fatalf("V2 controlled access admission domains are not isolated: v1=%s/%s/%s v2=%s/%s/%s errors=%v/%v/%v",
			v1Handle, v1Slot, v1Principal, v2Handle, v2Slot, v2Principal, handleErr, slotErr, principalErr)
	}
}
