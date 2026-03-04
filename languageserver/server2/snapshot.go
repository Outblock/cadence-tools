package server2

import "sync"

// AnalysisHost owns all mutable state for the language server.
// Writes are expected on the main goroutine; Snapshot() produces
// an immutable view safe for concurrent reads.
type AnalysisHost struct {
	mu       sync.RWMutex
	revision uint64
	docs     *DocumentStore
	cache    *LRUCheckerCache
	depGraph *DependencyGraph
}

// NewAnalysisHost creates a new AnalysisHost with the given checker cache capacity.
func NewAnalysisHost(cacheCapacity int) *AnalysisHost {
	return &AnalysisHost{
		docs:     NewDocumentStore(),
		cache:    NewLRUCheckerCache(cacheCapacity),
		depGraph: NewDependencyGraph(),
	}
}

// UpdateDocument stores the new document content, increments the revision,
// and invalidates cache entries for this file and all its transitive dependents.
func (h *AnalysisHost) UpdateDocument(uri DocumentURI, text string, version int32) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.revision++
	h.docs.Set(uri, text, version)

	// Invalidate transitive dependents first, then the file itself.
	dependents := h.depGraph.Invalidate(uri)
	for _, dep := range dependents {
		h.cache.Delete(dep)
	}
	h.cache.Delete(uri)
}

// RemoveDocument removes a document, increments the revision,
// invalidates cache entries for the file and its transitive dependents,
// and clears the file's dependency edges.
func (h *AnalysisHost) RemoveDocument(uri DocumentURI) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.revision++

	// Invalidate transitive dependents before clearing edges.
	dependents := h.depGraph.Invalidate(uri)
	for _, dep := range dependents {
		h.cache.Delete(dep)
	}
	h.cache.Delete(uri)

	h.depGraph.ClearDependenciesOf(uri)
	h.docs.Delete(uri)
}

// Snapshot returns an immutable view of the current state.
// Documents are copied; Cache and DepGraph are shared (they are internally thread-safe).
func (h *AnalysisHost) Snapshot() *Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return &Snapshot{
		Revision:  h.revision,
		Documents: h.docs.Snapshot(),
		Cache:     h.cache,
		DepGraph:  h.depGraph,
	}
}

// Cache returns the shared LRU checker cache.
func (h *AnalysisHost) Cache() *LRUCheckerCache {
	return h.cache
}

// GetDocument retrieves a document by URI from the document store.
func (h *AnalysisHost) GetDocument(uri DocumentURI) (Document, bool) {
	return h.docs.Get(uri)
}

// DepGraph returns the shared dependency graph.
func (h *AnalysisHost) DepGraph() *DependencyGraph {
	return h.depGraph
}

// Snapshot is an immutable view of the analysis state at a point in time.
type Snapshot struct {
	Revision  uint64
	Documents map[DocumentURI]Document
	Cache     *LRUCheckerCache // shared, thread-safe
	DepGraph  *DependencyGraph // shared, thread-safe
}
