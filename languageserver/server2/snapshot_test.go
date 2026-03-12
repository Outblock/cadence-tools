package server2

import (
	"testing"

	"github.com/onflow/cadence/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotReturnsCurrentDocuments(t *testing.T) {
	h := NewAnalysisHost(64)
	h.UpdateDocument("file:///a.cdc", "access(all) fun a() {}", 1)
	h.UpdateDocument("file:///b.cdc", "access(all) fun b() {}", 1)

	snap := h.Snapshot()

	require.Len(t, snap.Documents, 2)
	assert.Equal(t, "access(all) fun a() {}", snap.Documents["file:///a.cdc"].Text)
	assert.Equal(t, "access(all) fun b() {}", snap.Documents["file:///b.cdc"].Text)
}

func TestRevisionIncrementsOnUpdate(t *testing.T) {
	h := NewAnalysisHost(64)

	s0 := h.Snapshot()
	assert.Equal(t, uint64(0), s0.Revision)

	h.UpdateDocument("file:///a.cdc", "v1", 1)
	s1 := h.Snapshot()
	assert.Equal(t, uint64(1), s1.Revision)

	h.UpdateDocument("file:///a.cdc", "v2", 2)
	s2 := h.Snapshot()
	assert.Equal(t, uint64(2), s2.Revision)

	h.RemoveDocument("file:///a.cdc")
	s3 := h.Snapshot()
	assert.Equal(t, uint64(3), s3.Revision)
}

func TestSnapshotIsolation(t *testing.T) {
	h := NewAnalysisHost(64)
	h.UpdateDocument("file:///a.cdc", "original", 1)

	snap := h.Snapshot()
	assert.Equal(t, "original", snap.Documents["file:///a.cdc"].Text)

	// Mutate host after snapshot
	h.UpdateDocument("file:///a.cdc", "modified", 2)
	h.UpdateDocument("file:///c.cdc", "new file", 1)

	// Snapshot must be unchanged
	assert.Equal(t, "original", snap.Documents["file:///a.cdc"].Text)
	assert.NotContains(t, snap.Documents, "file:///c.cdc")
	assert.Len(t, snap.Documents, 1)
}

func TestUpdateDocumentInvalidatesCacheForChangedFile(t *testing.T) {
	h := NewAnalysisHost(64)

	keyA := CanonicalCacheKey(common.StringLocation("file:///a.cdc"))

	// Seed cache with an entry for the file
	h.Cache().Put(keyA, &CheckerEntry{Valid: true})

	// Verify it's present
	_, ok := h.Cache().Get(keyA)
	require.True(t, ok)

	// Update the document - should invalidate its cache entry
	h.UpdateDocument("file:///a.cdc", "changed", 2)

	_, ok = h.Cache().Get(keyA)
	assert.False(t, ok, "cache entry for the changed file should be invalidated")
}

func TestUpdateDocumentInvalidatesTransitiveDependents(t *testing.T) {
	h := NewAnalysisHost(64)

	keyA := CanonicalCacheKey(common.StringLocation("file:///a.cdc"))
	keyB := CanonicalCacheKey(common.StringLocation("file:///b.cdc"))
	keyC := CanonicalCacheKey(common.StringLocation("file:///c.cdc"))

	// Set up dependency chain: C imports B imports A
	// When A changes, both B and C should be invalidated.
	h.DepGraph().AddEdge(keyB, keyA) // B imports A
	h.DepGraph().AddEdge(keyC, keyB) // C imports B

	// Seed cache entries for all three
	h.Cache().Put(keyA, &CheckerEntry{Valid: true})
	h.Cache().Put(keyB, &CheckerEntry{Valid: true})
	h.Cache().Put(keyC, &CheckerEntry{Valid: true})

	// Update A - should invalidate A, B (direct dependent), and C (transitive)
	h.UpdateDocument("file:///a.cdc", "changed A", 2)

	_, okA := h.Cache().Get(keyA)
	_, okB := h.Cache().Get(keyB)
	_, okC := h.Cache().Get(keyC)

	assert.False(t, okA, "cache for changed file A should be invalidated")
	assert.False(t, okB, "cache for direct dependent B should be invalidated")
	assert.False(t, okC, "cache for transitive dependent C should be invalidated")
}

func TestRemoveDocumentClearsDependencyEdges(t *testing.T) {
	h := NewAnalysisHost(64)

	keyA := CanonicalCacheKey(common.StringLocation("file:///a.cdc"))
	keyB := CanonicalCacheKey(common.StringLocation("file:///b.cdc"))

	// B imports A
	h.DepGraph().AddEdge(keyB, keyA)
	h.UpdateDocument("file:///b.cdc", "import A", 1)

	// Verify edge exists
	deps := h.DepGraph().DependenciesOf(keyB)
	require.Len(t, deps, 1)

	// Remove B - should clear its forward edges
	h.RemoveDocument("file:///b.cdc")

	deps = h.DepGraph().DependenciesOf(keyB)
	assert.Empty(t, deps, "forward edges of removed file should be cleared")

	// Reverse edge from A should also be cleaned up
	dependents := h.DepGraph().DependentsOf(keyA)
	assert.Empty(t, dependents, "reverse edges pointing to removed file should be cleaned up")
}

func TestRemoveDocumentInvalidatesDependents(t *testing.T) {
	h := NewAnalysisHost(64)

	keyA := CanonicalCacheKey(common.StringLocation("file:///a.cdc"))
	keyB := CanonicalCacheKey(common.StringLocation("file:///b.cdc"))

	// B imports A
	h.DepGraph().AddEdge(keyB, keyA)
	h.Cache().Put(keyA, &CheckerEntry{Valid: true})
	h.Cache().Put(keyB, &CheckerEntry{Valid: true})

	h.UpdateDocument("file:///a.cdc", "code", 1)
	h.UpdateDocument("file:///b.cdc", "import A", 1)

	// Remove A - should invalidate B's cache too
	h.RemoveDocument("file:///a.cdc")

	_, okA := h.Cache().Get(keyA)
	_, okB := h.Cache().Get(keyB)
	assert.False(t, okA, "cache for removed file should be invalidated")
	assert.False(t, okB, "cache for dependent of removed file should be invalidated")
}

func TestCacheAndDepGraphAccessors(t *testing.T) {
	h := NewAnalysisHost(64)

	assert.NotNil(t, h.Cache())
	assert.NotNil(t, h.DepGraph())
}

func TestSnapshotSharesCacheAndDepGraph(t *testing.T) {
	h := NewAnalysisHost(64)
	snap := h.Snapshot()

	// Cache and DepGraph should be the same instances (shared, thread-safe)
	assert.Same(t, h.Cache(), snap.Cache)
	assert.Same(t, h.DepGraph(), snap.DepGraph)
}

func TestUpdateDocumentInvalidatesCacheWithCanonicalKey(t *testing.T) {
	host := NewAnalysisHost(64)
	uri := DocumentURI("file:///test.cdc")

	cacheKey := CanonicalCacheKey(common.StringLocation(uri))
	host.Cache().Put(cacheKey, &CheckerEntry{Valid: true})

	_, found := host.Cache().Get(cacheKey)
	require.True(t, found, "entry should be in cache before update")

	host.UpdateDocument(uri, "new content", 2)

	_, found = host.Cache().Get(cacheKey)
	assert.False(t, found, "cache entry should be invalidated after UpdateDocument")
}
