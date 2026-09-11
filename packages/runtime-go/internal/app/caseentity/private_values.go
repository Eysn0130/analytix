package caseentity

import "errors"

func (DeriveReferenceInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case entity reference input does not support ordinary JSON serialization")
}

func (*DeriveReferenceInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case entity reference input does not support ordinary JSON deserialization")
}

func (DeriveReferenceInputV1) String() string {
	return "DeriveReferenceInputV1{private:[REDACTED]}"
}

func (input DeriveReferenceInputV1) GoString() string {
	return input.String()
}

func (ResolveReferenceInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case entity reference resolution input does not support ordinary JSON serialization")
}

func (*ResolveReferenceInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case entity reference resolution input does not support ordinary JSON deserialization")
}

func (ResolveReferenceInputV1) String() string {
	return "ResolveReferenceInputV1{private:[REDACTED]}"
}

func (input ResolveReferenceInputV1) GoString() string {
	return input.String()
}

func (PersistIngressInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case ingress persistence input does not support ordinary JSON serialization")
}

func (*PersistIngressInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case ingress persistence input does not support ordinary JSON deserialization")
}

func (PersistIngressInputV1) String() string {
	return "PersistIngressInputV1{private:[REDACTED]}"
}

func (input PersistIngressInputV1) GoString() string {
	return input.String()
}

func (CompileAccountIngressInputV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case account ingress compilation input does not support ordinary JSON serialization")
}

func (*CompileAccountIngressInputV1) UnmarshalJSON([]byte) error {
	return errors.New("case account ingress compilation input does not support ordinary JSON deserialization")
}

func (CompileAccountIngressInputV1) String() string {
	return "CompileAccountIngressInputV1{private:[REDACTED]}"
}

func (input CompileAccountIngressInputV1) GoString() string {
	return input.String()
}

func (AccountIngressCandidateBatchV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case account ingress candidate batch does not support ordinary JSON serialization")
}

func (*AccountIngressCandidateBatchV1) UnmarshalJSON([]byte) error {
	return errors.New("case account ingress candidate batch does not support ordinary JSON deserialization")
}

func (AccountIngressCandidateBatchV1) String() string {
	return "AccountIngressCandidateBatchV1{private:[REDACTED]}"
}

func (batch AccountIngressCandidateBatchV1) GoString() string {
	return batch.String()
}

func (ProviderIngressProjectionV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case provider ingress projection does not support ordinary JSON serialization")
}

func (*ProviderIngressProjectionV1) UnmarshalJSON([]byte) error {
	return errors.New("case provider ingress projection does not support ordinary JSON deserialization")
}

func (ProviderIngressProjectionV1) String() string {
	return "ProviderIngressProjectionV1{private:[REDACTED]}"
}

func (projection ProviderIngressProjectionV1) GoString() string {
	return projection.String()
}

func (AccountIngressCompilationV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case account ingress compilation does not support ordinary JSON serialization")
}

func (*AccountIngressCompilationV1) UnmarshalJSON([]byte) error {
	return errors.New("case account ingress compilation does not support ordinary JSON deserialization")
}

func (AccountIngressCompilationV1) String() string {
	return "AccountIngressCompilationV1{private:[REDACTED]}"
}

func (result AccountIngressCompilationV1) GoString() string {
	return result.String()
}

func (PrivateRecordReferenceV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("case private record reference does not support ordinary JSON serialization")
}

func (*PrivateRecordReferenceV1) UnmarshalJSON([]byte) error {
	return errors.New("case private record reference does not support ordinary JSON deserialization")
}

func (PrivateRecordReferenceV1) String() string {
	return "PrivateRecordReferenceV1{private:[REDACTED]}"
}

func (reference PrivateRecordReferenceV1) GoString() string {
	return reference.String()
}
