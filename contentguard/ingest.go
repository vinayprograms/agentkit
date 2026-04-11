package contentguard

import "fmt"

// Ingest adds content to the guard's tracking.
func (g *Guard) Ingest(trust Trust, kind Kind, mutable bool, text, source string) *Content {
	return g.ingest(trust, kind, mutable, text, source, nil)
}

// IngestWithLineage adds content with explicit parent content IDs.
// Use this when the content was derived from other tracked content
// (e.g., an LLM response influenced by a web fetch result).
func (g *Guard) IngestWithLineage(trust Trust, kind Kind, mutable bool, text, source string, originIDs []string) *Content {
	return g.ingest(trust, kind, mutable, text, source, originIDs)
}

func (g *Guard) ingest(trust Trust, kind Kind, mutable bool, text, source string, originIDs []string) *Content {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Resolve origin IDs to pointers
	var origins []*Content
	for _, id := range originIDs {
		if c := g.contentByID[id]; c != nil {
			origins = append(origins, c)
		}
	}

	hash := computeHash(text)

	// De-duplicate untrusted content — still create a new entry
	// so callers get a unique ID, but link to the existing one.
	if trust == Untrusted {
		if existingID, exists := g.contentHashes[hash]; exists {
			if existing := g.contentByID[existingID]; existing != nil {
				origins = append(origins, existing)
			}
			return g.addContent(trust, kind, mutable, text, source, origins)
		}
	}

	c := g.addContent(trust, kind, mutable, text, source, origins)

	if trust == Untrusted {
		g.contentHashes[hash] = c.ID
	}

	return c
}

func (g *Guard) addContent(trust Trust, kind Kind, mutable bool, text, source string, origins []*Content) *Content {
	g.contentCount++
	id := fmt.Sprintf("b%04d", g.contentCount)
	c := newContent(id, trust, kind, mutable, text, source)
	c.Origins = origins
	g.tracked = append(g.tracked, c)
	g.contentByID[id] = c
	return c
}

// UntrustedIDs returns IDs of all untrusted content in context.
func (g *Guard) UntrustedIDs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ids []string
	for _, c := range g.tracked {
		if c.Trust == Untrusted {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// Find returns tracked content by ID, or nil if not found.
func (g *Guard) Find(id string) *Content {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.contentByID[id]
}

func (g *Guard) getUntrusted() []*Content {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Content
	for _, c := range g.tracked {
		if c.Trust == Untrusted {
			result = append(result, c)
		}
	}
	return result
}
