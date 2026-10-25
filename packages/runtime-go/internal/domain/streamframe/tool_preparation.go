package streamframe

// ToolPreparationV1 is a bounded, callback-only provider argument observation.
// CallID is the provider coordinate at the adapter; the Core replaces it with
// its existing host ID before a protected display owner can observe it.
// start records the first actual slot observation with no name or arguments;
// its optional adapter CallID is not a host dispatch identity. Core clears it.
// Named delta carries an incremental fragment; end carries authoritative full
// text. These observations supply neither execution nor viewer authority.
type ToolPreparationV1 struct {
	Index     int    `json:"-"`
	CallID    string `json:"-"`
	ToolName  string `json:"-"`
	Arguments string `json:"-"`
}
