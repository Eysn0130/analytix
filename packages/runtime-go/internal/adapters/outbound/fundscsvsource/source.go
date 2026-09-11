package fundscsvsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const maximumSourceBytesV1 = int64(domainnative.FundsCanonicalCSVMaximumSourceBytesV1)

var ErrInvalid = errors.New("funds CSV source is invalid")

type frozenFileV1 struct {
	path   string
	info   os.FileInfo
	sha256 string
	format string
}

// ReadExactV1 pins one canonical regular CSV contained by the canonical
// workspace, then exposes only its fixed byte snapshot during the callback.
// No source path or bytes survive this adapter call.
func ReadExactV1(
	ctx context.Context,
	workspaceInput string,
	sourceInput string,
	use func(context.Context, string, []byte, string, uint64) error,
) error {
	workspaceInput = strings.TrimSpace(workspaceInput)
	sourceInput = strings.TrimSpace(sourceInput)
	if ctx == nil || ctx.Err() != nil || use == nil || workspaceInput == "" || sourceInput == "" ||
		!filepath.IsAbs(workspaceInput) || !filepath.IsAbs(sourceInput) ||
		filepath.Clean(workspaceInput) != workspaceInput || filepath.Clean(sourceInput) != sourceInput {
		return ErrInvalid
	}
	workspace, err := filepath.EvalSymlinks(workspaceInput)
	if err != nil || workspace != workspaceInput {
		return ErrInvalid
	}
	sourcePath, err := filepath.EvalSymlinks(sourceInput)
	if err != nil || sourcePath != sourceInput {
		return ErrInvalid
	}
	relative, err := filepath.Rel(workspace, sourcePath)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return ErrInvalid
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return ErrInvalid
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > maximumSourceBytesV1 {
		return ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumSourceBytesV1+1))
	if err != nil || int64(len(body)) != before.Size() {
		clear(body)
		return ErrInvalid
	}
	defer clear(body)
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) {
		return ErrInvalid
	}
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = 0
	reader.ReuseRecord = true
	recordCount := uint64(0)
	for {
		_, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return ErrInvalid
		}
		recordCount++
		if recordCount > domainnative.FundsCanonicalCSVMaximumSourceRowsV1+1 {
			return ErrInvalid
		}
	}
	if recordCount < 2 {
		return ErrInvalid
	}
	return use(
		ctx,
		workspace,
		body,
		domainsecurity.SHA256Hex(body),
		recordCount-1,
	)
}

// ReadImportExactV1 freezes one owner-only regular CSV or ZIP selected by
// Electron Main. The callback receives no source locator. Its revalidator
// reopens the same host-private path, checks the original file identity and
// metadata, then yields a fresh exact byte snapshot for confirmation.
func ReadImportExactV1(
	ctx context.Context,
	workspaceInput string,
	sourceInput string,
	use func(
		context.Context,
		string,
		string,
		[]byte,
		string,
		func(context.Context, func(context.Context, []byte, string) error) error,
	) error,
) error {
	workspace, sourcePath, format, err := resolveImportSourceV1(ctx, workspaceInput, sourceInput)
	if err != nil || use == nil {
		return ErrInvalid
	}
	body, info, digest, err := readFrozenRegularFileV1(ctx, sourcePath)
	if err != nil {
		return ErrInvalid
	}
	defer clear(body)
	frozen := frozenFileV1{path: sourcePath, info: info, sha256: digest, format: format}
	revalidate := func(
		revalidateContext context.Context,
		consume func(context.Context, []byte, string) error,
	) error {
		if revalidateContext == nil || revalidateContext.Err() != nil || consume == nil {
			return ErrInvalid
		}
		currentBody, currentInfo, currentDigest, readErr := readFrozenRegularFileV1(revalidateContext, frozen.path)
		if readErr != nil {
			return ErrInvalid
		}
		defer clear(currentBody)
		if !os.SameFile(frozen.info, currentInfo) || frozen.info.Size() != currentInfo.Size() ||
			!frozen.info.ModTime().Equal(currentInfo.ModTime()) || frozen.sha256 != currentDigest {
			return ErrInvalid
		}
		return consume(revalidateContext, currentBody, currentDigest)
	}
	return use(ctx, workspace, format, body, digest, revalidate)
}

func resolveImportSourceV1(
	ctx context.Context,
	workspaceInput string,
	sourceInput string,
) (string, string, string, error) {
	workspaceInput = strings.TrimSpace(workspaceInput)
	sourceInput = strings.TrimSpace(sourceInput)
	if ctx == nil || ctx.Err() != nil || workspaceInput == "" || sourceInput == "" ||
		!filepath.IsAbs(workspaceInput) || !filepath.IsAbs(sourceInput) ||
		filepath.Clean(workspaceInput) != workspaceInput || filepath.Clean(sourceInput) != sourceInput {
		return "", "", "", ErrInvalid
	}
	workspace, err := filepath.EvalSymlinks(workspaceInput)
	if err != nil || workspace != workspaceInput {
		return "", "", "", ErrInvalid
	}
	sourcePath, err := filepath.EvalSymlinks(sourceInput)
	if err != nil || sourcePath != sourceInput {
		return "", "", "", ErrInvalid
	}
	relative, err := filepath.Rel(workspace, sourcePath)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", ErrInvalid
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(sourcePath)), ".")
	if format != "csv" && format != "zip" {
		return "", "", "", ErrInvalid
	}
	return workspace, sourcePath, format, nil
}

func readFrozenRegularFileV1(
	ctx context.Context,
	sourcePath string,
) ([]byte, os.FileInfo, string, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, nil, "", ErrInvalid
	}
	pathInfo, err := os.Lstat(sourcePath)
	if err != nil || !safeOwnerRegularFileV1(pathInfo) {
		return nil, nil, "", ErrInvalid
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, nil, "", ErrInvalid
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, before) || !safeOwnerRegularFileV1(before) ||
		before.Size() <= 0 || before.Size() > maximumSourceBytesV1 {
		return nil, nil, "", ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(&contextReaderV1{ctx: ctx, reader: file}, maximumSourceBytesV1+1))
	if err != nil || int64(len(body)) != before.Size() {
		clear(body)
		return nil, nil, "", ErrInvalid
	}
	after, statErr := file.Stat()
	pathAfter, pathErr := os.Lstat(sourcePath)
	if statErr != nil || pathErr != nil || !os.SameFile(before, after) || !os.SameFile(after, pathAfter) ||
		before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) ||
		!safeOwnerRegularFileV1(after) || !safeOwnerRegularFileV1(pathAfter) {
		clear(body)
		return nil, nil, "", ErrInvalid
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	return body, after, digest, nil
}

type contextReaderV1 struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReaderV1) Read(target []byte) (int, error) {
	if reader == nil || reader.ctx == nil || reader.ctx.Err() != nil {
		return 0, context.Canceled
	}
	return reader.reader.Read(target)
}

func safeOwnerRegularFileV1(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeType != 0 {
		return false
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		// Windows owner and link-count identity needs a platform-specific
		// implementation before this acquisition path can be advertised there.
		return false
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false
	}
	nlink := value.FieldByName("Nlink")
	uid := value.FieldByName("Uid")
	current, currentErr := user.Current()
	if !nlink.IsValid() || !nlink.CanUint() || !uid.IsValid() || !uid.CanUint() ||
		currentErr != nil || current == nil {
		return false
	}
	currentUID, uidErr := strconv.ParseUint(current.Uid, 10, 64)
	if uidErr != nil {
		return false
	}
	return nlink.Uint() == 1 && uid.Uint() == currentUID
}
