package proto

// MBPF magic bytes that start every payload.
const Magic = "MBPF"

// Current MBPF version.
const Version = 1

// Section type IDs.
const (
	SectionStringTable    = 1
	SectionTagTable       = 2
	SectionStyleTable     = 3
	SectionNodeTree       = 4
	SectionLayoutTable    = 5
	SectionInteraction    = 6
	SectionResourceTable  = 7
	SectionImageTable     = 8
	SectionFormState      = 9
	SectionScrollState    = 10
	SectionDeltaInstructions = 11
	SectionArchiveMeta    = 12
	SectionDebugInfo      = 13
)

// Node type IDs — must stay stable across versions.
const (
	NodeTypeDocument      = 1
	NodeTypeSection       = 2
	NodeTypeBlock         = 3
	NodeTypeInline        = 4
	NodeTypeText          = 5
	NodeTypeLink          = 6
	NodeTypeImage         = 7
	NodeTypeButton        = 8
	NodeTypeInput         = 9
	NodeTypeTextarea      = 10
	NodeTypeSelect        = 11
	NodeTypeOption        = 12
	NodeTypeForm          = 13
	NodeTypeTable         = 14
	NodeTypeTableRow      = 15
	NodeTypeTableCell     = 16
	NodeTypeList          = 17
	NodeTypeListItem      = 18
	NodeTypeHeading       = 19
	NodeTypeCanvasFallback = 20
	NodeTypeUnknown       = 21
)

// Interaction kind IDs.
const (
	KindLink    = 1
	KindButton  = 2
	KindInput   = 3
	KindTextarea = 4
	KindSelect  = 5
	KindForm    = 6
	KindCustom  = 7
)
