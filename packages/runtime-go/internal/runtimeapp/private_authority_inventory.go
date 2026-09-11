package runtimeapp

type privateAuthorityInventoryV1 struct {
	FinalRecords           bool
	Settlements            bool
	EvidenceRegistry       bool
	Continuations          bool
	PendingWork            bool
	ProviderCacheTelemetry bool
	TurnTerminal           bool
	AttachmentAuthority    bool
	AuthorityAdvance       bool
	EvidenceAuthority      bool
	DatasetSnapshot        bool
	ThreadRiskPolicy       bool
	CheckpointAuthority    bool
	PIIAuthorization       bool
	ReportPublication      bool
	ControlledAccessV1     bool
	ControlledAccessV2     bool
	CaseEntity             bool
	CaseThreadAuthority    bool
	SteeringAuthority      bool
}

func (inventory privateAuthorityInventoryV1) Required() bool {
	return inventory.FinalRecords || inventory.Settlements || inventory.EvidenceRegistry ||
		inventory.Continuations || inventory.PendingWork || inventory.ProviderCacheTelemetry ||
		inventory.TurnTerminal || inventory.AttachmentAuthority || inventory.AuthorityAdvance ||
		inventory.EvidenceAuthority || inventory.DatasetSnapshot || inventory.ThreadRiskPolicy || inventory.CheckpointAuthority || inventory.PIIAuthorization || inventory.ReportPublication ||
		inventory.ControlledAccessV1 || inventory.ControlledAccessV2 || inventory.CaseEntity ||
		inventory.CaseThreadAuthority || inventory.SteeringAuthority
}
