package server2

import (
	"fmt"
	"sync"
	"testing"
)

func TestDocumentStore_SetAndGet(t *testing.T) {
	store := NewDocumentStore()

	store.Set("file:///main.cdc", "access(all) fun main() {}", 1)

	doc, ok := store.Get("file:///main.cdc")
	if !ok {
		t.Fatal("expected document to be found")
	}
	if doc.Text != "access(all) fun main() {}" {
		t.Fatalf("unexpected text: %q", doc.Text)
	}
	if doc.Version != 1 {
		t.Fatalf("unexpected version: %d", doc.Version)
	}
}

func TestDocumentStore_GetMissing(t *testing.T) {
	store := NewDocumentStore()

	_, ok := store.Get("file:///nonexistent.cdc")
	if ok {
		t.Fatal("expected document not to be found")
	}
}

func TestDocumentStore_SetOverwrite(t *testing.T) {
	store := NewDocumentStore()

	store.Set("file:///main.cdc", "v1", 1)
	store.Set("file:///main.cdc", "v2", 2)

	doc, ok := store.Get("file:///main.cdc")
	if !ok {
		t.Fatal("expected document to be found")
	}
	if doc.Text != "v2" {
		t.Fatalf("expected overwritten text, got: %q", doc.Text)
	}
	if doc.Version != 2 {
		t.Fatalf("expected version 2, got: %d", doc.Version)
	}
}

func TestDocumentStore_Delete(t *testing.T) {
	store := NewDocumentStore()

	store.Set("file:///main.cdc", "hello", 1)
	store.Delete("file:///main.cdc")

	_, ok := store.Get("file:///main.cdc")
	if ok {
		t.Fatal("expected document to be deleted")
	}
}

func TestDocumentStore_DeleteNonexistent(t *testing.T) {
	store := NewDocumentStore()

	// Should not panic.
	store.Delete("file:///nonexistent.cdc")
}

func TestDocumentStore_Snapshot(t *testing.T) {
	store := NewDocumentStore()

	store.Set("file:///a.cdc", "a", 1)
	store.Set("file:///b.cdc", "b", 2)

	snap := store.Snapshot()

	if len(snap) != 2 {
		t.Fatalf("expected 2 documents in snapshot, got %d", len(snap))
	}
	if snap["file:///a.cdc"].Text != "a" {
		t.Fatalf("unexpected text for a.cdc: %q", snap["file:///a.cdc"].Text)
	}
	if snap["file:///b.cdc"].Text != "b" {
		t.Fatalf("unexpected text for b.cdc: %q", snap["file:///b.cdc"].Text)
	}
}

func TestDocumentStore_SnapshotIsACopy(t *testing.T) {
	store := NewDocumentStore()

	store.Set("file:///main.cdc", "original", 1)

	snap := store.Snapshot()

	// Mutate the snapshot map.
	snap["file:///main.cdc"] = Document{Text: "mutated", Version: 99}
	snap["file:///new.cdc"] = Document{Text: "added", Version: 1}

	// Original store must be unaffected.
	doc, ok := store.Get("file:///main.cdc")
	if !ok {
		t.Fatal("expected document to still exist in store")
	}
	if doc.Text != "original" {
		t.Fatalf("expected original text, got: %q", doc.Text)
	}

	_, ok = store.Get("file:///new.cdc")
	if ok {
		t.Fatal("snapshot mutation should not affect the store")
	}
}

func TestDocumentStore_ConcurrentAccess(t *testing.T) {
	store := NewDocumentStore()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()

			uri := DocumentURI(fmt.Sprintf("file:///file_%d.cdc", n))

			store.Set(uri, fmt.Sprintf("content_%d", n), int32(n))

			if doc, ok := store.Get(uri); ok {
				_ = doc.Text
			}

			store.Snapshot()

			if n%2 == 0 {
				store.Delete(uri)
			}
		}(i)
	}

	wg.Wait()
}
