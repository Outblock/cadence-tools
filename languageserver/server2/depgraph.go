package server2

import "sync"

// DependencyGraph tracks import relationships between files.
// "A imports B" is recorded as a forward edge from A to B and a reverse edge from B to A.
type DependencyGraph struct {
	mu      sync.RWMutex
	forward map[DocumentURI]map[DocumentURI]struct{}
	reverse map[DocumentURI]map[DocumentURI]struct{}
}

// NewDependencyGraph creates an empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		forward: make(map[DocumentURI]map[DocumentURI]struct{}),
		reverse: make(map[DocumentURI]map[DocumentURI]struct{}),
	}
}

// AddEdge records that "from" imports "to".
func (g *DependencyGraph) AddEdge(from, to DocumentURI) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.forward[from] == nil {
		g.forward[from] = make(map[DocumentURI]struct{})
	}
	g.forward[from][to] = struct{}{}

	if g.reverse[to] == nil {
		g.reverse[to] = make(map[DocumentURI]struct{})
	}
	g.reverse[to][from] = struct{}{}
}

// DependenciesOf returns the URIs that the given uri imports (forward edges).
func (g *DependencyGraph) DependenciesOf(uri DocumentURI) []DocumentURI {
	g.mu.RLock()
	defer g.mu.RUnlock()

	set := g.forward[uri]
	result := make([]DocumentURI, 0, len(set))
	for dep := range set {
		result = append(result, dep)
	}
	return result
}

// DependentsOf returns the URIs that import the given uri (reverse edges).
func (g *DependencyGraph) DependentsOf(uri DocumentURI) []DocumentURI {
	g.mu.RLock()
	defer g.mu.RUnlock()

	set := g.reverse[uri]
	result := make([]DocumentURI, 0, len(set))
	for dep := range set {
		result = append(result, dep)
	}
	return result
}

// Invalidate returns all transitive dependents of the given uri using BFS
// over reverse edges. The uri itself is NOT included in the result.
func (g *DependencyGraph) Invalidate(uri DocumentURI) []DocumentURI {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visited := make(map[DocumentURI]struct{})
	visited[uri] = struct{}{} // mark origin as visited so it won't appear in results

	queue := []DocumentURI{uri}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for dep := range g.reverse[current] {
			if _, seen := visited[dep]; !seen {
				visited[dep] = struct{}{}
				queue = append(queue, dep)
			}
		}
	}

	// Collect all visited except the origin.
	result := make([]DocumentURI, 0, len(visited)-1)
	for v := range visited {
		if v != uri {
			result = append(result, v)
		}
	}
	return result
}

// ClearDependenciesOf removes all forward edges from the given uri,
// and cleans up the corresponding reverse edges.
func (g *DependencyGraph) ClearDependenciesOf(uri DocumentURI) {
	g.mu.Lock()
	defer g.mu.Unlock()

	targets := g.forward[uri]
	for to := range targets {
		if revSet := g.reverse[to]; revSet != nil {
			delete(revSet, uri)
			if len(revSet) == 0 {
				delete(g.reverse, to)
			}
		}
	}
	delete(g.forward, uri)
}
