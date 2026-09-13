package mediaexecution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	mediaport "analytix.local/runtime-go/internal/ports/mediaexecution"
)

const (
	MaxPromptBytes       = 16 << 10
	MaxAudioBytes        = 16 << 20
	MaxReferenceImages   = 4
	MaxReferenceBytes    = 10 << 20
	MaxProviderBodyBytes = 24 << 20
	MaxImageBytes        = 16 << 20
	MaxTranscriptBytes   = 64 << 10
	DefaultTimeout       = 180 * time.Second
	MaximumTimeout       = 5 * time.Minute
)

var (
	ErrInvalidRequest     = errors.New("media execution request is invalid")
	ErrUnavailable        = errors.New("media provider is unavailable")
	ErrProviderFailed     = errors.New("media provider request failed")
	ErrAuthorityChanged   = errors.New("media provider authority changed")
	ErrPrivacyUnavailable = errors.New("media privacy projection is unavailable")
)

type Operation string

const (
	OperationImageGenerate    Operation = "image.generate"
	OperationImageEdit        Operation = "image.edit"
	OperationSpeechTranscribe Operation = "speech.transcribe"
)

type ReferenceImage struct {
	Name       string `json:"name"`
	MIMEType   string `json:"mimeType"`
	DataBase64 string `json:"dataBase64"`
}

type Request struct {
	Operation   Operation        `json:"operation"`
	Prompt      string           `json:"prompt,omitempty"`
	Size        string           `json:"size,omitempty"`
	AudioBase64 string           `json:"audioBase64,omitempty"`
	MIMEType    string           `json:"mimeType,omitempty"`
	Language    string           `json:"language,omitempty"`
	Images      []ReferenceImage `json:"images,omitempty"`
	TimeoutMS   int              `json:"timeoutMs,omitempty"`
}

type Result struct {
	Image      []byte
	MIMEType   string
	Transcript string
}

type RegistryAuthority interface {
	ResolveSelectedMediaForExecution(context.Context) (providerregistryapp.ExecutionResolution, error)
	ValidateMediaExecutionCurrent(context.Context, providerregistryapp.ExecutionAuthority) error
}

type Executor struct {
	Registry         RegistryAuthority
	TransportFactory mediaport.Factory
}

func New(registry RegistryAuthority, factory mediaport.Factory) *Executor {
	return &Executor{Registry: registry, TransportFactory: factory}
}

func (executor *Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if executor == nil || executor.Registry == nil || ctx == nil || ctx.Err() != nil || validateRequest(request) != nil {
		return Result{}, ErrInvalidRequest
	}
	// Registry execution authority does not admit uninspected source bytes.
	// The trusted local image/audio projectors are not implemented. Close only
	// these effects, before credential resolution or transport construction.
	if request.Operation != OperationImageGenerate {
		return Result{}, ErrPrivacyUnavailable
	}
	prompt, err := projectMediaText(request.Prompt)
	if err != nil || !mediaMetadataUnchanged(request.Size) || !validImageSize(request.Size) {
		return Result{}, ErrPrivacyUnavailable
	}
	request.Prompt = prompt
	resolution, err := executor.Registry.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer resolution.Clear()
	if strings.TrimSpace(resolution.Provider.SelectedMedia) == "" {
		return Result{}, ErrUnavailable
	}
	if !mediaMetadataUnchanged(resolution.Provider.SelectedMedia) {
		return Result{}, ErrPrivacyUnavailable
	}
	timeout := requestTimeout(request.TimeoutMS)
	factory := executor.TransportFactory
	if factory == nil {
		return Result{}, ErrUnavailable
	}
	client, err := factory(resolution.Provider.Proxy, timeout)
	if err != nil || client == nil {
		return Result{}, ErrUnavailable
	}
	defer client.Close()

	switch request.Operation {
	case OperationImageGenerate, OperationImageEdit:
		return executor.executeImage(ctx, client, resolution, request)
	case OperationSpeechTranscribe:
		return executor.executeSpeech(ctx, client, resolution, request)
	default:
		return Result{}, ErrInvalidRequest
	}
}

var mediaLocalPath = regexp.MustCompile(`(?i)(?:file://|/(?:Users|home|private|Volumes|tmp|var)/|(?:^|[^a-z0-9_/])(?:~[\\/]|/(?:[^/\s"'<>]+/|[^/\s"'<>]+\.[a-z0-9])|[a-z]:[\\/]|\\\\))`)
var mediaDataURI = regexp.MustCompile(`(?i)data:(?:image|audio|video|application)/`)
var mediaImageSize = regexp.MustCompile(`^[1-9][0-9]{1,4}x[1-9][0-9]{1,4}$`)
var mediaURLEscape = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

func validImageSize(size string) bool {
	return size == "" || size == "auto" || mediaImageSize.MatchString(size)
}

func projectMediaText(text string) (string, error) {
	// Media has no retained-source path authority. An explicit local path is
	// unclassified here, not a filename that may be silently sent to a model.
	if mediaLocalPath.MatchString(text) || mediaDataURI.MatchString(text) {
		return "", ErrPrivacyUnavailable
	}
	projected, err := privacyprojectionapp.ProjectProviderRequest(domainsecurity.TurnSecurityContext{}, domainmodel.Request{
		Messages: []domainmodel.Message{{Role: "user", Content: text}},
	})
	if err != nil || len(projected.Messages) != 1 {
		return "", ErrPrivacyUnavailable
	}
	return projected.Messages[0].Content, nil
}

func mediaMetadataUnchanged(value string) bool {
	projected, err := projectMediaText(value)
	return err == nil && projected == value
}

func mediaURLContentSafe(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	// Inspect route components without changing the destination or signature
	// of either a selected endpoint or a Provider-returned download URL.
	for _, prefix := range []string{"/Users/", "/home/", "/private/", "/Volumes/", "/tmp/", "/var/"} {
		if strings.Contains(strings.ToLower(u.Path), strings.ToLower(prefix)) {
			return false
		}
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return false
	}
	// Split before decoding so escaped slashes cannot erase a nested local
	// path's structure. Each component is then decoded and checked below.
	parts := strings.Split(u.EscapedPath(), "/")
	parts = append(parts, strings.Split(u.Hostname(), ".")...)
	for key, entries := range values {
		parts = append(parts, key)
		parts = append(parts, entries...)
		for _, entry := range entries {
			// Preserve labels: a short account number is sensitive with its
			// account cue even when the same bare digits are ordinary data.
			parts = append(parts, key+": "+entry)
		}
	}
	for _, part := range parts {
		for pass := 0; ; pass++ {
			if pass == 8 || !mediaMetadataUnchanged(part) {
				return false
			}
			// Initial URL/query parsing already validated escapes. A decoded
			// literal percent is legal; decode only remaining complete escapes.
			decoded := mediaURLEscape.ReplaceAllStringFunc(part, func(escape string) string {
				value, _ := url.PathUnescape(escape)
				return value
			})
			if decoded == part {
				break
			}
			part = decoded
		}
	}
	return true
}

func validateRequest(request Request) error {
	if request.TimeoutMS < 0 || request.TimeoutMS > int(MaximumTimeout/time.Millisecond) ||
		len(request.Prompt) > MaxPromptBytes || len(request.Images) > MaxReferenceImages ||
		len(request.AudioBase64) > base64.StdEncoding.EncodedLen(MaxAudioBytes)+4 ||
		len(request.Language) > 32 || len(request.Size) > 32 || len(request.MIMEType) > 128 {
		return ErrInvalidRequest
	}
	switch request.Operation {
	case OperationImageGenerate:
		if strings.TrimSpace(request.Prompt) == "" || request.AudioBase64 != "" || len(request.Images) != 0 {
			return ErrInvalidRequest
		}
	case OperationImageEdit:
		if strings.TrimSpace(request.Prompt) == "" || request.AudioBase64 != "" || len(request.Images) == 0 {
			return ErrInvalidRequest
		}
		for _, image := range request.Images {
			if len(image.Name) > 255 || !allowedImageMIME(image.MIMEType) ||
				len(image.DataBase64) > base64.StdEncoding.EncodedLen(MaxReferenceBytes)+4 {
				return ErrInvalidRequest
			}
			decoded, err := base64.StdEncoding.DecodeString(image.DataBase64)
			if err != nil || len(decoded) == 0 || len(decoded) > MaxReferenceBytes {
				clear(decoded)
				return ErrInvalidRequest
			}
			clear(decoded)
		}
	case OperationSpeechTranscribe:
		if request.Prompt != "" || len(request.Images) != 0 || request.AudioBase64 == "" || !allowedAudioMIME(request.MIMEType) {
			return ErrInvalidRequest
		}
		decoded, err := base64.StdEncoding.DecodeString(request.AudioBase64)
		if err != nil || len(decoded) == 0 || len(decoded) > MaxAudioBytes {
			clear(decoded)
			return ErrInvalidRequest
		}
		clear(decoded)
	default:
		return ErrInvalidRequest
	}
	return nil
}

func requestTimeout(milliseconds int) time.Duration {
	if milliseconds <= 0 {
		return DefaultTimeout
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func (executor *Executor) executeImage(
	ctx context.Context,
	client mediaport.Transport,
	resolution providerregistryapp.ExecutionResolution,
	request Request,
) (Result, error) {
	protocol := imageProtocol(resolution.Provider)
	endpointSuffix := "images/generations"
	if request.Operation == OperationImageEdit {
		endpointSuffix = "images/edits"
	}
	if protocol == "minimax" {
		endpointSuffix = "image_generation"
	}
	endpoint, err := mediaEndpoint(resolution.Provider.Endpoint, endpointSuffix)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	var body io.Reader
	contentType := "application/json"
	if request.Operation == OperationImageEdit && protocol != "minimax" {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		_ = writer.WriteField("model", resolution.Provider.SelectedMedia)
		_ = writer.WriteField("prompt", request.Prompt)
		if request.Size != "" && request.Size != "auto" {
			_ = writer.WriteField("size", request.Size)
		}
		_ = writer.WriteField("response_format", "b64_json")
		for _, image := range request.Images {
			decoded, decodeErr := base64.StdEncoding.DecodeString(image.DataBase64)
			if decodeErr != nil {
				return Result{}, ErrInvalidRequest
			}
			header := textproto.MIMEHeader{}
			header.Set("Content-Disposition", `form-data; name="image"; filename="`+safeFilename(image.Name)+`"`)
			header.Set("Content-Type", image.MIMEType)
			part, createErr := writer.CreatePart(header)
			if createErr != nil {
				clear(decoded)
				return Result{}, ErrInvalidRequest
			}
			_, _ = part.Write(decoded)
			clear(decoded)
		}
		if err := writer.Close(); err != nil {
			return Result{}, ErrInvalidRequest
		}
		body = &buffer
		contentType = writer.FormDataContentType()
	} else {
		payload := map[string]any{
			"model":  resolution.Provider.SelectedMedia,
			"prompt": request.Prompt,
			"n":      1,
		}
		if request.Size != "" && request.Size != "auto" {
			payload["size"] = request.Size
		}
		if protocol == "minimax" {
			payload["prompt_optimizer"] = true
			payload["response_format"] = "base64"
			if request.Operation == OperationImageEdit {
				references := make([]map[string]string, 0, len(request.Images))
				for _, image := range request.Images {
					references = append(references, map[string]string{"type": "character", "image_file": "data:" + image.MIMEType + ";base64," + image.DataBase64})
				}
				payload["subject_reference"] = references
			}
		} else {
			payload["response_format"] = "b64_json"
		}
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil || len(encoded) > MaxProviderBodyBytes {
			clear(encoded)
			return Result{}, ErrInvalidRequest
		}
		defer clear(encoded)
		body = bytes.NewReader(encoded)
	}
	response, err := executor.send(ctx, client, resolution, "POST", endpoint, body, contentType, true)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	data, err := readBounded(response.Body, MaxProviderBodyBytes)
	if err != nil {
		return Result{}, ErrProviderFailed
	}
	defer clear(data)
	image, mimeType, imageURL, err := decodeImageResponse(data, protocol)
	if err != nil {
		return Result{}, ErrProviderFailed
	}
	if imageURL != "" {
		return executor.downloadImage(ctx, client, resolution, imageURL)
	}
	if len(image) == 0 || len(image) > MaxImageBytes || !allowedImageMIME(mimeType) {
		clear(image)
		return Result{}, ErrProviderFailed
	}
	if err := executor.Registry.ValidateMediaExecutionCurrent(ctx, resolution.Authority()); err != nil {
		clear(image)
		return Result{}, ErrAuthorityChanged
	}
	return Result{Image: image, MIMEType: mimeType}, nil
}

func (executor *Executor) downloadImage(
	ctx context.Context,
	client mediaport.Transport,
	original providerregistryapp.ExecutionResolution,
	rawURL string,
) (Result, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return Result{}, ErrProviderFailed
	}
	current, err := executor.Registry.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		return Result{}, ErrAuthorityChanged
	}
	defer current.Clear()
	if current.Authority() != original.Authority() {
		return Result{}, ErrAuthorityChanged
	}
	response, err := executor.send(ctx, client, current, "GET", parsed.String(), nil, "", false)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	image, err := readBounded(response.Body, MaxImageBytes)
	if err != nil || len(image) == 0 {
		clear(image)
		return Result{}, ErrProviderFailed
	}
	mimeType := strings.TrimSpace(strings.Split(response.ContentType, ";")[0])
	if !allowedImageMIME(mimeType) {
		clear(image)
		return Result{}, ErrProviderFailed
	}
	if err := executor.Registry.ValidateMediaExecutionCurrent(ctx, current.Authority()); err != nil {
		clear(image)
		return Result{}, ErrAuthorityChanged
	}
	return Result{Image: image, MIMEType: mimeType}, nil
}

func (executor *Executor) executeSpeech(
	ctx context.Context,
	client mediaport.Transport,
	resolution providerregistryapp.ExecutionResolution,
	request Request,
) (Result, error) {
	audio, err := base64.StdEncoding.DecodeString(request.AudioBase64)
	if err != nil {
		return Result{}, ErrInvalidRequest
	}
	defer clear(audio)
	protocol := speechProtocol(resolution.Provider)
	pathSuffix := "audio/transcriptions"
	var body io.Reader
	contentType := "application/json"
	if protocol == "mimo-asr" {
		pathSuffix = "chat/completions"
		payload := map[string]any{
			"model": resolution.Provider.SelectedMedia,
			"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{
				"type": "input_audio", "input_audio": map[string]string{"data": "data:" + request.MIMEType + ";base64," + request.AudioBase64},
			}}}},
			"asr_options": map[string]string{"language": firstNonEmpty(request.Language, "auto")},
			"stream":      false,
		}
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil || len(encoded) > MaxProviderBodyBytes {
			clear(encoded)
			return Result{}, ErrInvalidRequest
		}
		defer clear(encoded)
		body = bytes.NewReader(encoded)
	} else {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="file"; filename="recording.`+audioExtension(request.MIMEType)+`"`)
		header.Set("Content-Type", request.MIMEType)
		part, createErr := writer.CreatePart(header)
		if createErr != nil {
			return Result{}, ErrInvalidRequest
		}
		_, _ = part.Write(audio)
		_ = writer.WriteField("model", resolution.Provider.SelectedMedia)
		_ = writer.WriteField("response_format", "json")
		if request.Language != "" && request.Language != "auto" {
			_ = writer.WriteField("language", request.Language)
		}
		if err := writer.Close(); err != nil {
			return Result{}, ErrInvalidRequest
		}
		body = &buffer
		contentType = writer.FormDataContentType()
	}
	endpoint, err := mediaEndpoint(resolution.Provider.Endpoint, pathSuffix)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	response, err := executor.send(ctx, client, resolution, "POST", endpoint, body, contentType, true)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	data, err := readBounded(response.Body, MaxTranscriptBytes)
	if err != nil {
		return Result{}, ErrProviderFailed
	}
	defer clear(data)
	transcript, err := decodeTranscript(data, protocol)
	if err != nil || transcript == "" || len(transcript) > MaxTranscriptBytes {
		return Result{}, ErrProviderFailed
	}
	if err := executor.Registry.ValidateMediaExecutionCurrent(ctx, resolution.Authority()); err != nil {
		transcript = ""
		return Result{}, ErrAuthorityChanged
	}
	return Result{Transcript: transcript}, nil
}

func (executor *Executor) send(
	ctx context.Context,
	client mediaport.Transport,
	resolution providerregistryapp.ExecutionResolution,
	method string,
	rawURL string,
	body io.Reader,
	contentType string,
	credentialed bool,
) (*mediaport.Response, error) {
	if !mediaURLContentSafe(rawURL) {
		return nil, ErrPrivacyUnavailable
	}
	if err := executor.Registry.ValidateMediaExecutionCurrent(ctx, resolution.Authority()); err != nil {
		return nil, ErrAuthorityChanged
	}
	var credential []byte
	if credentialed {
		credential = resolution.Credential
	}
	response, err := client.Send(ctx, mediaport.Request{Method: method, URL: rawURL, Body: body, ContentType: contentType, Credential: credential, Credentialed: credentialed})
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if errors.Is(err, mediaport.ErrInvalidRequest) {
			return nil, ErrInvalidRequest
		}
		return nil, ErrProviderFailed
	}
	if response == nil || response.Body == nil {
		return nil, ErrProviderFailed
	}
	if err := executor.Registry.ValidateMediaExecutionCurrent(ctx, resolution.Authority()); err != nil {
		_ = response.Body.Close()
		return nil, ErrAuthorityChanged
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return nil, ErrProviderFailed
	}
	return response, nil
}

func mediaEndpoint(baseURL, suffix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", ErrUnavailable
	}
	clean := strings.TrimSuffix(parsed.Path, "/")
	for _, known := range []string{"/images/generations", "/images/edits", "/image_generation", "/audio/transcriptions", "/chat/completions"} {
		if strings.HasSuffix(strings.ToLower(clean), known) {
			clean = strings.TrimSuffix(clean[:len(clean)-len(known)], "/")
			break
		}
	}
	parsed.Path = path.Join(clean, suffix)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func decodeImageResponse(data []byte, protocol string) ([]byte, string, string, error) {
	if protocol == "minimax" {
		var payload struct {
			Data struct {
				ImageBase64 []string `json:"image_base64"`
				ImageURLs   []string `json:"image_urls"`
			} `json:"data"`
			BaseResponse struct {
				StatusCode int `json:"status_code"`
			} `json:"base_resp"`
		}
		if json.Unmarshal(data, &payload) != nil || payload.BaseResponse.StatusCode != 0 {
			return nil, "", "", ErrProviderFailed
		}
		if len(payload.Data.ImageBase64) > 0 {
			image, err := base64.StdEncoding.DecodeString(payload.Data.ImageBase64[0])
			return image, detectImageMIME(image), "", err
		}
		if len(payload.Data.ImageURLs) > 0 {
			return nil, "", payload.Data.ImageURLs[0], nil
		}
		return nil, "", "", ErrProviderFailed
	}
	var payload struct {
		Data []struct {
			Base64 string `json:"b64_json"`
			URL    string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Data) != 1 {
		return nil, "", "", ErrProviderFailed
	}
	if payload.Data[0].Base64 != "" {
		image, err := base64.StdEncoding.DecodeString(payload.Data[0].Base64)
		return image, detectImageMIME(image), "", err
	}
	return nil, "", payload.Data[0].URL, nil
}

func decodeTranscript(data []byte, protocol string) (string, error) {
	if protocol == "mimo-asr" {
		var payload struct {
			Choices []struct {
				Message struct {
					Content any `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &payload) != nil || len(payload.Choices) == 0 {
			return "", ErrProviderFailed
		}
		switch content := payload.Choices[0].Message.Content.(type) {
		case string:
			return strings.TrimSpace(content), nil
		case []any:
			var result strings.Builder
			for _, part := range content {
				if object, ok := part.(map[string]any); ok {
					if text, ok := object["text"].(string); ok {
						result.WriteString(text)
					}
				}
			}
			return strings.TrimSpace(result.String()), nil
		}
		return "", ErrProviderFailed
	}
	var payload struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return "", ErrProviderFailed
	}
	return strings.TrimSpace(payload.Text), nil
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		clear(data)
		return nil, ErrProviderFailed
	}
	return data, nil
}

func imageProtocol(provider domainregistry.Provider) string {
	identity := strings.ToLower(strings.Join([]string{
		provider.ID, provider.Kind, provider.Endpoint, provider.SelectedMedia,
	}, " "))
	if strings.Contains(identity, "minimax") {
		return "minimax"
	}
	return "openai-images"
}

func speechProtocol(provider domainregistry.Provider) string {
	identity := strings.ToLower(strings.Join([]string{
		provider.ID, provider.Kind, provider.Endpoint, provider.SelectedMedia,
	}, " "))
	if strings.Contains(identity, "mimo") || strings.Contains(identity, "xiaomi") {
		return "mimo-asr"
	}
	return "openai-transcriptions"
}

func allowedImageMIME(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/png", "image/jpeg", "image/webp":
		return true
	default:
		return false
	}
}

func allowedAudioMIME(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp4", "audio/webm", "audio/ogg", "audio/flac":
		return true
	default:
		return false
	}
}

func detectImageMIME(data []byte) string {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x89, 0x50, 0x4e, 0x47}) {
		return "image/png"
	}
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

func safeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return "reference.png"
	}
	var safe strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '-' || char == '_' {
			safe.WriteRune(char)
		} else {
			safe.WriteByte('_')
		}
		if safe.Len() >= 128 {
			break
		}
	}
	if safe.Len() == 0 || safe.String() == "." || safe.String() == ".." {
		return "reference.png"
	}
	return safe.String()
}

func audioExtension(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4":
		return "m4a"
	case "audio/webm":
		return "webm"
	case "audio/ogg":
		return "ogg"
	case "audio/flac":
		return "flac"
	default:
		return "wav"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
