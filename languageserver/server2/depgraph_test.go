package server2

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

func sortedURIs(uris []DocumentURI) []DocumentURI {
	sort.Strings(uris)
	return uris
}

func TestDependencyGraph_AddEdgeAndQuery(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B
	g.AddEdge("file:///a.cdc", "file:///b.cdc")

	deps := g.DependenciesOf("file:///a.cdc")
	if len(deps) != 1 || deps[0] != "file:///b.cdc" {
		t.Fatalf("expected DependenciesOf(A) = [B], got %v", deps)
	}

	rev := g.DependentsOf("file:///b.cdc")
	if len(rev) != 1 || rev[0] != "file:///a.cdc" {
		t.Fatalf("expected DependentsOf(B) = [A], got %v", rev)
	}
}

func TestDependencyGraph_MultipleDependencies(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B and C
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///a.cdc", "file:///c.cdc")

	deps := sortedURIs(g.DependenciesOf("file:///a.cdc"))
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d: %v", len(deps), deps)
	}
	if deps[0] != "file:///b.cdc" || deps[1] != "file:///c.cdc" {
		t.Fatalf("unexpected dependencies: %v", deps)
	}
}

func TestDependencyGraph_MultipleDependents(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B, C imports B
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///c.cdc", "file:///b.cdc")

	rev := sortedURIs(g.DependentsOf("file:///b.cdc"))
	if len(rev) != 2 {
		t.Fatalf("expected 2 dependents, got %d: %v", len(rev), rev)
	}
	if rev[0] != "file:///a.cdc" || rev[1] != "file:///c.cdc" {
		t.Fatalf("unexpected dependents: %v", rev)
	}
}

func TestDependencyGraph_DuplicateEdge(t *testing.T) {
	g := NewDependencyGraph()

	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///a.cdc", "file:///b.cdc")

	deps := g.DependenciesOf("file:///a.cdc")
	if len(deps) != 1 {
		t.Fatalf("duplicate edge should not create extra entries, got %v", deps)
	}
}

func TestDependencyGraph_QueryEmpty(t *testing.T) {
	g := NewDependencyGraph()

	deps := g.DependenciesOf("file:///a.cdc")
	if len(deps) != 0 {
		t.Fatalf("expected empty dependencies, got %v", deps)
	}

	rev := g.DependentsOf("file:///a.cdc")
	if len(rev) != 0 {
		t.Fatalf("expected empty dependents, got %v", rev)
	}
}

func TestDependencyGraph_InvalidateTransitive(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B, C imports B, D imports A
	// Invalidate(B) should return {A, C, D}
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///c.cdc", "file:///b.cdc")
	g.AddEdge("file:///d.cdc", "file:///a.cdc")

	result := sortedURIs(g.Invalidate("file:///b.cdc"))
	expected := []DocumentURI{"file:///a.cdc", "file:///c.cdc", "file:///d.cdc"}

	if len(result) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	}
}

func TestDependencyGraph_InvalidateDoesNotIncludeSelf(t *testing.T) {
	g := NewDependencyGraph()

	g.AddEdge("file:///a.cdc", "file:///b.cdc")

	result := g.Invalidate("file:///b.cdc")
	for _, uri := range result {
		if uri == "file:///b.cdc" {
			t.Fatal("Invalidate should NOT include the uri itself")
		}
	}
}

func TestDependencyGraph_InvalidateLeaf(t *testing.T) {
	g := NewDependencyGraph()

	g.AddEdge("file:///a.cdc", "file:///b.cdc")

	// A is a leaf — nothing imports A
	result := g.Invalidate("file:///a.cdc")
	if len(result) != 0 {
		t.Fatalf("expected empty result for leaf, got %v", result)
	}
}

func TestDependencyGraph_InvalidateUnknown(t *testing.T) {
	g := NewDependencyGraph()

	result := g.Invalidate("file:///unknown.cdc")
	if len(result) != 0 {
		t.Fatalf("expected empty result for unknown uri, got %v", result)
	}
}

func TestDependencyGraph_InvalidateCycle(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B, B imports A (cycle)
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///b.cdc", "file:///a.cdc")

	result := sortedURIs(g.Invalidate("file:///a.cdc"))
	// B imports A, so B is a dependent. A imports B, so A is also a dependent of B.
	// But Invalidate(A) should not include A itself.
	if len(result) != 1 || result[0] != "file:///b.cdc" {
		t.Fatalf("expected [B] for cycle, got %v", result)
	}
}

func TestDependencyGraph_InvalidateDeepChain(t *testing.T) {
	g := NewDependencyGraph()

	// E imports D imports C imports B imports A
	g.AddEdge("file:///b.cdc", "file:///a.cdc")
	g.AddEdge("file:///c.cdc", "file:///b.cdc")
	g.AddEdge("file:///d.cdc", "file:///c.cdc")
	g.AddEdge("file:///e.cdc", "file:///d.cdc")

	result := sortedURIs(g.Invalidate("file:///a.cdc"))
	expected := []DocumentURI{"file:///b.cdc", "file:///c.cdc", "file:///d.cdc", "file:///e.cdc"}

	if len(result) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, result)
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Fatalf("expected %v, got %v", expected, result)
		}
	}
}

func TestDependencyGraph_ClearDependenciesOf(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B and C
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///a.cdc", "file:///c.cdc")

	g.ClearDependenciesOf("file:///a.cdc")

	// Forward edges from A should be gone
	deps := g.DependenciesOf("file:///a.cdc")
	if len(deps) != 0 {
		t.Fatalf("expected no dependencies after clear, got %v", deps)
	}

	// Reverse edges should also be cleaned
	revB := g.DependentsOf("file:///b.cdc")
	if len(revB) != 0 {
		t.Fatalf("expected B to have no dependents after clear, got %v", revB)
	}
	revC := g.DependentsOf("file:///c.cdc")
	if len(revC) != 0 {
		t.Fatalf("expected C to have no dependents after clear, got %v", revC)
	}
}

func TestDependencyGraph_ClearDependenciesPreservesOtherEdges(t *testing.T) {
	g := NewDependencyGraph()

	// A imports B, C imports B
	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.AddEdge("file:///c.cdc", "file:///b.cdc")

	g.ClearDependenciesOf("file:///a.cdc")

	// C still imports B
	revB := g.DependentsOf("file:///b.cdc")
	if len(revB) != 1 || revB[0] != "file:///c.cdc" {
		t.Fatalf("expected C to still depend on B, got %v", revB)
	}
}

func TestDependencyGraph_ClearDependenciesOfUnknown(t *testing.T) {
	g := NewDependencyGraph()

	// Should not panic.
	g.ClearDependenciesOf("file:///unknown.cdc")
}

func TestDependencyGraph_ClearThenReAdd(t *testing.T) {
	g := NewDependencyGraph()

	g.AddEdge("file:///a.cdc", "file:///b.cdc")
	g.ClearDependenciesOf("file:///a.cdc")
	g.AddEdge("file:///a.cdc", "file:///c.cdc")

	deps := g.DependenciesOf("file:///a.cdc")
	if len(deps) != 1 || deps[0] != "file:///c.cdc" {
		t.Fatalf("expected [C] after clear+re-add, got %v", deps)
	}

	revB := g.DependentsOf("file:///b.cdc")
	if len(revB) != 0 {
		t.Fatalf("expected B to have no dependents, got %v", revB)
	}

	revC := g.DependentsOf("file:///c.cdc")
	if len(revC) != 1 || revC[0] != "file:///a.cdc" {
		t.Fatalf("expected C dependents = [A], got %v", revC)
	}
}

func TestDependencyGraph_ConcurrentAccess(t *testing.T) {
	g := NewDependencyGraph()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()

			from := DocumentURI(fmt.Sprintf("file:///file_%d.cdc", n))
			to := DocumentURI(fmt.Sprintf("file:///dep_%d.cdc", n%5))

			g.AddEdge(from, to)
			g.DependenciesOf(from)
			g.DependentsOf(to)
			g.Invalidate(to)

			if n%3 == 0 {
				g.ClearDependenciesOf(from)
			}
		}(i)
	}

	wg.Wait()
}
