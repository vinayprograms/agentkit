// Package memory provides persistent knowledge storage with BM25 text search.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/standard"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/google/uuid"
)

// BleveStore implements Store using Bleve for BM25 full-text search.
type BleveStore struct {
	mu sync.RWMutex

	// Bleve index for full-text search
	index bleve.Index

	// KV store for scratchpad
	kv     map[string]string
	kvPath string

	// Base path for all storage
	basePath string
}

// BleveStoreConfig configures the Bleve-based memory store.
type BleveStoreConfig struct {
	// BasePath is the directory for all storage files
	BasePath string
}

// ObservationDocument represents a stored observation in Bleve.
type ObservationDocument struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Category  string    `json:"category"` // "finding" | "insight" | "lesson"
	Source    string    `json:"source"`   // "GOAL:step-name" for provenance
	CreatedAt time.Time `json:"created_at"`
}

// NewBleveStore creates a new Bleve-based memory store.
func NewBleveStore(cfg BleveStoreConfig) (*BleveStore, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(cfg.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	indexPath := filepath.Join(cfg.BasePath, "observations.bleve")
	kvPath := filepath.Join(cfg.BasePath, "kv.json")

	// Open or create Bleve index
	var index bleve.Index
	var idxErr error

	if _, statErr := os.Stat(indexPath); os.IsNotExist(statErr) {
		// Create new index
		indexMapping := buildIndexMapping()
		index, idxErr = bleve.New(indexPath, indexMapping)
	} else {
		// Open existing index
		index, idxErr = bleve.Open(indexPath)
	}
	if idxErr != nil {
		return nil, fmt.Errorf("failed to open bleve index: %w", idxErr)
	}

	store := &BleveStore{
		index:    index,
		kv:       make(map[string]string),
		kvPath:   kvPath,
		basePath: cfg.BasePath,
	}

	// Load KV store
	if err := store.loadKV(); err != nil && !os.IsNotExist(err) {
		index.Close()
		return nil, fmt.Errorf("failed to load KV store: %w", err)
	}

	return store, nil
}

// buildIndexMapping creates the Bleve index mapping.
func buildIndexMapping() mapping.IndexMapping {
	// Create a document mapping for observations
	obsMapping := bleve.NewDocumentMapping()

	// Text field mapping (analyzed for full-text search)
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Analyzer = standard.Name

	// Keyword field mapping (not analyzed, exact match)
	keywordFieldMapping := bleve.NewKeywordFieldMapping()

	// Date field mapping
	dateFieldMapping := bleve.NewDateTimeFieldMapping()

	obsMapping.AddFieldMappingsAt("content", textFieldMapping)
	obsMapping.AddFieldMappingsAt("category", keywordFieldMapping)
	obsMapping.AddFieldMappingsAt("source", keywordFieldMapping)
	obsMapping.AddFieldMappingsAt("created_at", dateFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultMapping = obsMapping
	indexMapping.DefaultAnalyzer = standard.Name

	return indexMapping
}

// RememberObservation stores an observation with its category and returns the ID.
func (s *BleveStore) RememberObservation(ctx context.Context, content, category, source string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	now := time.Now()

	doc := ObservationDocument{
		ID:        id,
		Content:   content,
		Category:  category,
		Source:    source,
		CreatedAt: now,
	}

	// Index in Bleve
	if err := s.index.Index(id, doc); err != nil {
		return "", fmt.Errorf("failed to index document: %w", err)
	}

	return id, nil
}

// RememberFIL stores multiple observations and returns their IDs.
func (s *BleveStore) RememberFIL(ctx context.Context, findings, insights, lessons []string, source string) ([]string, error) {
	var ids []string

	for _, f := range findings {
		id, err := s.RememberObservation(ctx, f, "finding", source)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}

	for _, i := range insights {
		id, err := s.RememberObservation(ctx, i, "insight", source)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}

	for _, l := range lessons {
		id, err := s.RememberObservation(ctx, l, "lesson", source)
		if err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// RetrieveByID gets a single observation by ID.
func (s *BleveStore) RetrieveByID(ctx context.Context, id string) (*ObservationItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Search for the exact ID using a doc ID query
	docIDQuery := bleve.NewDocIDQuery([]string{id})
	searchReq := bleve.NewSearchRequest(docIDQuery)
	searchReq.Fields = []string{"content", "category"}
	searchReq.Size = 1

	results, err := s.index.Search(searchReq)
	if err != nil {
		return nil, err
	}

	if results.Total == 0 {
		return nil, nil
	}

	hit := results.Hits[0]
	item := &ObservationItem{
		ID: id,
	}

	if v, ok := hit.Fields["content"].(string); ok {
		item.Content = v
	}
	if v, ok := hit.Fields["category"].(string); ok {
		item.Category = v
	}

	return item, nil
}

// RecallByCategory performs semantic search for a specific category.
func (s *BleveStore) RecallByCategory(ctx context.Context, queryText, category string, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	// Use pure BM25 - semantic graph expansion was found to hurt recall quality
	// by diluting queries with noisy related terms and losing original query terms.
	contentQuery := bleve.NewMatchQuery(queryText)

	// Build category filter
	categoryQuery := bleve.NewTermQuery(category)
	categoryQuery.SetField("category")

	// Combine: content matches AND category matches
	boolQuery := bleve.NewBooleanQuery()
	boolQuery.AddMust(contentQuery)
	boolQuery.AddMust(categoryQuery)

	// Create search request
	searchReq := bleve.NewSearchRequest(boolQuery)
	searchReq.Size = limit
	searchReq.Fields = []string{"content"}

	// Execute search
	searchResult, err := s.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Extract content from results
	var results []string
	for _, hit := range searchResult.Hits {
		if content, ok := hit.Fields["content"].(string); ok {
			results = append(results, content)
		}
	}

	return results, nil
}

// RecallFIL performs semantic search and returns results grouped as Findings, Insights, Lessons.
func (s *BleveStore) RecallFIL(ctx context.Context, queryText string, limitPerCategory int) (*FILResult, error) {
	if limitPerCategory <= 0 {
		limitPerCategory = 5
	}

	// Query each category in parallel
	var wg sync.WaitGroup
	var findings, insights, lessons []string
	var findingsErr, insightsErr, lessonsErr error

	wg.Add(3)

	go func() {
		defer wg.Done()
		findings, findingsErr = s.RecallByCategory(ctx, queryText, "finding", limitPerCategory)
	}()

	go func() {
		defer wg.Done()
		insights, insightsErr = s.RecallByCategory(ctx, queryText, "insight", limitPerCategory)
	}()

	go func() {
		defer wg.Done()
		lessons, lessonsErr = s.RecallByCategory(ctx, queryText, "lesson", limitPerCategory)
	}()

	wg.Wait()

	// Return first error if any
	if findingsErr != nil {
		return nil, findingsErr
	}
	if insightsErr != nil {
		return nil, insightsErr
	}
	if lessonsErr != nil {
		return nil, lessonsErr
	}

	return &FILResult{
		Findings: findings,
		Insights: insights,
		Lessons:  lessons,
	}, nil
}

// Recall performs semantic search for relevant memories.
func (s *BleveStore) Recall(ctx context.Context, queryText string, opts RecallOpts) ([]MemoryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Use pure BM25 - semantic graph expansion was found to hurt recall quality
	searchQuery := bleve.NewMatchQuery(queryText)

	// Create search request
	searchReq := bleve.NewSearchRequest(searchQuery)
	searchReq.Size = limit
	searchReq.Fields = []string{"*"}

	// Execute search
	searchResult, err := s.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert results
	var results []MemoryResult
	for _, hit := range searchResult.Hits {
		// Convert score to 0-1 range (BM25 scores can be > 1)
		score := float32(hit.Score)
		if score > 1 {
			score = 1 - (1 / (1 + score)) // Normalize high scores
		}

		if score < opts.MinScore {
			continue
		}

		// Extract fields from hit
		content, _ := hit.Fields["content"].(string)
		source, _ := hit.Fields["source"].(string)
		category, _ := hit.Fields["category"].(string)

		result := MemoryResult{
			Memory: Memory{
				ID:       hit.ID,
				Content:  content,
				Category: category,
				Source:   source,
			},
			Score: score,
		}
		results = append(results, result)
	}

	return results, nil
}

// Get retrieves a value by key (KV store).
func (s *BleveStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.kv[key]; ok {
		return val, nil
	}
	return "", fmt.Errorf("key not found: %s", key)
}

// Set stores a key-value pair.
func (s *BleveStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.kv[key] = value
	return s.saveKV()
}

// List returns keys matching a prefix.
func (s *BleveStore) List(prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var keys []string
	for k := range s.kv {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// Search performs substring search on KV store.
func (s *BleveStore) Search(queryStr string) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queryLower := strings.ToLower(queryStr)
	var results []SearchResult
	for k, v := range s.kv {
		if strings.Contains(strings.ToLower(k), queryLower) ||
			strings.Contains(strings.ToLower(v), queryLower) {
			results = append(results, SearchResult{Key: k, Value: v})
		}
	}
	return results, nil
}

// ConsolidateSession extracts and stores insights from a session transcript.
func (s *BleveStore) ConsolidateSession(ctx context.Context, sessionID string, transcript []Message) error {
	if len(transcript) == 0 {
		return nil
	}

	// Extract key content from the session
	var insights []string
	for _, msg := range transcript {
		content := msg.Content
		lower := strings.ToLower(content)

		// Heuristic: messages containing decision/conclusion language
		if containsAny(lower, []string{
			"decided", "conclusion", "important", "remember",
			"note that", "key insight", "learned that",
			"will use", "should use", "agreed",
		}) {
			insights = append(insights, content)
		}
	}

	// Also include the last assistant message as a summary
	for i := len(transcript) - 1; i >= 0; i-- {
		if transcript[i].Role == "assistant" && len(transcript[i].Content) > 100 {
			insights = append(insights, transcript[i].Content)
			break
		}
	}

	// Store each insight
	source := "session:" + sessionID
	for _, insight := range insights {
		if len(insight) < 50 {
			continue
		}
		if len(insight) > 2000 {
			insight = insight[:2000] + "..."
		}

		s.RememberObservation(ctx, insight, "insight", source)
	}

	return nil
}

// Close closes the store and saves state.
func (s *BleveStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Save KV store
	s.saveKV()

	// Close Bleve index
	return s.index.Close()
}

// loadKV loads the KV store from disk.
func (s *BleveStore) loadKV() error {
	data, err := os.ReadFile(s.kvPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.kv)
}

// saveKV saves the KV store to disk.
func (s *BleveStore) saveKV() error {
	data, err := json.MarshalIndent(s.kv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.kvPath, data, 0644)
}

// ListAll returns all observations, optionally filtered by category.
// If category is empty, returns all observations.
func (s *BleveStore) ListAll(ctx context.Context, category string, limit int) ([]ObservationItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 1000
	}

	// Build query: MatchAll optionally filtered by category
	var searchQuery query.Query
	if category != "" {
		categoryQuery := bleve.NewTermQuery(category)
		categoryQuery.SetField("category")
		searchQuery = categoryQuery
	} else {
		searchQuery = bleve.NewMatchAllQuery()
	}

	searchReq := bleve.NewSearchRequest(searchQuery)
	searchReq.Size = limit
	searchReq.Fields = []string{"content", "category", "source", "created_at"}

	result, err := s.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	items := make([]ObservationItem, 0, len(result.Hits))
	for _, hit := range result.Hits {
		item := ObservationItem{ID: hit.ID}
		if v, ok := hit.Fields["content"].(string); ok {
			item.Content = v
		}
		if v, ok := hit.Fields["category"].(string); ok {
			item.Category = v
		}
		items = append(items, item)
	}

	return items, nil
}

// containsAny checks if text contains any of the patterns.
func containsAny(text string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
