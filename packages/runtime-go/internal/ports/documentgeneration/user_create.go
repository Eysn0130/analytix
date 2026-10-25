package documentgeneration

import (
	"context"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	editing "analytix.local/runtime-go/internal/ports/objectediting"
)

// Workspace and subject are resolved by Core; the visible request is a basename.
type UserCreateInput struct {
	Workspace, FileName, SubjectIdentity, OperationID, RequestHash string
	Content                                                        []byte
	Encode                                                         func(context.Context) ([]byte, error)
	CheckCurrent                                                   func() error
}
type UserCreation struct {
	OperationID string `json:"operationId"`
	Workspace   string `json:"workspace"`
	Path        string `json:"path"`
	Revision    string `json:"revision"`
	Kind        string `json:"kind"`
	ByteSize    int    `json:"byteSize"`
	SavedAt     string `json:"savedAt"`
}
type UserCreationFiles interface {
	CreateUserDocument(context.Context, UserCreateInput) (UserCreation, error)
}

func ValidUserDOCXName(name string) bool {
	if !utf8.ValidString(name) || len(name) < 6 || len(name) > 255 || strings.TrimSpace(name) != name || path.Base(name) != name || !strings.EqualFold(path.Ext(name), ".docx") || strings.TrimSpace(name[:len(name)-5]) == "" {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			return false
		}
	}
	return true
}

// Unknown binds a query/retry to the original operation. It grants no rewrite.
type UserCreationUnknown struct{ OperationID, Revision string }

func (e *UserCreationUnknown) Error() string { return "user_document_creation_unknown" }
func (e *UserCreationUnknown) Unwrap() error { return editing.ErrPersistence }
