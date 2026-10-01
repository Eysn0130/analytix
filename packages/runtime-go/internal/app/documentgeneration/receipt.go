package documentgeneration

// CreatedReceipt is an in-process host result, produced only after the binary
// creation checkpoint settles. It intentionally contains no path or content.
// Remote maps claiming the same fields cannot become this Go carrier type.
type CreatedReceipt struct {
	ArtifactID  string
	Kind        string
	ContentHash string
	ByteSize    int64
	SavedAt     string
}
