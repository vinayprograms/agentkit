package contentguard

import "fmt"

// Ingest adds content to the guard's tracking.
func (g *Guard) Ingest(trust TrustLevel, typ ContentKind, mutable bool, content, source string) *Taint {
	return g.ingest(trust, typ, mutable, content, source, "", 0, nil)
}

// IngestFrom adds content with an agent context identifier.
func (g *Guard) IngestFrom(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string) *Taint {
	return g.ingest(trust, typ, mutable, content, source, agentContext, 0, nil)
}

// IngestWithLineage adds content with explicit taint lineage.
func (g *Guard) IngestWithLineage(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string, eventSeq uint64, taintedBy []string) *Taint {
	return g.ingest(trust, typ, mutable, content, source, agentContext, eventSeq, taintedBy)
}

func (g *Guard) ingest(trust TrustLevel, typ ContentKind, mutable bool, content, source, agentContext string, eventSeq uint64, taintedBy []string) *Taint {
	g.taintsMu.Lock()
	defer g.taintsMu.Unlock()

	contentHash := computeHash(content)

	// De-duplicate untrusted content
	if trust == Untrusted {
		if existingID, exists := g.contentHashes[contentHash]; exists {
			for _, t := range g.taints {
				if t.ID == existingID {
					if g.logger != nil {
						g.logger.Debug("content deduplicated", map[string]interface{}{
							"hash":        contentHash[:16] + "...",
							"existing_id": existingID,
							"source":      source,
						})
					}
					g.taintCounter++
					id := fmt.Sprintf("b%04d", g.taintCounter)
					taint := newTaint(id, trust, typ, mutable, content, source)
					taint.AgentContext = agentContext
					taint.CreatedAtSeq = eventSeq
					taint.TaintedBy = append(taintedBy, existingID)
					taint.DedupeHit = true
					g.taints = append(g.taints, taint)
					return taint
				}
			}
		}
	}

	g.taintCounter++
	id := fmt.Sprintf("b%04d", g.taintCounter)
	taint := newTaint(id, trust, typ, mutable, content, source)
	taint.AgentContext = agentContext
	taint.CreatedAtSeq = eventSeq
	taint.TaintedBy = taintedBy
	g.taints = append(g.taints, taint)

	if trust == Untrusted {
		g.contentHashes[contentHash] = id
	}

	return taint
}

// UntrustedIDs returns IDs of all untrusted taints in context.
func (g *Guard) UntrustedIDs() []string {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var ids []string
	for _, t := range g.taints {
		if t.Trust == Untrusted {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// FindTaint returns a taint by ID, or nil if not found.
func (g *Guard) FindTaint(id string) *Taint {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()
	return g.findByID(id)
}

func (g *Guard) findByID(id string) *Taint {
	for _, t := range g.taints {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (g *Guard) getUntrustedTaints() []*Taint {
	return g.getUntrustedTaintsForContext("")
}

func (g *Guard) getUntrustedTaintsForContext(agentContext string) []*Taint {
	g.taintsMu.RLock()
	defer g.taintsMu.RUnlock()

	var result []*Taint
	for _, t := range g.taints {
		if t.Trust != Untrusted {
			continue
		}
		if agentContext == "" || t.AgentContext == "" || t.AgentContext == agentContext {
			result = append(result, t)
		}
	}
	return result
}
