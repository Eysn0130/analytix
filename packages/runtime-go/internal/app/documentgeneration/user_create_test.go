package documentgeneration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	editingport "analytix.local/runtime-go/internal/ports/objectediting"
)

type userTestWorkspace struct{ workspace, display string }

func (r *userTestWorkspace) ResolveDocumentWorkspace(context.Context, identitydomain.PrincipalV1, string) (string, string, error) {
	display := r.display
	if display == "" {
		display = r.workspace
	}
	return r.workspace, display, nil
}

type userTestCodec struct {
	seen     codecport.Input
	onEncode func()
}

func (c *userTestCodec) Encode(_ context.Context, input codecport.Input) ([]byte, error) {
	c.seen = input
	if c.onEncode != nil {
		c.onEncode()
	}
	return []byte("codec-docx-fixture"), nil
}

type userTestFiles struct {
	calls          int
	input          codecport.UserCreateInput
	foreignReceipt bool
}

func (f *userTestFiles) CreateUserDocument(_ context.Context, input codecport.UserCreateInput) (codecport.UserCreation, error) {
	if input.Encode != nil {
		content, err := input.Encode(context.Background())
		if err != nil {
			return codecport.UserCreation{}, err
		}
		input.Content = content
	}
	if err := input.CheckCurrent(); err != nil {
		return codecport.UserCreation{}, err
	}
	f.calls++
	f.input = input
	if f.foreignReceipt {
		return codecport.UserCreation{Workspace: "/foreign/thread", Path: "/foreign/thread/" + input.FileName, OperationID: input.OperationID}, nil
	}
	return codecport.UserCreation{Workspace: input.Workspace, Path: input.Workspace + "/" + input.FileName, OperationID: input.OperationID}, nil
}
func TestUserDOCXCreationUsesCurrentThreadAndRevalidatesBeforeWrite(t *testing.T) {
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("a", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	identity := &artifactTestIdentity{principal: principal}
	workspace, codec, files := &userTestWorkspace{workspace: "/core/thread", display: "/Core/Thread"}, &userTestCodec{}, &userTestFiles{}
	creator := NewUserCreator(identity, files)
	held := false
	if err := creator.BindHost(workspace, codec, func(context.Context) (func(), error) { held = true; return func() { held = false }, nil }); err != nil {
		t.Fatal(err)
	}
	codec.onEncode = func() {
		if !held {
			t.Fatal("codec escaped workspace mutation coordinator")
		}
	}
	created, err := creator.Create(context.Background(), principal, "thread_main", "create_word_001", "新建.docx")
	if err != nil || created.Workspace != "/Core/Thread" || created.Path != "/Core/Thread/新建.docx" || files.calls != 1 || held || codec.seen.Kind != "docx" || codec.seen.Markdown != "" {
		t.Fatal(created, err)
	}
	if files.input.SubjectIdentity == "" || files.input.RequestHash != userCreateDigest("user-blank-docx/v1\x00/core/thread\x00新建.docx") || files.input.Workspace != "/core/thread" {
		t.Fatal("lost Core bindings")
	}
	for _, moved := range []string{"/different/thread", "/Core/thread"} {
		workspace.workspace = "/core/thread"
		codec.onEncode = func() { workspace.workspace = moved }
		if _, err := creator.Create(context.Background(), principal, "thread_main", "create_word_002", "other.docx"); !errors.Is(err, editingport.ErrForbidden) || files.calls != 1 {
			t.Fatal("distinct identity accepted during encoding", err)
		}
	}
	workspace.workspace, codec.onEncode, files.foreignReceipt = "/core/thread", nil, true
	if _, err := creator.Create(context.Background(), principal, "thread_main", "create_word_004", "foreign.docx"); !errors.Is(err, editingport.ErrPersistence) {
		t.Fatal("foreign receipt projected into the trusted workspace", err)
	}
	for _, name := range []string{"../outside.docx", "/private.docx", "bad.md", ".docx", "bad\\name.docx"} {
		if _, err := creator.Create(context.Background(), principal, "thread_main", "create_word_003", name); !errors.Is(err, editingport.ErrInvalidInput) {
			t.Fatal("invalid basename", name, err)
		}
	}
}
func TestUserBlankCodecDoesNotWidenAgentOrOtherFormatContract(t *testing.T) {
	body, err := json.Marshal(codecport.Input{SchemaVersion: 1, Kind: "docx"})
	if err != nil || !bytes.Contains(body, []byte(`"markdown":""`)) {
		t.Fatal("blank DOCX markdown omitted", string(body), err)
	}
	if _, err := ParseRequest(map[string]any{"kind": "docx", "path": "agent.docx", "markdown": ""}); err == nil {
		t.Fatal("Agent accepted empty document")
	}
	type legacy codecport.Input
	for _, input := range []codecport.Input{{SchemaVersion: 1, Kind: "docx", Markdown: "existing Agent content"}, {SchemaVersion: 1, Kind: "xlsx", Workbook: json.RawMessage(`{}`)}, {SchemaVersion: 1, Kind: "pptx", Presentation: json.RawMessage(`{}`)}} {
		before, _ := json.Marshal(legacy(input))
		after, _ := json.Marshal(input)
		if !bytes.Equal(before, after) {
			t.Fatal("existing codec bytes changed", input.Kind)
		}
	}
}
