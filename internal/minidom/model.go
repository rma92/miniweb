package minidom

// NodeType identifies the semantic role of a MiniDOM node.
type NodeType string

const (
	NodeDocument      NodeType = "DOCUMENT"
	NodeSection       NodeType = "SECTION"
	NodeBlock         NodeType = "BLOCK"
	NodeInline        NodeType = "INLINE"
	NodeText          NodeType = "TEXT"
	NodeLink          NodeType = "LINK"
	NodeImage         NodeType = "IMAGE"
	NodeButton        NodeType = "BUTTON"
	NodeInput         NodeType = "INPUT"
	NodeTextarea      NodeType = "TEXTAREA"
	NodeSelect        NodeType = "SELECT"
	NodeOption        NodeType = "OPTION"
	NodeForm          NodeType = "FORM"
	NodeTable         NodeType = "TABLE"
	NodeTableRow      NodeType = "TABLE_ROW"
	NodeTableCell     NodeType = "TABLE_CELL"
	NodeList          NodeType = "LIST"
	NodeListItem      NodeType = "LIST_ITEM"
	NodeHeading       NodeType = "HEADING"
	NodeCanvasFallback NodeType = "CANVAS_FALLBACK"
	NodeUnknown       NodeType = "UNKNOWN"
)

// LayoutBox holds server-computed coordinates in CSS pixels.
type LayoutBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// StyleSubset carries the computed style properties relevant to rendering.
type StyleSubset struct {
	Color      string `json:"color,omitempty"`
	BgColor    string `json:"bg_color,omitempty"`
	FontSize   string `json:"font_size,omitempty"`
	FontWeight string `json:"font_weight,omitempty"`
	Display    string `json:"display,omitempty"`
}

// InteractionMeta describes how a client can interact with a node.
type InteractionMeta struct {
	ElementID   int    `json:"element_id"`
	Kind        string `json:"kind"` // link, button, input, textarea, select, checkbox, radio, form, custom
	Enabled     bool   `json:"enabled"`
	Readonly    bool   `json:"readonly,omitempty"`
	Value       string `json:"value,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Href        string `json:"href,omitempty"`
	FormID      int    `json:"form_id,omitempty"`
	ActionHint  string `json:"action_hint,omitempty"` // click, submit, focus, type, change
	InputType   string `json:"input_type,omitempty"`
	Name        string `json:"name,omitempty"`
}

// ResourceRef points to an image or other resource associated with a node.
type ResourceRef struct {
	ResourceID string `json:"resource_id"`
	URL        string `json:"url,omitempty"`
	MIMEType   string `json:"mime_type,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	InlineData []byte `json:"inline_data,omitempty"`
}

// Node is a single element in the MiniDOM tree.
type Node struct {
	ID          int              `json:"id"`
	StableID    string           `json:"stable_id,omitempty"` // content-hash of structural path; stable across renders
	Type        NodeType         `json:"type"`
	ParentID    int              `json:"parent_id,omitempty"`
	Children    []int            `json:"children,omitempty"`
	Text        string           `json:"text,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
	Layout      *LayoutBox       `json:"layout,omitempty"`
	Style       *StyleSubset     `json:"style,omitempty"`
	Interaction *InteractionMeta `json:"interaction,omitempty"`
	ResourceID  string           `json:"resource_id,omitempty"`
}

// PageSnapshot is the complete rendered representation of a page.
type PageSnapshot struct {
	Format     string        `json:"format"`
	Version    int           `json:"version"`
	SnapshotID int           `json:"snapshot_id"`
	URL        string        `json:"url"`
	Title      string        `json:"title"`
	FaviconURL string        `json:"favicon_url,omitempty"`
	Nodes      []Node        `json:"nodes"`
	Resources  []ResourceRef `json:"resources,omitempty"`
}
