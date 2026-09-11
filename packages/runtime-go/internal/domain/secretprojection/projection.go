package secretprojection

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	RedactedV1 = "<redacted>"

	// Public projection is intentionally bounded because every value handled by
	// this package may originate outside the host trust boundary. Oversized
	// values are projected to RedactedV1 and validators reject them instead of
	// spending unbounded CPU, stack, or allocation budget on recursive data.
	maxTextBytesV1       = 1 << 20
	maxAggregateBytesV1  = 8 << 20
	maxValueDepthV1      = 64
	maxValueNodesV1      = 10_000
	maxExplicitSecretsV1 = 4_096

	credentialNamePatternV1 = `(?:authorization|proxy[-_ ]?authorization|x[-_ ]?api[-_ ]?key|api[-_ ]?key|apikey|access[-_ ]?token|refresh[-_ ]?token|id[-_ ]?token|aws[-_ ]?access[-_ ]?key[-_ ]?id|aws[-_ ]?secret[-_ ]?access[-_ ]?key|aws[-_ ]?(?:security|session)[-_ ]?token|github[-_ ]?token|slack[-_ ]?token|session[-_ ]?token|client[-_ ]?secret|password|passwd|pwd|secret|credential|cookie|[A-Za-z][A-Za-z0-9_. -]{0,95}(?:token|secret|password|credential|api[-_. ]?key))`
)

var ErrCredentialMaterialV1 = errors.New("public value contains credential material")

var ErrProjectionLimitV1 = errors.New("public value exceeds credential projection limits")

// urlCandidateV1 keeps a Markdown code delimiter outside an unescaped URL.
// This prevents benign URL serialization from looking like credential
// redaction while the URL body still receives the full scan.
var (
	credentialAssignmentV1  = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])["']?(` + credentialNamePatternV1 + `)["']?\s*[:=]\s*(?:"([^"]*)"|'([^']*)'|([^"',;\s}\]]+))`)
	credentialFlagV1        = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])--(` + credentialNamePatternV1 + `)(?:=|\s+)(?:"([^"]*)"|'([^']*)'|([^"',;\s}\]]+))`)
	authSchemeV1            = regexp.MustCompile(`(?i)\b(bearer|basic)\s+(?:"([^"]*)"|'([^']*)'|([A-Za-z0-9._~+/=-]+))`)
	knownTokenV1            = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr][_-][A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,}|xox[baprsocd]-[-A-Za-z0-9.]{8,}|xapp-[-A-Za-z0-9.]{8,}|xoxe(?:\.xoxp)?-[-A-Za-z0-9.]{8,})\b`)
	awsAccessKeyV1          = regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)
	privateKeyBlockV1       = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	privateKeyRemainderV1   = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*$`)
	urlCandidateV1          = regexp.MustCompile("(?i)\\b[a-z][a-z0-9+.-]*://[^\\s\"'<>`]+")
	credentialKeyReplacerV1 = strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
)

// ProjectTextV1 removes credential material from untrusted process and
// provider text. It preserves non-secret diagnostics; callers handling a
// security-bound process may choose the stronger empty projection instead.
func ProjectTextV1(text string, explicitSecrets ...string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if len(text) > maxTextBytesV1 || len(explicitSecrets) > maxExplicitSecretsV1 {
		return RedactedV1
	}
	for _, secret := range explicitSecrets {
		if len(secret) > maxTextBytesV1 {
			return RedactedV1
		}
	}
	hints := buildCredentialScanHintsV1(text)
	if len(explicitSecrets) == 0 && !hints.any() {
		return text
	}
	// Redact URLs before inserting the angle-bracket placeholder. Otherwise a
	// placeholder in userinfo/query text terminates the URL matcher and can
	// leave a duplicated suffix outside the redacted URL.
	out := text
	if hints.url {
		out = urlCandidateV1.ReplaceAllStringFunc(out, redactURLV1)
	}
	secrets := append([]string(nil), explicitSecrets...)
	secrets = append(secrets, extractExplicitSecretsWithHintsV1(text, hints)...)
	sort.SliceStable(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if len(secret) < 4 || secret == RedactedV1 {
			continue
		}
		out = strings.ReplaceAll(out, secret, RedactedV1)
	}
	// URL normalization may decode an otherwise hidden token-shaped value (for
	// example %73k-...). Re-scan the projected text before the final passes so
	// a transformation cannot introduce credential material behind stale hints.
	postHints := buildCredentialScanHintsV1(out)
	if postHints.privateKey {
		out = privateKeyBlockV1.ReplaceAllString(out, RedactedV1)
		out = privateKeyRemainderV1.ReplaceAllString(out, RedactedV1)
	}
	if postHints.knownToken {
		out = knownTokenV1.ReplaceAllString(out, RedactedV1)
	}
	if postHints.awsAccessKey {
		out = awsAccessKeyV1.ReplaceAllString(out, RedactedV1)
	}
	if postHints.authScheme {
		out = authSchemeV1.ReplaceAllStringFunc(out, func(match string) string {
			parts := authSchemeV1.FindStringSubmatch(match)
			if len(parts) < 3 || normalizedCredentialLiteralV1(firstNonEmptyV1(parts[2:]...)) == RedactedV1 {
				return match
			}
			return parts[1] + " " + RedactedV1
		})
	}
	if postHints.assignment {
		out = credentialAssignmentV1.ReplaceAllStringFunc(out, func(match string) string {
			parts := credentialAssignmentV1.FindStringSubmatch(match)
			if len(parts) < 4 {
				return RedactedV1
			}
			rawValue := firstNonEmptyV1(parts[3:]...)
			if !credentialTextKeyHasMaterialV1(parts[2], rawValue) {
				return match
			}
			return parts[1] + parts[2] + "=" + RedactedV1
		})
	}
	if postHints.flag {
		out = credentialFlagV1.ReplaceAllStringFunc(out, func(match string) string {
			parts := credentialFlagV1.FindStringSubmatch(match)
			if len(parts) < 4 {
				return "--credential=" + RedactedV1
			}
			rawValue := firstNonEmptyV1(parts[3:]...)
			if !credentialTextKeyHasMaterialV1(parts[2], rawValue) {
				return match
			}
			return parts[1] + "--" + parts[2] + "=" + RedactedV1
		})
	}
	return out
}

type credentialScanHintsV1 struct {
	url          bool
	assignment   bool
	flag         bool
	authScheme   bool
	knownToken   bool
	awsAccessKey bool
	privateKey   bool
}

func (hints credentialScanHintsV1) any() bool {
	return hints.url || hints.assignment || hints.flag || hints.authScheme || hints.knownToken ||
		hints.awsAccessKey || hints.privateKey
}

// buildCredentialScanHintsV1 identifies necessary lexical features before running
// the more expensive credential regular expressions. The folded view preserves
// every Unicode simple-fold equivalent of an ASCII letter that Go's (?i)
// matcher accepts, so this optimization cannot turn a prior match into a miss.
func buildCredentialScanHintsV1(text string) credentialScanHintsV1 {
	hints := credentialScanHintsV1{
		url:          strings.Contains(text, "://"),
		knownToken:   strings.Contains(text, "sk-") || strings.Contains(text, "gh") || strings.Contains(text, "github_pat_") || strings.Contains(text, "xox") || strings.Contains(text, "xapp-"),
		awsAccessKey: strings.Contains(text, "AKIA") || strings.Contains(text, "ASIA"),
		privateKey:   strings.Contains(text, "-----BEGIN "),
	}
	needsAssignment := strings.ContainsAny(text, ":=")
	needsFlag := strings.Contains(text, "--")
	// bearer and basic both begin with ASCII B/b. Unicode simple-fold has no
	// additional members for that rune, so this byte gate is equivalent to the
	// first rune accepted by the case-insensitive regular expression.
	if !needsAssignment && !needsFlag && !strings.ContainsAny(text, "bB") {
		return hints
	}
	folded := foldASCIISimpleV1(text)
	hints.authScheme = strings.Contains(folded, "bearer") || strings.Contains(folded, "basic")
	if needsAssignment || needsFlag {
		credentialName := containsAnyStringV1(folded,
			"authorization", "api", "token", "secret", "password",
			"credential", "cookie", "passwd", "pwd", "aws",
		)
		hints.assignment = needsAssignment && credentialName
		hints.flag = needsFlag && credentialName
	}
	return hints
}

func containsAnyStringV1(text string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(text, candidate) {
			return true
		}
	}
	return false
}

func foldASCIISimpleV1(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	for _, r := range text {
		out.WriteRune(asciiSimpleFoldRuneV1(r))
	}
	return out.String()
}

func asciiSimpleFoldRuneV1(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	if r >= 'a' && r <= 'z' {
		return r
	}
	for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
		if folded >= 'A' && folded <= 'Z' {
			return folded + ('a' - 'A')
		}
		if folded >= 'a' && folded <= 'z' {
			return folded
		}
	}
	return r
}

// ProjectValueV1 applies the same credential projection recursively to the
// JSON-like values used by public job/event envelopes. Unknown scalar types
// are preserved; no map or slice is mutated in place.
func ProjectValueV1(value any) any {
	projected, limited := projectValueV1(value, &valueBudgetV1{}, 0)
	if limited {
		return RedactedV1
	}
	return projected
}

type valueBudgetV1 struct {
	nodes     int
	textBytes int
}

func (budget *valueBudgetV1) consumeText(value string) bool {
	if len(value) > maxTextBytesV1 || budget.textBytes > maxAggregateBytesV1-len(value) {
		return false
	}
	budget.textBytes += len(value)
	return true
}

func projectValueV1(value any, budget *valueBudgetV1, depth int) (any, bool) {
	if depth > maxValueDepthV1 || budget.nodes >= maxValueNodesV1 {
		return RedactedV1, true
	}
	budget.nodes++
	switch typed := value.(type) {
	case string:
		if !budget.consumeText(typed) {
			return RedactedV1, true
		}
		return ProjectTextV1(typed), false
	case map[string]any:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !budget.consumeText(key) {
				return RedactedV1, true
			}
			projectedKey := ProjectTextV1(key)
			if _, collision := out[projectedKey]; collision {
				// Two distinct untrusted keys can collapse to the same public key
				// after credential projection. Keeping either value would silently
				// discard audit data, so reject the whole record deterministically.
				return RedactedV1, true
			}
			if credentialKeyHasMaterialV1(key, typed[key]) {
				out[projectedKey] = RedactedV1
				continue
			}
			projectedChild, limited := projectValueV1(typed[key], budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[projectedKey] = projectedChild
		}
		return out, false
	case map[string]string:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make(map[string]string, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if !budget.consumeText(key) {
				return RedactedV1, true
			}
			projectedKey := ProjectTextV1(key)
			if _, collision := out[projectedKey]; collision {
				return RedactedV1, true
			}
			if credentialKeyHasMaterialV1(key, typed[key]) {
				out[projectedKey] = RedactedV1
				continue
			}
			projectedChild, limited := projectValueV1(typed[key], budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[projectedKey], _ = projectedChild.(string)
		}
		return out, false
	case []any:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make([]any, len(typed))
		for index, item := range typed {
			projectedChild, limited := projectValueV1(item, budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[index] = projectedChild
		}
		return out, false
	case []map[string]any:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make([]map[string]any, len(typed))
		for index, item := range typed {
			projectedChild, limited := projectValueV1(item, budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[index], _ = projectedChild.(map[string]any)
		}
		return out, false
	case []map[string]string:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make([]map[string]string, len(typed))
		for index, item := range typed {
			projectedChild, limited := projectValueV1(item, budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[index], _ = projectedChild.(map[string]string)
		}
		return out, false
	case []string:
		if len(typed) > maxValueNodesV1 {
			return RedactedV1, true
		}
		out := make([]string, len(typed))
		for index, item := range typed {
			projectedChild, limited := projectValueV1(item, budget, depth+1)
			if limited {
				return RedactedV1, true
			}
			out[index], _ = projectedChild.(string)
		}
		return out, false
	case json.RawMessage:
		if !budget.consumeText(string(typed)) {
			return RedactedV1, true
		}
		return json.RawMessage(ProjectTextV1(string(typed))), false
	default:
		return value, false
	}
}

// ValidateValueV1 rejects credentials at a public write or transport boundary.
// Projection belongs before this validator; validation never silently mutates
// authority-bearing records.
func ValidateValueV1(value any) error {
	containsCredential, limited := containsCredentialValueV1(value, &valueBudgetV1{}, 0)
	if limited {
		return ErrProjectionLimitV1
	}
	if containsCredential {
		return ErrCredentialMaterialV1
	}
	return nil
}

// ExtractExplicitSecretsV1 returns literal credentials visible in a command
// so echoed stdout/stderr can be projected without persisting the command.
func ExtractExplicitSecretsV1(text string) []string {
	if len(text) > maxTextBytesV1 {
		return nil
	}
	return extractExplicitSecretsWithHintsV1(text, buildCredentialScanHintsV1(text))
}

func extractExplicitSecretsWithHintsV1(text string, hints credentialScanHintsV1) []string {
	values := []string{}
	seen := map[string]struct{}{}
	appendValue := func(value string) {
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		decoded, decodeErr := url.QueryUnescape(value)
		if len(value) < 4 || value == RedactedV1 || (decodeErr == nil && decoded == RedactedV1) {
			return
		}
		if _, found := seen[value]; found {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if hints.assignment {
		for _, match := range credentialAssignmentV1.FindAllStringSubmatch(text, maxExplicitSecretsV1) {
			if len(match) > 3 {
				rawValue := firstNonEmptyV1(match[3:]...)
				if credentialTextKeyHasMaterialV1(match[2], rawValue) {
					appendValue(rawValue)
				}
			}
		}
	}
	if hints.authScheme {
		for _, match := range authSchemeV1.FindAllStringSubmatch(text, maxExplicitSecretsV1) {
			if len(match) > 2 {
				appendValue(firstNonEmptyV1(match[2:]...))
			}
		}
	}
	if hints.flag {
		for _, match := range credentialFlagV1.FindAllStringSubmatch(text, maxExplicitSecretsV1) {
			if len(match) > 3 {
				rawValue := firstNonEmptyV1(match[3:]...)
				if credentialTextKeyHasMaterialV1(match[2], rawValue) {
					appendValue(rawValue)
				}
			}
		}
	}
	if hints.knownToken {
		for _, match := range knownTokenV1.FindAllString(text, maxExplicitSecretsV1) {
			appendValue(match)
		}
	}
	if hints.awsAccessKey {
		for _, match := range awsAccessKeyV1.FindAllString(text, maxExplicitSecretsV1) {
			appendValue(match)
		}
	}
	if hints.url {
		for _, candidate := range urlCandidateV1.FindAllString(text, maxExplicitSecretsV1) {
			parsed, err := url.Parse(candidate)
			if err != nil || parsed == nil {
				continue
			}
			if parsed.User != nil {
				appendValue(parsed.User.Username())
				if password, ok := parsed.User.Password(); ok {
					appendValue(password)
				}
			}
			for key, queryValues := range parsed.Query() {
				if !credentialURLKeyHasMaterialV1(key, queryValues) {
					continue
				}
				for _, value := range queryValues {
					appendValue(value)
				}
			}
		}
	}
	return values
}

func ContainsCredentialV1(text string) bool {
	if len(text) > maxTextBytesV1 {
		return true
	}
	return ProjectTextV1(text) != text
}

func AuthishKeyV1(key string) bool {
	normalized := normalizedCredentialKeyV1(key)
	if safeCredentialMetadataKeyV1(normalized) {
		return false
	}
	switch normalized {
	case "authorization", "proxyauthorization", "xapikey", "apikey", "accesstoken",
		"refreshtoken", "idtoken", "token", "clientsecret", "password", "passwd", "pwd",
		"secret", "credential", "cookie", "awsaccesskeyid", "awssecretaccesskey",
		"awssecuritytoken", "awssessiontoken", "sessiontoken", "githubtoken", "slacktoken":
		return true
	}
	return strings.HasSuffix(normalized, "token") || strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "password") || strings.HasSuffix(normalized, "credential") ||
		strings.HasSuffix(normalized, "apikey")
}

func safeCredentialMetadataKeyV1(normalized string) bool {
	// Keep this allowlist exact. Prefix checks such as "has" and "is" turn
	// credential-bearing names like hashicorpToken and issuerSecret into false
	// metadata exceptions.
	switch normalized {
	case "hasapikey", "hastoken", "hasaccesstoken", "hasrefreshtoken", "hasidtoken",
		"hasclientsecret", "haspassword", "hassecret", "hascredential", "hascookie":
		return true
	default:
		return false
	}
}

func normalizedCredentialKeyV1(key string) string {
	lower := strings.ToLower(strings.TrimSpace(key))
	return credentialKeyReplacerV1.Replace(lower)
}

func credentialKeyHasMaterialV1(key string, value any) bool {
	if !credentialValuePresentV1(value) {
		return false
	}
	if safeCredentialMetadataKeyV1(normalizedCredentialKeyV1(key)) {
		_, safeBoolean := value.(bool)
		return !safeBoolean
	}
	return AuthishKeyV1(key)
}

func credentialTextKeyHasMaterialV1(key string, rawValue string) bool {
	normalizedValue := normalizedCredentialLiteralV1(rawValue)
	if normalizedValue == "" || normalizedValue == RedactedV1 {
		return false
	}
	if safeCredentialMetadataKeyV1(normalizedCredentialKeyV1(key)) {
		switch strings.ToLower(normalizedValue) {
		case "true", "false", "null":
			return false
		default:
			return true
		}
	}
	return AuthishKeyV1(key)
}

func credentialURLKeyHasMaterialV1(key string, values []string) bool {
	if AuthishKeyV1(key) {
		return true
	}
	if !safeCredentialMetadataKeyV1(normalizedCredentialKeyV1(key)) {
		return false
	}
	for _, value := range values {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "true", "false", "null", RedactedV1:
			continue
		default:
			return true
		}
	}
	return false
}

func firstNonEmptyV1(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizedCredentialLiteralV1(value string) string {
	normalized := strings.TrimSpace(value)
	for range 2 {
		if len(normalized) >= 2 && ((normalized[0] == '"' && normalized[len(normalized)-1] == '"') ||
			(normalized[0] == '\'' && normalized[len(normalized)-1] == '\'')) {
			normalized = strings.TrimSpace(normalized[1 : len(normalized)-1])
		}
		lower := strings.ToLower(normalized)
		if strings.HasPrefix(lower, "bearer ") {
			normalized = strings.TrimSpace(normalized[len("bearer "):])
		} else if strings.HasPrefix(lower, "basic ") {
			normalized = strings.TrimSpace(normalized[len("basic "):])
		}
	}
	return normalized
}

func containsCredentialValueV1(value any, budget *valueBudgetV1, depth int) (bool, bool) {
	if depth > maxValueDepthV1 || budget.nodes >= maxValueNodesV1 {
		return false, true
	}
	budget.nodes++
	switch typed := value.(type) {
	case string:
		if !budget.consumeText(typed) {
			return false, true
		}
		return ProjectTextV1(typed) != typed, false
	case map[string]any:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for key, child := range typed {
			if !budget.consumeText(key) {
				return false, true
			}
			if ProjectTextV1(key) != key || credentialKeyHasMaterialV1(key, child) {
				return true, false
			}
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case map[string]string:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for key, child := range typed {
			if !budget.consumeText(key) {
				return false, true
			}
			if ProjectTextV1(key) != key || credentialKeyHasMaterialV1(key, child) {
				return true, false
			}
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case []any:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for _, child := range typed {
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case []map[string]any:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for _, child := range typed {
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case []map[string]string:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for _, child := range typed {
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case []string:
		if len(typed) > maxValueNodesV1 {
			return false, true
		}
		for _, child := range typed {
			containsCredential, limited := containsCredentialValueV1(child, budget, depth+1)
			if containsCredential || limited {
				return containsCredential, limited
			}
		}
	case json.RawMessage:
		if !budget.consumeText(string(typed)) {
			return false, true
		}
		return ProjectTextV1(string(typed)) != string(typed), false
	}
	return false, false
}

func credentialValuePresentV1(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed != "" && trimmed != RedactedV1
	case json.RawMessage:
		trimmed := strings.TrimSpace(string(typed))
		return trimmed != "" && trimmed != `"`+RedactedV1+`"` && trimmed != "null"
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case map[string]string:
		return len(typed) > 0
	default:
		return true
	}
}

func redactURLV1(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return RedactedV1
	}
	hadTrailingEmptyFragment := strings.HasSuffix(raw, "#") && parsed.Fragment == ""
	if parsed.User != nil {
		parsed.User = url.UserPassword(RedactedV1, RedactedV1)
	}
	query := parsed.Query()
	for key, values := range query {
		if credentialURLKeyHasMaterialV1(key, values) {
			query.Set(key, RedactedV1)
		}
	}
	parsed.RawQuery = query.Encode()
	if credentialAssignmentV1.MatchString(parsed.Fragment) || authSchemeV1.MatchString(parsed.Fragment) {
		parsed.Fragment = RedactedV1
	}
	projected := parsed.String()
	if hadTrailingEmptyFragment && !strings.HasSuffix(projected, "#") {
		projected += "#"
	}
	return projected
}
