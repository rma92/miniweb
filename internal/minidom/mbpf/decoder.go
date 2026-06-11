package mbpf

import (
	"bytes"
	"fmt"

	"github.com/user/miniweb/internal/minidom"
	"github.com/user/miniweb/internal/proto"
)

// Decode parses an MBPF payload and returns a PageSnapshot.
func Decode(data []byte) (*minidom.PageSnapshot, error) {
	if len(data) < 4 || string(data[:4]) != proto.Magic {
		return nil, fmt.Errorf("invalid MBPF magic bytes")
	}

	r := bytes.NewReader(data[4:])

	version, err := ReadVarint(r)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	if version != proto.Version {
		return nil, fmt.Errorf("unsupported MBPF version %d", version)
	}

	_, err = ReadVarint(r) // flags
	if err != nil {
		return nil, fmt.Errorf("read flags: %w", err)
	}
	pageID, _ := ReadVarint(r)
	snapshotID, _ := ReadVarint(r)
	_, _ = ReadVarint(r) // profile_id
	sectionCount, err := ReadVarint(r)
	if err != nil {
		return nil, fmt.Errorf("read section_count: %w", err)
	}

	snap := &minidom.PageSnapshot{
		Format:     "mbpf",
		Version:    proto.Version,
		SnapshotID: int(snapshotID),
	}
	_ = pageID

	// Collect raw section bytes keyed by type ID.
	rawSections := make(map[uint64][]byte)
	for i := uint64(0); i < sectionCount; i++ {
		typeID, err := ReadVarint(r)
		if err != nil {
			return nil, fmt.Errorf("read section type: %w", err)
		}
		length, err := ReadVarint(r)
		if err != nil {
			return nil, fmt.Errorf("read section length: %w", err)
		}
		secData := make([]byte, length)
		if _, err := r.Read(secData); err != nil {
			return nil, fmt.Errorf("read section data: %w", err)
		}
		rawSections[typeID] = secData
	}

	// Decode string table first — everything else references it.
	var strings []string
	if sd, ok := rawSections[proto.SectionStringTable]; ok {
		strings, err = decodeStringTable(sd)
		if err != nil {
			return nil, fmt.Errorf("string table: %w", err)
		}
	}

	str := func(idx uint64) string {
		if int(idx) < len(strings) {
			return strings[idx]
		}
		return ""
	}

	// Decode layout table.
	var layouts []minidom.LayoutBox
	if ld, ok := rawSections[proto.SectionLayoutTable]; ok {
		layouts, err = decodeLayoutTable(ld)
		if err != nil {
			return nil, fmt.Errorf("layout table: %w", err)
		}
	}

	// Decode interaction table.
	var interactions []minidom.InteractionMeta
	if id, ok := rawSections[proto.SectionInteraction]; ok {
		interactions, err = decodeInteractionTable(id, str)
		if err != nil {
			return nil, fmt.Errorf("interaction table: %w", err)
		}
	}

	// Decode page metadata (URL, title, favicon_url).
	if md, ok := rawSections[proto.SectionPageMeta]; ok {
		mr := bytes.NewReader(md)
		if urlIdx, e2 := ReadVarint(mr); e2 == nil {
			snap.URL = str(urlIdx)
		}
		if titleIdx, e2 := ReadVarint(mr); e2 == nil {
			snap.Title = str(titleIdx)
		}
		if faviconIdx, e2 := ReadVarint(mr); e2 == nil {
			snap.FaviconURL = str(faviconIdx)
		}
	}

	// Decode resource table.
	var resources []minidom.ResourceRef
	if rd, ok := rawSections[proto.SectionResourceTable]; ok {
		resources, err = decodeResourceTable(rd, str)
		if err != nil {
			return nil, fmt.Errorf("resource table: %w", err)
		}
	}
	snap.Resources = resources

	// Decode node tree.
	if nd, ok := rawSections[proto.SectionNodeTree]; ok {
		snap.Nodes, err = decodeNodeTree(nd, str, layouts, interactions)
		if err != nil {
			return nil, fmt.Errorf("node tree: %w", err)
		}
	}

	return snap, nil
}

func decodeStringTable(data []byte) ([]string, error) {
	r := bytes.NewReader(data)
	count, err := ReadVarint(r)
	if err != nil {
		return nil, err
	}
	strs := make([]string, count)
	for i := uint64(0); i < count; i++ {
		length, err := ReadVarint(r)
		if err != nil {
			return nil, err
		}
		b := make([]byte, length)
		if _, err := r.Read(b); err != nil {
			return nil, err
		}
		strs[i] = string(b)
	}
	return strs, nil
}

func decodeLayoutTable(data []byte) ([]minidom.LayoutBox, error) {
	r := bytes.NewReader(data)
	count, err := ReadVarint(r)
	if err != nil {
		return nil, err
	}
	layouts := make([]minidom.LayoutBox, count)
	for i := uint64(0); i < count; i++ {
		x, _ := ReadVarint(r)
		y, _ := ReadVarint(r)
		w, _ := ReadVarint(r)
		h, err := ReadVarint(r)
		if err != nil {
			return nil, err
		}
		layouts[i] = minidom.LayoutBox{
			X: float64(x) / 10,
			Y: float64(y) / 10,
			W: float64(w) / 10,
			H: float64(h) / 10,
		}
	}
	return layouts, nil
}

func decodeInteractionTable(data []byte, str func(uint64) string) ([]minidom.InteractionMeta, error) {
	r := bytes.NewReader(data)
	count, err := ReadVarint(r)
	if err != nil {
		return nil, err
	}
	items := make([]minidom.InteractionMeta, count)
	for i := uint64(0); i < count; i++ {
		elementID, _ := ReadVarint(r)
		kindID, _ := ReadVarint(r)
		flags, _ := ReadVarint(r)
		hrefIdx, _ := ReadVarint(r)
		valueIdx, _ := ReadVarint(r)
		placeholderIdx, _ := ReadVarint(r)
		formID, _ := ReadVarint(r)
		inputTypeIdx, _ := ReadVarint(r)
		nameIdx, err := ReadVarint(r)
		if err != nil {
			return nil, err
		}
		items[i] = minidom.InteractionMeta{
			ElementID:   int(elementID),
			Kind:        kindIDToString(kindID),
			Enabled:     flags&1 != 0,
			Readonly:    flags&2 != 0,
			Href:        str(hrefIdx),
			Value:       str(valueIdx),
			Placeholder: str(placeholderIdx),
			FormID:      int(formID),
			InputType:   str(inputTypeIdx),
			Name:        str(nameIdx),
		}
	}
	return items, nil
}

func decodeResourceTable(data []byte, str func(uint64) string) ([]minidom.ResourceRef, error) {
	r := bytes.NewReader(data)
	count, err := ReadVarint(r)
	if err != nil {
		return nil, err
	}
	items := make([]minidom.ResourceRef, count)
	for i := uint64(0); i < count; i++ {
		idIdx, _ := ReadVarint(r)
		urlIdx, _ := ReadVarint(r)
		mimeIdx, _ := ReadVarint(r)
		width, _ := ReadVarint(r)
		height, _ := ReadVarint(r)
		hasInline, err := ReadVarint(r)
		if err != nil {
			return nil, err
		}
		var inline []byte
		if hasInline == 1 {
			inlineLen, _ := ReadVarint(r)
			inline = make([]byte, inlineLen)
			r.Read(inline)
		}
		items[i] = minidom.ResourceRef{
			ResourceID: str(idIdx),
			URL:        str(urlIdx),
			MIMEType:   str(mimeIdx),
			Width:      int(width),
			Height:     int(height),
			InlineData: inline,
		}
	}
	return items, nil
}

func decodeNodeTree(data []byte, str func(uint64) string, layouts []minidom.LayoutBox, interactions []minidom.InteractionMeta) ([]minidom.Node, error) {
	r := bytes.NewReader(data)
	count, err := ReadVarint(r)
	if err != nil {
		return nil, err
	}
	nodes := make([]minidom.Node, count)
	idxByID := make(map[int]int, count)

	for i := uint64(0); i < count; i++ {
		nodeID, _ := ReadVarint(r)
		nodeTypeID, _ := ReadVarint(r)
		flags, _ := ReadVarint(r)
		parentID, _ := ReadVarint(r)
		textIdx, err := ReadVarint(r)
		if err != nil {
			return nil, err
		}

		n := minidom.Node{
			ID:       int(nodeID),
			Type:     nodeIDToType(int(nodeTypeID)),
			ParentID: int(parentID),
			Text:     str(textIdx),
		}

		if flags&1 != 0 { // has layout
			layoutIdx, _ := ReadVarint(r)
			if int(layoutIdx) < len(layouts) {
				lb := layouts[layoutIdx]
				n.Layout = &lb
			}
		}
		if flags&4 != 0 { // has resource_id (written before interaction)
			resIdx, _ := ReadVarint(r)
			n.ResourceID = str(resIdx)
		}
		if flags&2 != 0 { // has interaction
			interIdx, _ := ReadVarint(r)
			if int(interIdx) < len(interactions) {
				im := interactions[interIdx]
				n.Interaction = &im
			}
		}

		nodes[i] = n
		idxByID[n.ID] = int(i)
	}

	// Rebuild children lists.
	for _, n := range nodes {
		if n.ParentID != 0 {
			if pi, ok := idxByID[n.ParentID]; ok {
				nodes[pi].Children = append(nodes[pi].Children, n.ID)
			}
		}
	}

	return nodes, nil
}

func kindIDToString(id uint64) string {
	switch id {
	case proto.KindLink:
		return "link"
	case proto.KindButton:
		return "button"
	case proto.KindInput:
		return "input"
	case proto.KindTextarea:
		return "textarea"
	case proto.KindSelect:
		return "select"
	case proto.KindForm:
		return "form"
	default:
		return "custom"
	}
}

func nodeIDToType(id int) minidom.NodeType {
	switch id {
	case proto.NodeTypeDocument:
		return minidom.NodeDocument
	case proto.NodeTypeSection:
		return minidom.NodeSection
	case proto.NodeTypeBlock:
		return minidom.NodeBlock
	case proto.NodeTypeInline:
		return minidom.NodeInline
	case proto.NodeTypeText:
		return minidom.NodeText
	case proto.NodeTypeLink:
		return minidom.NodeLink
	case proto.NodeTypeImage:
		return minidom.NodeImage
	case proto.NodeTypeButton:
		return minidom.NodeButton
	case proto.NodeTypeInput:
		return minidom.NodeInput
	case proto.NodeTypeTextarea:
		return minidom.NodeTextarea
	case proto.NodeTypeSelect:
		return minidom.NodeSelect
	case proto.NodeTypeOption:
		return minidom.NodeOption
	case proto.NodeTypeForm:
		return minidom.NodeForm
	case proto.NodeTypeTable:
		return minidom.NodeTable
	case proto.NodeTypeTableRow:
		return minidom.NodeTableRow
	case proto.NodeTypeTableCell:
		return minidom.NodeTableCell
	case proto.NodeTypeList:
		return minidom.NodeList
	case proto.NodeTypeListItem:
		return minidom.NodeListItem
	case proto.NodeTypeHeading:
		return minidom.NodeHeading
	case proto.NodeTypeCanvasFallback:
		return minidom.NodeCanvasFallback
	default:
		return minidom.NodeUnknown
	}
}
