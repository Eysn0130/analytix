package filestore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
)

type ReplaceExistingFile func(relativePath string, sourcePath string, targetPath string) bool

type MoveRegularFileOptions struct {
	Workspace         string
	SourcePath        string
	DestinationPath   string
	SandboxMode       string
	AllowWriteRoots   []string
	MutationAuthority ConditionalMutationAuthority
}

type MoveRegularFilePlan struct {
	SourcePath              string
	DestinationPath         string
	SourceRelativePath      string
	DestinationRelativePath string
	BytesMoved              int64
	Noop                    bool

	sourceHash                string
	sourceMode                os.FileMode
	sourceDevice              uint64
	sourceInode               uint64
	sourceRevision            string
	mutationAuthority         ConditionalMutationAuthority
	operationGroupID          string
	operationDigest           string
	sourceAuthority           string
	sourceRelative            string
	destinationAuthority      string
	destinationRelative       string
	sourceEncoding            string
	sourceBytesBase64         string
	operationBound            bool
	destinationParentMissing  bool
	destinationExistingParent string
	destinationExistingDevice uint64
	destinationExistingInode  uint64
	destinationExistingMode   os.FileMode
	destinationTopMissing     string
	destinationRelativeTail   string
	destinationStageName      string
	testHooks                 *conditionalMoveTestHooks
}

func SamePath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func IsInsideTempDir(path string) bool {
	tempRoot, err := WorkspaceRealPath(os.TempDir())
	if err != nil {
		return false
	}
	resolved, err := WorkspaceRealPath(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(tempRoot), filepath.Clean(resolved))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func IsDirectoryNotEmptyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "directory not empty")
}

func PrepareMoveRegularFile(options MoveRegularFileOptions) (MoveRegularFilePlan, error) {
	source, ok := ResolveWritePathForSandbox(options.Workspace, options.SourcePath, options.SandboxMode, options.AllowWriteRoots)
	if !ok {
		return MoveRegularFilePlan{}, OperationError{Code: "workspace_escape", Message: "source_path must stay inside workspace or configured allow_write root"}
	}
	destination, ok := ResolveWritePathForSandbox(options.Workspace, options.DestinationPath, options.SandboxMode, options.AllowWriteRoots)
	if !ok {
		return MoveRegularFilePlan{}, OperationError{Code: "workspace_escape", Message: "destination_path must stay inside workspace or configured allow_write root"}
	}
	plan := MoveRegularFilePlan{
		SourcePath:              source,
		DestinationPath:         destination,
		SourceRelativePath:      WorkspaceRelativePath(options.Workspace, source),
		DestinationRelativePath: WorkspaceRelativePath(options.Workspace, destination),
	}
	if SamePath(source, destination) {
		observation, err := prepareConditionalMoveSource(source)
		if err != nil {
			return MoveRegularFilePlan{}, OperationError{Code: "stat_failed", Message: err.Error(), Path: source, RelativePath: plan.SourceRelativePath}
		}
		plan.BytesMoved = observation.Size
		plan.Noop = true
		return plan, nil
	}
	observation, err := prepareConditionalMoveSource(source)
	if err != nil {
		return MoveRegularFilePlan{}, OperationError{Code: "stat_failed", Message: err.Error(), Path: source, RelativePath: plan.SourceRelativePath}
	}
	destinationExists, err := prepareConditionalMoveDestination(destination)
	if err != nil {
		return MoveRegularFilePlan{}, OperationError{Code: "stat_failed", Message: err.Error(), Path: destination, RelativePath: plan.DestinationRelativePath}
	}
	if destinationExists {
		return MoveRegularFilePlan{}, OperationError{Code: "destination_exists", Message: "destination_path already exists", Path: destination, RelativePath: plan.DestinationRelativePath}
	}
	plan.BytesMoved = observation.Size
	plan.sourceHash = observation.Hash
	plan.sourceMode = observation.Mode
	plan.sourceDevice = observation.Device
	plan.sourceInode = observation.Inode
	plan.sourceRevision = observation.Revision
	plan.mutationAuthority = options.MutationAuthority
	return plan, nil
}

func ApplyMoveRegularFile(plan MoveRegularFilePlan) error {
	if plan.Noop {
		return nil
	}
	if err := applyConditionalMove(plan); err != nil {
		return OperationError{Code: "move_failed", Message: fmt.Sprintf("move %s to %s: %v", plan.SourcePath, plan.DestinationPath, err)}
	}
	return nil
}

// BindMoveRegularFilePlanToIntent makes the durable checkpoint operation
// intent, rather than a journal path or filename, the authority for a move.
// The returned plan is the only plan eligible to enter the private move
// journal.
func BindMoveRegularFilePlanToIntent(
	plan MoveRegularFilePlan,
	intent domaincheckpoint.OperationGroupIntentV2,
) (MoveRegularFilePlan, error) {
	if plan.Noop {
		return plan, nil
	}
	if domaincheckpoint.ValidateOperationGroupIntentV2(intent) != nil || intent.ToolName != "move_file" ||
		intent.OperationGroupID == "" || intent.IntentDigest == "" || len(intent.Paths) != 2 {
		return MoveRegularFilePlan{}, errors.New("conditional move operation intent is invalid")
	}
	var source, destination *domaincheckpoint.OperationPathV2
	for index := range intent.Paths {
		item := &intent.Paths[index]
		switch item.Role {
		case "source":
			source = item
		case "destination":
			destination = item
		}
	}
	if source == nil || destination == nil {
		return MoveRegularFilePlan{}, errors.New("conditional move operation intent roles are incomplete")
	}
	if !sameMoveIntentAbsolutePath(*source, plan.SourcePath) || !sameMoveIntentAbsolutePath(*destination, plan.DestinationPath) {
		return MoveRegularFilePlan{}, errors.New("conditional move operation intent paths do not match the prepared plan")
	}
	if !source.BeforeExisted || !source.BeforeAvailable || source.BeforeHash != plan.sourceHash || source.BeforeSizeBytes != plan.BytesMoved {
		return MoveRegularFilePlan{}, errors.New("conditional move operation intent source before-state does not match the prepared plan")
	}
	if source.ExpectedAfterExisted || destination.BeforeExisted || !destination.ExpectedAfterExisted || destination.ExpectedAfterHash != plan.sourceHash {
		return MoveRegularFilePlan{}, errors.New("conditional move operation intent destination transition does not match the prepared plan")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(source.BeforeBytesBase64)
	if err != nil || int64(len(raw)) != plan.BytesMoved || digestAtomicText(raw) != plan.sourceHash {
		return MoveRegularFilePlan{}, errors.New("conditional move source snapshot does not match its prepared bytes")
	}
	plan.operationGroupID = intent.OperationGroupID
	plan.operationDigest = intent.IntentDigest
	plan.sourceAuthority = source.AuthorityRootHash
	plan.sourceRelative = source.RelativePath
	plan.destinationAuthority = destination.AuthorityRootHash
	plan.destinationRelative = destination.RelativePath
	plan.sourceEncoding = source.BeforeEncoding
	plan.sourceBytesBase64 = source.BeforeBytesBase64
	plan.operationBound = true
	return plan, nil
}

func moveIntentAbsolutePath(item domaincheckpoint.OperationPathV2) string {
	return filepath.Clean(filepath.Join(item.AuthorityRoot, filepath.FromSlash(item.RelativePath)))
}

func sameMoveIntentAbsolutePath(item domaincheckpoint.OperationPathV2, prepared string) bool {
	intentPath, intentErr := WorkspaceRealPath(moveIntentAbsolutePath(item))
	preparedPath, preparedErr := WorkspaceRealPath(prepared)
	return intentErr == nil && preparedErr == nil && filepath.Clean(intentPath) == filepath.Clean(preparedPath)
}

func CopyMissingDirectory(sourceDir string, targetDir string, shouldReplace ReplaceExistingFile) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			return err
		}
		targetExists := false
		if targetInfo, err := os.Stat(targetPath); err == nil {
			if !targetInfo.Mode().IsRegular() {
				return nil
			}
			if targetInfo.Size() > 0 && (shouldReplace == nil || !shouldReplace(relativePath, path, targetPath)) {
				return nil
			}
			targetExists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		flags := os.O_CREATE | os.O_WRONLY
		if targetExists {
			flags |= os.O_TRUNC
		} else {
			flags |= os.O_EXCL
		}
		output, err := os.OpenFile(targetPath, flags, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(output, input); err != nil {
			_ = output.Close()
			return err
		}
		return output.Close()
	})
}

// CopyMissingDirectoryLossless refuses a divergent existing target before the
// first write in this directory. It is used for legacy startup inputs whose
// source must never be deleted after a silent skip.
func CopyMissingDirectoryLossless(sourceDir string, targetDir string, shouldReplace ReplaceExistingFile) error {
	if err := ValidateLosslessDirectoryMerge(sourceDir, targetDir, shouldReplace); err != nil {
		return err
	}
	return CopyMissingDirectory(sourceDir, targetDir, shouldReplace)
}

func ValidateLosslessDirectoryMerge(sourceDir string, targetDir string, shouldReplace ReplaceExistingFile) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relativePath)
		sourceInfo, err := entry.Info()
		if err != nil || sourceInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("legacy directory merge contains an unsafe source entry")
		}
		targetInfo, targetErr := os.Lstat(targetPath)
		if errors.Is(targetErr, os.ErrNotExist) {
			return nil
		}
		if targetErr != nil || targetInfo.Mode()&os.ModeSymlink != 0 || sourceInfo.IsDir() != targetInfo.IsDir() {
			return errors.New("legacy directory merge target type conflicts with source")
		}
		if sourceInfo.IsDir() {
			return nil
		}
		if !sourceInfo.Mode().IsRegular() || !targetInfo.Mode().IsRegular() {
			return errors.New("legacy directory merge requires regular files")
		}
		if targetInfo.Size() == 0 || shouldReplace != nil && shouldReplace(relativePath, path, targetPath) {
			return nil
		}
		sourceBody, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		targetBody, err := os.ReadFile(targetPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(sourceBody, targetBody) {
			return errors.New("legacy directory merge would discard divergent data")
		}
		return nil
	})
}
