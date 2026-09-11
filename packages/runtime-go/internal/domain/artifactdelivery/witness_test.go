package artifactdelivery

import (
	"reflect"
	"testing"
)

func TestVerifiedArtifactDeliveryCarriesNoRawOrLocationBearingField(t *testing.T) {
	typeOf := reflect.TypeOf(VerifiedArtifactDeliveryV1{})
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map ||
			field.Name == "Path" || field.Name == "Body" || field.Name == "Token" ||
			field.Name == "ControlledHandle" {
			t.Fatalf("verified artifact delivery exposes forbidden field %s %s", field.Name, field.Type)
		}
	}
}
