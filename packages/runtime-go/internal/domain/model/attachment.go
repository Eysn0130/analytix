package model

const (
	AttachmentMaxImageBytes                 = 5 * 1024 * 1024
	AttachmentMaxImageDimension             = 4096
	AttachmentMaxDocumentBytes              = 10 * 1024 * 1024
	AttachmentMaxDocumentTextChars          = 200_000
	AttachmentTextFallbackMaxBase64Bytes    = 512 * 1024
	AttachmentTextFallbackMaxImageDimension = 1280
	AttachmentTextFallbackPreferredMimeType = "image/webp"
)

var AttachmentAllowedMimeTypes = []string{
	"image/png",
	"image/jpeg",
	"image/webp",
	"application/pdf",
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
	"application/octet-stream",
}

var AttachmentImageMimeTypes = []string{
	"image/png",
	"image/jpeg",
	"image/webp",
}

var AttachmentDocumentMimeTypes = []string{
	"application/pdf",
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
}
