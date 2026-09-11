//go:build !darwin && !linux && !windows

package finalauthority

import (
	"context"
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type privateCASShardIdentity struct{}
type privateCASRootAuthority struct{}
type privateCASRootAnchor struct{}
type privateCASShardAnchor struct{}
type privateCASPreparedRecoveryPlan struct{}

func privateCASOpenRootAnchor(privateCASRootAuthority) (privateCASRootAnchor, error) {
	return privateCASRootAnchor{}, errors.New("private CAS secure storage is unsupported")
}

func privateCASValidateRootAnchor(privateCASRootAnchor, privateCASRootAuthority) error {
	return errors.New("private CAS secure storage is unsupported")
}

func privateCASCloseRootAnchor(privateCASRootAnchor) error { return nil }

func privateCASOpenShardAnchor(privateCASRootAuthority, string, privateCASShardIdentity) (privateCASShardAnchor, error) {
	return privateCASShardAnchor{}, errors.New("private CAS secure storage is unsupported")
}

func privateCASValidateShardAnchor(privateCASRootAuthority, string, privateCASShardIdentity, privateCASShardAnchor) error {
	return errors.New("private CAS secure storage is unsupported")
}

func privateCASCloseShardAnchor(privateCASShardAnchor) error { return nil }

func newPrivateCASRootAuthority(privatecasport.RootBinding) (privateCASRootAuthority, error) {
	return privateCASRootAuthority{}, errors.New("private CAS secure storage is unsupported")
}

func existingPrivateCASRootAuthority(privatecasport.RootBinding) (privateCASRootAuthority, bool, error) {
	return privateCASRootAuthority{}, false, errors.New("private CAS secure storage is unsupported")
}

func securePreflightPrivateCASRoot(context.Context, privateCASRootAuthority, int) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASObserveRecovery(context.Context, privateCASRootAuthority, int) (privateCASRecoveryObservation, error) {
	return privateCASRecoveryObservation{}, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASValidateNoUnsignedRecoveryPhasesV4(
	context.Context,
	privateCASRootAuthority,
	int,
	bool,
) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASReadPreparedCommitted(
	context.Context,
	privateCASRootAuthority,
	privateCASRecoveryObservation,
	int,
	string,
) ([]byte, error) {
	return nil, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASCreateRecoveryMarker(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int, string) (bool, error) {
	return false, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASStagePreparedRecovery(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int, string) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASRollbackPreparedRecovery(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASMarkPreparedRecoveryCommit(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int, string) (bool, error) {
	return false, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASCommitPreparedRecovery(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int, string, bool) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASFinalizePreparedRecoveryCommit(context.Context, privateCASRootAuthority, *privateCASRecoveryObservation, int, string) error {
	return errors.New("private CAS secure storage is unsupported")
}

func capturePrivateCASShardIdentities(privateCASRootAuthority) (map[string]privateCASShardIdentity, error) {
	return nil, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASWrite(privateCASRootAuthority, map[string]privateCASShardIdentity, string, []byte, int) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASWriteWithStageHook(privateCASRootAuthority, map[string]privateCASShardIdentity, string, []byte, int, func()) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASWriteWithAdditionReceipt(
	privateCASRootAuthority,
	map[string]privateCASShardIdentity,
	string,
	[]byte,
	int,
	func(),
	...privateCASOriginalWriteValidationV1,
) (privateCASAdditionObservation, bool, error) {
	return privateCASAdditionObservation{}, false, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASRead(privateCASRootAuthority, map[string]privateCASShardIdentity, string, int) ([]byte, error) {
	return nil, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASList(context.Context, privateCASRootAuthority, map[string]privateCASShardIdentity, int) ([]SecurePrivateCASFile, error) {
	return nil, errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASVisit(context.Context, privateCASRootAuthority, map[string]privateCASShardIdentity, int, func(SecurePrivateCASFile) error) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASValidateInventory(privateCASRootAuthority, map[string]privateCASShardIdentity, int) error {
	return errors.New("private CAS secure storage is unsupported")
}

func securePrivateCASObserveAddition(
	privateCASRootAuthority,
	map[string]privateCASShardIdentity,
	string,
	int,
	...privateCASOriginalWriteValidationV1,
) (privateCASAdditionObservation, error) {
	return privateCASAdditionObservation{}, errors.New("private CAS secure storage is unsupported")
}
