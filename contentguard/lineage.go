package contentguard

// TaintLineageNode represents a node in the taint propagation tree.
type TaintLineageNode struct {
	TaintID   string             `json:"taint_id"`
	Trust     TrustLevel         `json:"trust"`
	Source    string             `json:"source"`
	EventSeq  uint64             `json:"event_seq,omitempty"`
	Depth     int                `json:"depth"`
	TaintedBy []*TaintLineageNode `json:"tainted_by,omitempty"`
}

// TaintLineage returns the lineage tree for a taint by ID.
func (g *Guard) TaintLineage(taintID string) *TaintLineageNode {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	t := g.findByID(taintID)
	if t == nil {
		return nil
	}
	return g.buildLineageTree(t, 0, make(map[string]bool))
}

// TaintLineageFor returns lineage trees for multiple taints.
func (g *Guard) TaintLineageFor(taints []*Taint) []*TaintLineageNode {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var nodes []*TaintLineageNode
	for _, t := range taints {
		node := g.buildLineageTree(t, 0, make(map[string]bool))
		if node != nil {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func (g *Guard) buildLineageTree(t *Taint, depth int, visited map[string]bool) *TaintLineageNode {
	if t == nil || visited[t.ID] {
		return nil
	}
	visited[t.ID] = true

	node := &TaintLineageNode{
		TaintID:  t.ID,
		Trust:    t.Trust,
		Source:   t.Source,
		EventSeq: t.CreatedAtSeq,
		Depth:    depth,
	}

	for _, parentID := range t.TaintedBy {
		parent := g.findByID(parentID)
		if parent != nil {
			if child := g.buildLineageTree(parent, depth+1, visited); child != nil {
				node.TaintedBy = append(node.TaintedBy, child)
			}
		}
	}

	return node
}
