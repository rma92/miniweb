package mbpf

import (
	"bytes"
	"math"

	"github.com/user/miniweb/internal/minidom"
	"github.com/user/miniweb/internal/proto"
)

// Encode serializes a PageSnapshot into MBPF binary format.
func Encode(snap *minidom.PageSnapshot) ([]byte, error) {
	e := &encoder{
		strings: make(map[string]uint64),
	}
	return e.encode(snap)
}

type encoder struct {
	strings   map[string]uint64 // string → table index
	stringList []string
}

func (e *encoder) internString(s string) uint64 {
	if idx, ok := e.strings[s]; ok {
		return idx
	}
	idx := uint64(len(e.stringList))
	e.stringList = append(e.stringList, s)
	e.strings[s] = idx
	return idx
}

func (e *encoder) encode(snap *minidom.PageSnapshot) ([]byte, error) {
	// Pre-pass: intern all strings so the string table is complete before we
	// encode nodes.
	e.internString(snap.URL)
	e.internString(snap.Title)
	e.internString(snap.FaviconURL)
	for i := range snap.Nodes {
		e.internNodeStrings(&snap.Nodes[i])
	}
	for _, res := range snap.Resources {
		e.internString(res.ResourceID)
		e.internString(res.URL)
		e.internString(res.MIMEType)
	}

	// Build sections.
	stringSection := e.buildStringSection()
	pageMetaSection := e.buildPageMetaSection(snap)
	nodeSection, layoutSection, interactionSection, resourceSection := e.buildNodeSections(snap)

	sections := [][]byte{stringSection, pageMetaSection, nodeSection}
	if len(layoutSection) > 0 {
		sections = append(sections, layoutSection)
	}
	if len(interactionSection) > 0 {
		sections = append(sections, interactionSection)
	}
	if len(resourceSection) > 0 {
		sections = append(sections, resourceSection)
	}

	// Assemble container.
	var buf bytes.Buffer

	// Magic.
	buf.WriteString(proto.Magic)

	// Header varints.
	buf.Write(AppendVarint(nil, uint64(proto.Version)))         // version
	buf.Write(AppendVarint(nil, 0))                             // flags
	buf.Write(AppendVarint(nil, uint64(snap.SnapshotID)))       // page_id (reuse snapshot_id for now)
	buf.Write(AppendVarint(nil, uint64(snap.SnapshotID)))       // snapshot_id
	buf.Write(AppendVarint(nil, 0))                             // profile_id
	buf.Write(AppendVarint(nil, uint64(len(sections))))         // section_count

	for _, sec := range sections {
		buf.Write(sec)
	}

	return buf.Bytes(), nil
}

func (e *encoder) internNodeStrings(n *minidom.Node) {
	e.internString(n.Text)
	e.internString(n.ResourceID)
	if n.Interaction != nil {
		e.internString(n.Interaction.Href)
		e.internString(n.Interaction.Value)
		e.internString(n.Interaction.Placeholder)
		e.internString(n.Interaction.Kind)
		e.internString(n.Interaction.ActionHint)
		e.internString(n.Interaction.InputType)
		e.internString(n.Interaction.Name)
	}
	for _, v := range n.Attrs {
		e.internString(v)
	}
}

func (e *encoder) buildPageMetaSection(snap *minidom.PageSnapshot) []byte {
	var data bytes.Buffer
	data.Write(AppendVarint(nil, e.internString(snap.URL)))
	data.Write(AppendVarint(nil, e.internString(snap.Title)))
	data.Write(AppendVarint(nil, e.internString(snap.FaviconURL)))
	return wrapSection(proto.SectionPageMeta, data.Bytes())
}

func (e *encoder) buildStringSection() []byte {
	var data bytes.Buffer
	data.Write(AppendVarint(nil, uint64(len(e.stringList))))
	for _, s := range e.stringList {
		b := []byte(s)
		data.Write(AppendVarint(nil, uint64(len(b))))
		data.Write(b)
	}
	return wrapSection(proto.SectionStringTable, data.Bytes())
}

// buildNodeSections builds NODE_TREE, LAYOUT_TABLE, INTERACTION_TABLE, RESOURCE_TABLE.
func (e *encoder) buildNodeSections(snap *minidom.PageSnapshot) (nodeTree, layoutTable, interactionTable, resourceTable []byte) {
	var nodeBuf, layoutBuf, interBuf, resBuf bytes.Buffer

	// Layouts and interactions are stored as separate flat tables indexed by a
	// per-node serial number written into the node record.
	var layoutCount, interCount uint64

	nodeBuf.Write(AppendVarint(nil, uint64(len(snap.Nodes))))

	for _, n := range snap.Nodes {
		nodeTypeID := nodeTypeToID(n.Type)

		// flags: bit0=hasLayout, bit1=hasInteraction, bit2=hasResource
		var flags uint64
		if n.Layout != nil {
			flags |= 1
		}
		if n.Interaction != nil {
			flags |= 2
		}
		if n.ResourceID != "" {
			flags |= 4
		}

		nodeBuf.Write(AppendVarint(nil, uint64(n.ID)))
		nodeBuf.Write(AppendVarint(nil, uint64(nodeTypeID)))
		nodeBuf.Write(AppendVarint(nil, flags))
		nodeBuf.Write(AppendVarint(nil, uint64(n.ParentID)))
		nodeBuf.Write(AppendVarint(nil, e.internString(n.Text)))

		if n.Layout != nil {
			nodeBuf.Write(AppendVarint(nil, layoutCount))
			// Encode coordinates as fixed-point ×10 varints.
			layoutBuf.Write(AppendVarint(nil, floatToFixed(n.Layout.X)))
			layoutBuf.Write(AppendVarint(nil, floatToFixed(n.Layout.Y)))
			layoutBuf.Write(AppendVarint(nil, floatToFixed(n.Layout.W)))
			layoutBuf.Write(AppendVarint(nil, floatToFixed(n.Layout.H)))
			layoutCount++
		}

		if n.ResourceID != "" {
			nodeBuf.Write(AppendVarint(nil, e.internString(n.ResourceID)))
		}

		if n.Interaction != nil {
			nodeBuf.Write(AppendVarint(nil, interCount))
			im := n.Interaction
			interBuf.Write(AppendVarint(nil, uint64(im.ElementID)))
			interBuf.Write(AppendVarint(nil, kindToID(im.Kind)))
			var iflags uint64
			if im.Enabled {
				iflags |= 1
			}
			if im.Readonly {
				iflags |= 2
			}
			interBuf.Write(AppendVarint(nil, iflags))
			interBuf.Write(AppendVarint(nil, e.internString(im.Href)))
			interBuf.Write(AppendVarint(nil, e.internString(im.Value)))
			interBuf.Write(AppendVarint(nil, e.internString(im.Placeholder)))
			interBuf.Write(AppendVarint(nil, uint64(im.FormID)))
			interBuf.Write(AppendVarint(nil, e.internString(im.InputType)))
			interBuf.Write(AppendVarint(nil, e.internString(im.Name)))
			interCount++
		}
	}

	// Resource table.
	resBuf.Write(AppendVarint(nil, uint64(len(snap.Resources))))
	for _, res := range snap.Resources {
		resBuf.Write(AppendVarint(nil, e.internString(res.ResourceID)))
		resBuf.Write(AppendVarint(nil, e.internString(res.URL)))
		resBuf.Write(AppendVarint(nil, e.internString(res.MIMEType)))
		resBuf.Write(AppendVarint(nil, uint64(res.Width)))
		resBuf.Write(AppendVarint(nil, uint64(res.Height)))
		hasInline := uint64(0)
		if len(res.InlineData) > 0 {
			hasInline = 1
		}
		resBuf.Write(AppendVarint(nil, hasInline))
		if hasInline == 1 {
			resBuf.Write(AppendVarint(nil, uint64(len(res.InlineData))))
			resBuf.Write(res.InlineData)
		}
	}

	nodeTree = wrapSection(proto.SectionNodeTree, nodeBuf.Bytes())
	if layoutBuf.Len() > 0 {
		layoutBuf2 := AppendVarint(nil, layoutCount)
		layoutBuf2 = append(layoutBuf2, layoutBuf.Bytes()...)
		layoutTable = wrapSection(proto.SectionLayoutTable, layoutBuf2)
	}
	if interBuf.Len() > 0 {
		interBuf2 := AppendVarint(nil, interCount)
		interBuf2 = append(interBuf2, interBuf.Bytes()...)
		interactionTable = wrapSection(proto.SectionInteraction, interBuf2)
	}
	if resBuf.Len() > 0 {
		resourceTable = wrapSection(proto.SectionResourceTable, resBuf.Bytes())
	}
	return
}

func wrapSection(typeID int, data []byte) []byte {
	var buf []byte
	buf = AppendVarint(buf, uint64(typeID))
	buf = AppendVarint(buf, uint64(len(data)))
	buf = append(buf, data...)
	return buf
}

// floatToFixed converts a float64 CSS pixel value to a fixed-point uint64 (×10).
func floatToFixed(f float64) uint64 {
	if f < 0 {
		return 0
	}
	return uint64(math.Round(f * 10))
}

func nodeTypeToID(t minidom.NodeType) int {
	switch t {
	case minidom.NodeDocument:
		return proto.NodeTypeDocument
	case minidom.NodeSection:
		return proto.NodeTypeSection
	case minidom.NodeBlock:
		return proto.NodeTypeBlock
	case minidom.NodeInline:
		return proto.NodeTypeInline
	case minidom.NodeText:
		return proto.NodeTypeText
	case minidom.NodeLink:
		return proto.NodeTypeLink
	case minidom.NodeImage:
		return proto.NodeTypeImage
	case minidom.NodeButton:
		return proto.NodeTypeButton
	case minidom.NodeInput:
		return proto.NodeTypeInput
	case minidom.NodeTextarea:
		return proto.NodeTypeTextarea
	case minidom.NodeSelect:
		return proto.NodeTypeSelect
	case minidom.NodeOption:
		return proto.NodeTypeOption
	case minidom.NodeForm:
		return proto.NodeTypeForm
	case minidom.NodeTable:
		return proto.NodeTypeTable
	case minidom.NodeTableRow:
		return proto.NodeTypeTableRow
	case minidom.NodeTableCell:
		return proto.NodeTypeTableCell
	case minidom.NodeList:
		return proto.NodeTypeList
	case minidom.NodeListItem:
		return proto.NodeTypeListItem
	case minidom.NodeHeading:
		return proto.NodeTypeHeading
	case minidom.NodeCanvasFallback:
		return proto.NodeTypeCanvasFallback
	default:
		return proto.NodeTypeUnknown
	}
}

func kindToID(kind string) uint64 {
	switch kind {
	case "link":
		return proto.KindLink
	case "button":
		return proto.KindButton
	case "input":
		return proto.KindInput
	case "textarea":
		return proto.KindTextarea
	case "select":
		return proto.KindSelect
	case "form":
		return proto.KindForm
	default:
		return proto.KindCustom
	}
}
