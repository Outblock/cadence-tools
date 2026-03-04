package server2

import "sync"

// DocumentURI is a file URI as defined by the LSP spec.
type DocumentURI = string

// Document represents an open text document.
type Document struct {
	Text    string
	Version int32
}

// DocumentStore is a thread-safe store for open documents.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[DocumentURI]Document
}

// NewDocumentStore creates a new, empty DocumentStore.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		docs: make(map[DocumentURI]Document),
	}
}

// Set inserts or updates a document in the store.
func (s *DocumentStore) Set(uri DocumentURI, text string, version int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[uri] = Document{Text: text, Version: version}
}

// Get retrieves a document by URI. The second return value indicates
// whether the document was found.
func (s *DocumentStore) Get(uri DocumentURI) (Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	return doc, ok
}

// Delete removes a document from the store. It is a no-op if the
// document does not exist.
func (s *DocumentStore) Delete(uri DocumentURI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

// Snapshot returns a shallow copy of all documents in the store.
// Mutating the returned map does not affect the store.
func (s *DocumentStore) Snapshot() map[DocumentURI]Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[DocumentURI]Document, len(s.docs))
	for k, v := range s.docs {
		cp[k] = v
	}
	return cp
}
