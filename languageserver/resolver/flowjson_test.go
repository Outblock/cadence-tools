package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onflow/cadence/common"
)

func TestFlowJSONResolver_SimpleString(t *testing.T) {
	dir := t.TempDir()

	// Create flow.json with simple string contract path
	flowJSON := `{
		"contracts": {
			"FungibleToken": "./contracts/FungibleToken.cdc"
		}
	}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	// Create the contract file
	contractCode := `access(all) contract FungibleToken {}`
	writeFile(t, filepath.Join(dir, "contracts", "FungibleToken.cdc"), contractCode)

	r := NewFlowJSONResolver(dir)
	code, err := r.ResolveImport(context.Background(), common.StringLocation("FungibleToken"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != contractCode {
		t.Fatalf("got %q, want %q", code, contractCode)
	}
}

func TestFlowJSONResolver_ObjectEntry(t *testing.T) {
	dir := t.TempDir()

	// flow.json with object-style contract entry (as flow dependencies install creates)
	flowJSON := `{
		"contracts": {
			"NonFungibleToken": {
				"source": "./cadence/contracts/NonFungibleToken/NonFungibleToken.cdc",
				"aliases": {"mainnet": "0x1d7e57aa55817448"}
			}
		}
	}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	contractCode := `access(all) contract interface NonFungibleToken {}`
	writeFile(t, filepath.Join(dir, "cadence/contracts/NonFungibleToken/NonFungibleToken.cdc"), contractCode)

	r := NewFlowJSONResolver(dir)
	code, err := r.ResolveImport(context.Background(), common.StringLocation("NonFungibleToken"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != contractCode {
		t.Fatalf("got %q, want %q", code, contractCode)
	}
}

func TestFlowJSONResolver_NotFound(t *testing.T) {
	dir := t.TempDir()

	flowJSON := `{"contracts": {"Foo": "./foo.cdc"}}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	r := NewFlowJSONResolver(dir)

	// Unknown contract name
	_, err := r.ResolveImport(context.Background(), common.StringLocation("Unknown"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// File path imports should pass through
	_, err = r.ResolveImport(context.Background(), common.StringLocation("./foo.cdc"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for file path, got: %v", err)
	}

	// Non-string locations should pass through
	_, err = r.ResolveImport(context.Background(), common.AddressLocation{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for AddressLocation, got: %v", err)
	}
}

func TestFlowJSONResolver_NoFlowJSON(t *testing.T) {
	dir := t.TempDir()

	r := NewFlowJSONResolver(dir)
	_, err := r.ResolveImport(context.Background(), common.StringLocation("Foo"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFlowJSONResolver_Reload(t *testing.T) {
	dir := t.TempDir()

	// Start with empty contracts
	writeFile(t, filepath.Join(dir, "flow.json"), `{"contracts": {}}`)

	r := NewFlowJSONResolver(dir)
	_, err := r.ResolveImport(context.Background(), common.StringLocation("NewContract"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// Update flow.json with new contract
	writeFile(t, filepath.Join(dir, "flow.json"), `{"contracts": {"NewContract": "./new.cdc"}}`)
	writeFile(t, filepath.Join(dir, "new.cdc"), `access(all) contract NewContract {}`)

	// Still cached — should still fail
	_, err = r.ResolveImport(context.Background(), common.StringLocation("NewContract"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before reload, got: %v", err)
	}

	// Reload and retry
	r.Reload()
	code, err := r.ResolveImport(context.Background(), common.StringLocation("NewContract"))
	if err != nil {
		t.Fatalf("unexpected error after reload: %v", err)
	}
	if code != `access(all) contract NewContract {}` {
		t.Fatalf("unexpected code: %q", code)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
