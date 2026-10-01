package officeengineassets

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	domainauthority "analytix.local/runtime-go/internal/domain/packagedbuildauthority"
)

//go:embed private-local-layout.json
var privateLayoutBytes []byte

const privateContract = "analytix.office-private-local/v1"
const privateDomain = "AnalytixOfficePrivateLocalV1\x00"
const privateMaxQualification = 64 << 10
const privateMaxTotal = 320 << 20

type privateLayoutFile struct {
	Path       string  `json:"path"`
	Source     string  `json:"source"`
	Owner      string  `json:"owner"`
	Maximum    int64   `json:"maximum"`
	ByteLength *int64  `json:"byteLength"`
	SHA256     *string `json:"sha256"`
}
type privateLayout struct {
	SchemaVersion  int                 `json:"schemaVersion"`
	Contract       string              `json:"contract"`
	BuildID        string              `json:"buildId"`
	ManifestSHA256 string              `json:"manifestSha256"`
	Files          []privateLayoutFile `json:"files"`
}
type privateFile struct {
	Path       string `json:"path"`
	ByteLength int64  `json:"byteLength"`
	SHA256     string `json:"sha256"`
}

// Field order is the JS wire contract, including embedded body fields.
type privateBody struct {
	SchemaVersion          int           `json:"schemaVersion"`
	Contract               string        `json:"contract"`
	Usage                  string        `json:"usage"`
	Publishable            bool          `json:"publishable"`
	ReleaseEligible        bool          `json:"releaseEligible"`
	TargetKey              string        `json:"targetKey"`
	SourceCommit           string        `json:"sourceCommit"`
	WorktreeSnapshotDigest string        `json:"worktreeSnapshotDigest"`
	EngineBuildID          string        `json:"engineBuildId"`
	EngineManifestSHA256   string        `json:"engineManifestSha256"`
	Files                  []privateFile `json:"files"`
}
type privateQualification struct {
	privateBody
	QualificationDigest string `json:"qualificationDigest"`
}
type privateStamp struct {
	device, inode, links                                               uint64
	mode, uid, gid                                                     uint32
	size, modifiedSeconds, modifiedNanos, changedSeconds, changedNanos int64
}
type privateObservation struct {
	path  string
	stamp privateStamp
}

// PrivateLocal is a content witness derived only from an already verified
// packaged inspection. It is not package identity or permission to publish.
type PrivateLocal struct {
	root          string
	qualification privateQualification
	files         []privateObservation
	directories   []privateObservation
}

func privateHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func privateRelative(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && !strings.ContainsRune("._/-", c) {
			return false
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
func privateJSON(value any) []byte {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(value) != nil {
		return nil
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}
func privateHash(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func privateDigest(body privateBody) string {
	h := sha256.New()
	_, _ = h.Write([]byte(privateDomain))
	_, _ = h.Write(privateJSON(body))
	return hex.EncodeToString(h.Sum(nil))
}
func privateDecode(raw []byte, target any) error {
	if jsonstrict.Validate(raw, jsonstrict.Options{RequireObject: true, MaxBytes: privateMaxQualification, MaxDepth: 8, MaxTokens: 4096, MaxStringBytes: 1024}) != nil {
		return ErrUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return ErrUnavailable
	}
	return nil
}
func loadPrivateLayout() (privateLayout, error) {
	var layout privateLayout
	if privateDecode(privateLayoutBytes, &layout) != nil || layout.SchemaVersion != 1 || layout.Contract != "analytix.office-private-local-layout/v1" ||
		!privateHex(layout.BuildID, 40) || layout.ManifestSHA256 != privateHash(manifestBytes) || len(layout.Files) != 35 {
		return layout, ErrUnavailable
	}
	byPath := make(map[string]privateLayoutFile)
	previous := ""
	for _, f := range layout.Files {
		if !privateRelative(f.Path) || !privateRelative(f.Source) || f.Path <= previous || f.Path == "qualification.json" || (f.Owner != "repo" && f.Owner != "asset") ||
			f.Maximum <= 0 || f.Maximum > privateMaxTotal || (f.ByteLength == nil) != (f.SHA256 == nil) || (f.Owner == "asset" && f.SHA256 == nil) {
			return layout, ErrUnavailable
		}
		if f.ByteLength != nil && (*f.ByteLength <= 0 || *f.ByteLength > f.Maximum || !privateHex(*f.SHA256, 64)) {
			return layout, ErrUnavailable
		}
		byPath[f.Path] = f
		previous = f.Path
	}
	m, ok := byPath["manifest.json"]
	if !ok || m.Owner != "repo" || m.SHA256 == nil || *m.SHA256 != layout.ManifestSHA256 || m.ByteLength == nil || *m.ByteLength != int64(len(manifestBytes)) {
		return layout, ErrUnavailable
	}
	var engine struct {
		BuildID string  `json:"buildId"`
		Assets  []asset `json:"assets"`
	}
	if json.Unmarshal(manifestBytes, &engine) != nil || engine.BuildID != layout.BuildID || len(engine.Assets) != 6 {
		return layout, ErrUnavailable
	}
	for _, asset := range engine.Assets {
		f, ok := byPath["assets/"+asset.Path]
		if !ok || f.Owner != "asset" || f.SHA256 == nil || *f.SHA256 != asset.SHA256 || f.ByteLength == nil || *f.ByteLength != asset.Bytes {
			return layout, ErrUnavailable
		}
	}
	return layout, nil
}
func parsePrivateQualification(raw []byte) (privateQualification, error) {
	var q privateQualification
	layout, err := loadPrivateLayout()
	if err != nil || privateDecode(raw, &q) != nil || q.SchemaVersion != 1 || q.Contract != privateContract || q.Usage != "private-local" || q.Publishable || q.ReleaseEligible ||
		q.TargetKey != "darwin-arm64" || !privateHex(q.SourceCommit, 40) || !privateHex(q.WorktreeSnapshotDigest, 64) || q.EngineBuildID != layout.BuildID || q.EngineManifestSHA256 != layout.ManifestSHA256 ||
		!privateHex(q.QualificationDigest, 64) || len(q.Files) != len(layout.Files) {
		return q, ErrUnavailable
	}
	var total int64
	for i, f := range q.Files {
		expected := layout.Files[i]
		if f.Path != expected.Path || f.ByteLength <= 0 || f.ByteLength > expected.Maximum || !privateHex(f.SHA256, 64) ||
			(expected.ByteLength != nil && f.ByteLength != *expected.ByteLength) || (expected.SHA256 != nil && f.SHA256 != *expected.SHA256) {
			return q, ErrUnavailable
		}
		total += f.ByteLength
	}
	if total > privateMaxTotal || privateDigest(q.privateBody) != q.QualificationDigest || !bytes.Equal(raw, append(privateJSON(q), '\n')) {
		return q, ErrUnavailable
	}
	return q, nil
}
func privateInspectionAllowed(i packagedbuildauthorityfs.InspectionV2) bool {
	a, d := i.Authority.Authority, i.Authority.Development
	return i.PackageAnchor == "macos_nonpublishable_resource_seal" && d != nil && i.Authority.Controlled == nil &&
		d.Kind == domainauthority.DevelopmentDispositionKindV2 && d.TargetKey == "darwin-arm64" && a.TargetKey == "darwin-arm64" &&
		!i.Publishable && !i.FactToolsEnabled && !a.Publishable && !a.ReleaseEligible && !a.PublicationReceiptIssued &&
		privateHex(a.SourceCommit, 40) && a.SourceCommit == a.WorktreeSnapshot.SourceCommit && privateHex(a.WorktreeSnapshot.SnapshotDigest, 64)
}
func privateSafeFile(s privateStamp, maximum int64) bool {
	return s.mode&0170000 == 0100000 && s.mode&0022 == 0 && s.links == 1 && s.size > 0 && s.size <= maximum
}

// Only the private payload root and descendants have this permission rule.
// Host ancestors remain identity-checked (e.g. /Applications or cache roots).
func privateSafePayloadDirectory(s privateStamp) bool {
	return s.mode&0170000 == 0040000 && s.mode&0022 == 0
}
func privateSameDirectory(a, b privateStamp) bool {
	return a.mode&0170000 == 0040000 && b.mode&0170000 == 0040000 && a.device == b.device && a.inode == b.inode && a.mode == b.mode && a.uid == b.uid && a.gid == b.gid
}
func privateChain(root string) ([]privateObservation, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, ErrUnavailable
	}
	var result []privateObservation
	for current := root; ; current = filepath.Dir(current) {
		stamp, err := privateLstat(current)
		if err != nil || stamp.mode&0170000 != 0040000 {
			return nil, ErrUnavailable
		}
		result = append(result, privateObservation{current, stamp})
		if filepath.Dir(current) == current {
			break
		}
	}
	return result, nil
}
func privateDirectoriesCurrent(observations []privateObservation) bool {
	for _, o := range observations {
		s, err := privateLstat(o.path)
		if err != nil || !privateSameDirectory(o.stamp, s) {
			return false
		}
	}
	return true
}

// The held descriptor, pathname, ctime, single-link count and ancestor identity
// must agree before and after every bounded stream. No engine-sized buffer.
func privateRead(ctx context.Context, path string, maximum int64, sink io.Writer) (privateObservation, string, error) {
	empty := privateObservation{}
	if ctx == nil || ctx.Err() != nil {
		return empty, "", ErrUnavailable
	}
	chain, err := privateChain(filepath.Dir(path))
	if err != nil {
		return empty, "", ErrUnavailable
	}
	before, err := privateLstat(path)
	if err != nil || !privateSafeFile(before, maximum) {
		return empty, "", ErrUnavailable
	}
	f, err := privateOpen(path)
	if err != nil {
		return empty, "", ErrUnavailable
	}
	defer f.Close()
	held, err := privateFstat(f)
	if err != nil || before != held {
		return empty, "", ErrUnavailable
	}
	h := sha256.New()
	writer := io.Writer(h)
	if sink != nil {
		writer = io.MultiWriter(h, sink)
	}
	buffer := make([]byte, 64<<10)
	var count int64
	for {
		if ctx.Err() != nil {
			return empty, "", ErrUnavailable
		}
		n, readErr := f.Read(buffer)
		if n > 0 {
			count += int64(n)
			if count > maximum {
				return empty, "", ErrUnavailable
			}
			if _, err := writer.Write(buffer[:n]); err != nil {
				return empty, "", ErrUnavailable
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return empty, "", ErrUnavailable
		}
	}
	after, err := privateFstat(f)
	if err != nil {
		return empty, "", ErrUnavailable
	}
	pathAfter, err := privateLstat(path)
	if err != nil || before != after || after != pathAfter || count != before.size || !privateDirectoriesCurrent(chain) || ctx.Err() != nil {
		return empty, "", ErrUnavailable
	}
	return privateObservation{path, after}, hex.EncodeToString(h.Sum(nil)), nil
}
func privateTree(ctx context.Context, root string, files []privateFile) ([]privateObservation, error) {
	chain, err := privateChain(root)
	if err != nil {
		return nil, ErrUnavailable
	}
	expected := map[string]bool{"qualification.json": true}
	directories := make(map[string]bool)
	for _, f := range files {
		expected[f.Path] = true
		for p := filepath.Dir(f.Path); p != "."; p = filepath.Dir(p) {
			directories[filepath.ToSlash(p)] = true
		}
	}
	seen := 0
	var walk func(string, string) error
	walk = func(path, prefix string) error {
		if ctx == nil || ctx.Err() != nil {
			return ErrUnavailable
		}
		entries, err := privateDirectoryEntries(path)
		if err != nil {
			return ErrUnavailable
		}
		for _, entry := range entries {
			rel := entry.Name()
			if prefix != "" {
				rel = prefix + "/" + rel
			}
			absolute := filepath.Join(path, entry.Name())
			s, err := privateLstat(absolute)
			if err != nil {
				return ErrUnavailable
			}
			if directories[rel] && privateSafePayloadDirectory(s) {
				chain = append(chain, privateObservation{absolute, s})
				if walk(absolute, rel) != nil {
					return ErrUnavailable
				}
			} else if expected[rel] && privateSafeFile(s, privateMaxTotal) {
				seen++
			} else {
				return ErrUnavailable
			}
		}
		return nil
	}
	if walk(root, "") != nil || seen != len(expected) || !privateDirectoriesCurrent(chain) {
		return nil, ErrUnavailable
	}
	return chain, nil
}

func privateDirectoryEntries(path string) ([]os.DirEntry, error) {
	before, err := privateLstat(path)
	if err != nil || !privateSafePayloadDirectory(before) {
		return nil, ErrUnavailable
	}
	f, err := privateOpen(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	held, err := privateFstat(f)
	if err != nil || held != before {
		return nil, ErrUnavailable
	}
	// The entire qualification has only 36 files. Never enumerate an
	// unbounded replacement directory before detecting extra payload.
	entries, err := f.ReadDir(37)
	if (err != nil && err != io.EOF) || len(entries) >= 37 {
		return nil, ErrUnavailable
	}
	after, err := privateFstat(f)
	if err != nil || after != before {
		return nil, ErrUnavailable
	}
	pathAfter, err := privateLstat(path)
	if err != nil || pathAfter != after {
		return nil, ErrUnavailable
	}
	return entries, nil
}

// OpenPrivateLocal accepts only an already verified inspection supplied by
// trusted runtime composition. ResourcesRoot is not read from environment or
// an Office request. The existing package resource seal remains mandatory.
func OpenPrivateLocal(ctx context.Context, inspection packagedbuildauthorityfs.InspectionV2) (*PrivateLocal, error) {
	if ctx == nil || ctx.Err() != nil || !privateInspectionAllowed(inspection) {
		return nil, ErrUnavailable
	}
	if _, err := privateChain(inspection.ResourcesRoot); err != nil {
		return nil, ErrUnavailable
	}
	root := filepath.Join(inspection.ResourcesRoot, "office-private")
	var initial bytes.Buffer
	first, _, err := privateRead(ctx, filepath.Join(root, "qualification.json"), privateMaxQualification, &initial)
	if err != nil {
		return nil, ErrUnavailable
	}
	q, err := parsePrivateQualification(initial.Bytes())
	a := inspection.Authority.Authority
	if err != nil || q.SourceCommit != a.SourceCommit || q.WorktreeSnapshotDigest != a.WorktreeSnapshot.SnapshotDigest || q.TargetKey != a.TargetKey {
		return nil, ErrUnavailable
	}
	directories, err := privateTree(ctx, root, q.Files)
	if err != nil {
		return nil, ErrUnavailable
	}
	result := &PrivateLocal{root: root, qualification: q, directories: directories}
	for _, expected := range q.Files {
		o, hash, err := privateRead(ctx, filepath.Join(root, filepath.FromSlash(expected.Path)), expected.ByteLength, nil)
		if err != nil || hash != expected.SHA256 || o.stamp.size != expected.ByteLength {
			return nil, ErrUnavailable
		}
		result.files = append(result.files, o)
	}
	var final bytes.Buffer
	last, _, err := privateRead(ctx, first.path, privateMaxQualification, &final)
	if err != nil || first.stamp != last.stamp || !bytes.Equal(initial.Bytes(), final.Bytes()) {
		return nil, ErrUnavailable
	}
	result.files = append(result.files, last)
	if !result.Current(ctx) {
		return nil, ErrUnavailable
	}
	return result, nil
}
func (p *PrivateLocal) Root() string {
	if p == nil {
		return ""
	}
	return p.root
}
func (p *PrivateLocal) AssetRoot() string {
	if p == nil {
		return ""
	}
	return filepath.Join(p.root, "assets")
}
func (p *PrivateLocal) QualificationDigest() string {
	if p == nil {
		return ""
	}
	return p.qualification.QualificationDigest
}

// Current detects pathname replacement, content writes (including restored
// mtime), link aliases and closed-tree changes. It does not reissue a seal.
func (p *PrivateLocal) Current(ctx context.Context) bool {
	if p == nil || ctx == nil || ctx.Err() != nil || len(p.files) != 36 || len(p.qualification.Files) != 35 || !privateDirectoriesCurrent(p.directories) {
		return false
	}
	if _, err := privateTree(ctx, p.root, p.qualification.Files); err != nil {
		return false
	}
	for _, o := range p.files {
		s, err := privateLstat(o.path)
		if err != nil || !privateSafeFile(s, privateMaxTotal) || s != o.stamp {
			return false
		}
	}
	return ctx.Err() == nil && privateDirectoriesCurrent(p.directories)
}
