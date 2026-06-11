package minidom

import "encoding/json"

// DeltaOp identifies the operation in a DeltaInstruction.
type DeltaOp int

const (
	DeltaAdd    DeltaOp = 1
	DeltaRemove DeltaOp = 2
	DeltaUpdate DeltaOp = 3
)

// DeltaInstruction describes a single change to the node set.
type DeltaInstruction struct {
	Op       DeltaOp `json:"op"`
	StableID string  `json:"stable_id"`
	Node     *Node   `json:"node,omitempty"` // present for Add and Update
}

// PageDelta holds incremental changes from one snapshot to the next.
type PageDelta struct {
	BaseSnapshotID int                `json:"base_snapshot_id"`
	SnapshotID     int                `json:"snapshot_id"`
	URL            string             `json:"url,omitempty"`
	Title          string             `json:"title,omitempty"`
	FaviconURL     string             `json:"favicon_url,omitempty"`
	Instructions   []DeltaInstruction `json:"instructions"`
}

// nodeFingerprint returns a compact representation of a node for equality checks.
// Layout is excluded because it changes on every scroll/resize.
func nodeFingerprint(n *Node) string {
	type fp struct {
		Type        NodeType         `json:"t"`
		ParentID    int              `json:"p"`
		Text        string           `json:"x,omitempty"`
		Attrs       map[string]string `json:"a,omitempty"`
		Interaction *InteractionMeta `json:"i,omitempty"`
		ResourceID  string           `json:"r,omitempty"`
	}
	b, _ := json.Marshal(fp{
		Type:        n.Type,
		ParentID:    n.ParentID,
		Text:        n.Text,
		Attrs:       n.Attrs,
		Interaction: n.Interaction,
		ResourceID:  n.ResourceID,
	})
	return string(b)
}

// ComputeDelta returns a PageDelta describing how to transform base into current.
// Returns nil if delta would be larger than a full snapshot (e.g. too many changes),
// or if either snapshot has no stable IDs.
func ComputeDelta(base, current *PageSnapshot) *PageDelta {
	if base == nil || len(base.Nodes) == 0 || len(current.Nodes) == 0 {
		return nil
	}

	// Index base nodes by stable ID.
	baseByStable := make(map[string]*Node, len(base.Nodes))
	for i := range base.Nodes {
		if base.Nodes[i].StableID != "" {
			baseByStable[base.Nodes[i].StableID] = &base.Nodes[i]
		}
	}
	// If fewer than 50% of base nodes have stable IDs, delta is unreliable.
	if len(baseByStable) < len(base.Nodes)/2 {
		return nil
	}

	// Index current nodes by stable ID.
	curByStable := make(map[string]*Node, len(current.Nodes))
	for i := range current.Nodes {
		if current.Nodes[i].StableID != "" {
			curByStable[current.Nodes[i].StableID] = &current.Nodes[i]
		}
	}

	var instructions []DeltaInstruction

	// Find removed nodes (in base, not in current).
	for sid, bn := range baseByStable {
		if _, ok := curByStable[sid]; !ok {
			instructions = append(instructions, DeltaInstruction{
				Op:       DeltaRemove,
				StableID: sid,
				Node:     bn,
			})
		}
	}

	// Find added or updated nodes.
	for sid, cn := range curByStable {
		bn, exists := baseByStable[sid]
		if !exists {
			// New node.
			instructions = append(instructions, DeltaInstruction{
				Op:       DeltaAdd,
				StableID: sid,
				Node:     cn,
			})
		} else if nodeFingerprint(cn) != nodeFingerprint(bn) {
			// Changed node.
			instructions = append(instructions, DeltaInstruction{
				Op:       DeltaUpdate,
				StableID: sid,
				Node:     cn,
			})
		}
	}

	// If more than 60% of nodes changed, send full snapshot — delta not worth it.
	total := len(base.Nodes)
	if total > 0 && len(instructions) > total*6/10 {
		return nil
	}

	return &PageDelta{
		BaseSnapshotID: base.SnapshotID,
		SnapshotID:     current.SnapshotID,
		URL:            current.URL,
		Title:          current.Title,
		FaviconURL:     current.FaviconURL,
		Instructions:   instructions,
	}
}

// ApplyDelta applies a PageDelta to a base snapshot, returning the merged snapshot.
func ApplyDelta(base *PageSnapshot, delta *PageDelta) *PageSnapshot {
	if base == nil || delta == nil {
		return nil
	}

	// Build a working copy indexed by stable ID.
	nodes := make(map[string]Node, len(base.Nodes))
	for _, n := range base.Nodes {
		if n.StableID != "" {
			nodes[n.StableID] = n
		}
	}

	for _, inst := range delta.Instructions {
		switch inst.Op {
		case DeltaAdd, DeltaUpdate:
			if inst.Node != nil {
				nodes[inst.StableID] = *inst.Node
			}
		case DeltaRemove:
			delete(nodes, inst.StableID)
		}
	}

	// Rebuild ordered node list preserving original order where possible.
	// First pass: nodes still from base in their original order.
	seen := make(map[string]bool, len(nodes))
	result := make([]Node, 0, len(nodes))
	for _, bn := range base.Nodes {
		if bn.StableID == "" {
			continue
		}
		if n, ok := nodes[bn.StableID]; ok {
			result = append(result, n)
			seen[bn.StableID] = true
		}
	}
	// Second pass: new nodes not in base (added).
	for _, inst := range delta.Instructions {
		if inst.Op == DeltaAdd && !seen[inst.StableID] {
			if n, ok := nodes[inst.StableID]; ok {
				result = append(result, n)
			}
		}
	}

	return &PageSnapshot{
		Format:     base.Format,
		Version:    base.Version,
		SnapshotID: delta.SnapshotID,
		URL:        delta.URL,
		Title:      delta.Title,
		FaviconURL: delta.FaviconURL,
		Nodes:      result,
		Resources:  base.Resources, // resources are stable across renders
	}
}
