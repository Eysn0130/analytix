package datasetsnapshot

import (
	"reflect"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestCanonicalCurrentSelectionContentDigestV2BindsCompleteDurableGraph(t *testing.T) {
	selection := CurrentSelectionV2{DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{{}}}
	baseline, err := CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	// Enumerate every scalar field, including every Bundle field and the full
	// nested snapshot graph. Empty slices/maps are also changed to nonempty
	// values, so no durable container can silently drop out of the binding.
	var check func(reflect.Value, string)
	check = func(value reflect.Value, path string) {
		if path == "Head.Request" || path == "Head.Observation" || path == "SelectionDigest" {
			return
		}
		switch value.Kind() {
		case reflect.Struct:
			for i := 0; i < value.NumField(); i++ {
				name := value.Type().Field(i).Name
				if path != "" {
					name = path + "." + name
				}
				check(value.Field(i), name)
			}
		case reflect.Array:
			for i := 0; i < value.Len(); i++ {
				check(value.Index(i), path)
			}
		case reflect.Slice:
			if value.Len() > 0 {
				check(value.Index(0), path+"[0]")
				return
			}
			original := reflect.New(value.Type()).Elem()
			original.Set(value)
			value.Set(reflect.MakeSlice(value.Type(), 1, 1))
			digest, err := CanonicalCurrentSelectionContentDigestV2(selection)
			if err == nil && digest == baseline {
				t.Errorf("unbound durable container %s", path)
			}
			value.Set(original)
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int64, reflect.Uint64:
			original := reflect.New(value.Type()).Elem()
			original.Set(value)
			switch value.Kind() {
			case reflect.String:
				value.SetString("changed")
			case reflect.Bool:
				value.SetBool(!value.Bool())
			case reflect.Int, reflect.Int64:
				value.SetInt(value.Int() + 1)
			case reflect.Uint64:
				value.SetUint(value.Uint() + 1)
			}
			digest, err := CanonicalCurrentSelectionContentDigestV2(selection)
			if err == nil && digest == baseline {
				t.Errorf("unbound durable field %s", path)
			}
			value.Set(original)
		default:
			t.Fatalf("uncovered selection field %s: %s", path, value.Kind())
		}
	}
	check(reflect.ValueOf(&selection).Elem(), "")
	selection.Head.Request.ChallengeNonce = "fresh-request"
	selection.Head.Observation.ChallengeNonce = "fresh-observation"
	selection.Head.Observation.WitnessSignature = "fresh-signature"
	selection.SelectionDigest = "self-digest"
	digest, err := CanonicalCurrentSelectionContentDigestV2(selection)
	if err != nil || digest != baseline {
		t.Fatal("fresh witness exchange changed durable content binding")
	}
	full, _ := CanonicalCurrentSelectionDigestV2(selection)
	if full == digest {
		t.Fatal("full witness and content digests lost domain separation")
	}
}

func TestCanonicalCurrentSelectionDigestV2RejectsSyntaxOnlyAndGraphDrift(t *testing.T) {
	selection := CurrentSelectionV2{
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{{
			IndexDigest: domainsecurity.SHA256Hex([]byte("selection-index")),
		}},
		SelectionDigest: domainsecurity.SHA256Hex([]byte("syntax-only")),
	}
	digest, err := CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	selection.SelectionDigest = digest
	if err := ValidateCurrentSelectionDigestV2(selection); err != nil {
		t.Fatalf("canonical selection digest failed validation: %v", err)
	}
	drifted := selection
	drifted.DatasetIndexPath = append([]domainsecurity.DatasetSnapshotIndexV1(nil), selection.DatasetIndexPath...)
	drifted.DatasetIndexPath[0].IndexDigest = domainsecurity.SHA256Hex([]byte("selection-index-drift"))
	if err := ValidateCurrentSelectionDigestV2(drifted); err == nil {
		t.Fatal("selection graph drift retained digest authority")
	}
}
